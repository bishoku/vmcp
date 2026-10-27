// Semantic chart regression tests; no cluster or chart-testing plugin required.
const {test}=require('node:test');const assert=require('node:assert/strict');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path'),cp=require('node:child_process');
const yaml=require('../../web/node_modules/yaml');
const helm=process.env.HELM_BIN||'helm',chart='deploy/helm/vmcp',production=chart+'/examples/production.yaml';
function render(values={},args=[],success=true){
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'vmcp-chart-'));
 try{
  const file=path.join(dir,'values.yaml');fs.writeFileSync(file,yaml.stringify(values));
  const r=cp.spawnSync(helm,['template','test',chart,'-f',production,'-f',file,...args],{encoding:'utf8'});
  if(!success){assert.notEqual(r.status,0,'invalid configuration rendered successfully');return r.stderr;}
  assert.equal(r.status,0,r.stderr);return yaml.parseAllDocuments(r.stdout).map(d=>d.toJSON()).filter(Boolean);
 }finally{fs.rmSync(dir,{recursive:true,force:true});}
}
const deployments=docs=>docs.filter(d=>d.kind==='Deployment');
const env=d=>d.spec.template.spec.containers[0].env;
test('external deployment includes three hardened workloads and an install migration Job',()=>{
 const docs=render(),ds=deployments(docs);assert.equal(ds.length,3);
 const job=docs.find(d=>d.kind==='Job');assert.ok(job);assert.equal(job.metadata.annotations?.['helm.sh/hook'],undefined);
 assert.equal(job.spec.template.spec.automountServiceAccountToken,false);
 for(const d of ds){const p=d.spec.template.spec;assert.equal(p.automountServiceAccountToken,false);assert.equal(p.securityContext.seccompProfile.type,'RuntimeDefault');assert.equal(p.containers[0].securityContext.readOnlyRootFilesystem,true);assert.equal(d.spec.replicas,2);assert.equal(d.spec.strategy.rollingUpdate.maxUnavailable,0);
  const names=env(d).map(e=>e.name);assert.equal(new Set(names).size,names.length,'duplicate environment keys');}
 const dp=ds.find(d=>d.metadata.name.endsWith('data-plane'));
 assert.ok(!env(dp).some(e=>e.name==='VMCP_ADMIN_TOKEN'));assert.ok(env(dp).find(e=>e.name==='VMCP_NODE_ID').valueFrom.fieldRef);
 assert.equal(docs.filter(d=>d.kind==='Secret').length,0,'external secrets must not be copied');
 const ingress=docs.find(d=>d.kind==='Ingress');assert.ok(ingress.spec.rules.flatMap(r=>r.http.paths).every(p=>p.path!=='/metrics'));
});
test('upgrade blocks on a migration hook and manual migration mode emits no Job',()=>{
 const job=render({},['--is-upgrade']).find(d=>d.kind==='Job');assert.equal(job.metadata.annotations['helm.sh/hook'],'pre-upgrade');
 assert.ok(!render({migration:{mode:'manual'}}).some(d=>d.kind==='Job'));
});
test('chart-created secrets are stable and never fall back to random generation',()=>{
 const values={credentials:{masterKey:{existingSecret:'',value:Buffer.alloc(32,1).toString('base64')}},database:{listener:{existingSecret:'',value:'postgres://listener.example/db'}}};
 const a=render(values).filter(d=>d.kind==='Secret'),b=render(values).filter(d=>d.kind==='Secret');assert.deepEqual(a,b);assert.equal(a.length,2);
 assert.ok(a.some(s=>Object.values(s.stringData).includes(values.credentials.masterKey.value)));
});
test('all exporter binaries are independent and use only their own credentials',()=>{
 const docs=render({},['-f',chart+'/examples/exporters.yaml']);assert.equal(deployments(docs).length,6);
 for(const suffix of ['http','kafka','rabbitmq']){
  const d=deployments(docs).find(d=>d.metadata.name.endsWith('audit-exporter-'+suffix));const c=d.spec.template.spec.containers[0];
  assert.equal(env(d).find(e=>e.name==='VMCP_AUDIT_EXPORT_DATABASE_URL').valueFrom.secretKeyRef.key,suffix+'-url');
  assert.ok(!env(d).some(e=>e.name==='VMCP_MASTER_KEY'));assert.equal(c.readinessProbe,undefined);assert.equal(c.ports,undefined);
 }
});
test('autoscaling owns replica count and preserves minimum availability',()=>{
 const docs=render({dataPlane:{autoscaling:{enabled:true,minReplicas:3,maxReplicas:7}}});
 const dp=deployments(docs).find(d=>d.metadata.name.endsWith('data-plane'));assert.equal(dp.spec.replicas,undefined);
 const hpa=docs.find(d=>d.kind==='HorizontalPodAutoscaler');assert.equal(hpa.spec.maxReplicas,7);assert.equal(hpa.spec.minReplicas,3);
});
test('network policies select application components and do not accidentally select dependency pods',()=>{
 const docs=render();const policies=docs.filter(d=>d.kind==='NetworkPolicy');assert.equal(policies.length,4);
 for(const p of policies)assert.ok(p.spec.podSelector.matchLabels['app.kubernetes.io/component']);
 const web=policies.find(p=>p.metadata.name.endsWith('-web'));assert.equal(web.spec.egress.length,2);
 const cpPolicy=policies.find(p=>p.metadata.name.endsWith('control-plane'));assert.equal(cpPolicy.spec.ingress[0].from[0].podSelector.matchLabels['app.kubernetes.io/component'],'web');
});
test('monitoring requires CRDs and packages the existing dashboards and alert groups',()=>{
 assert.match(render({},['-f',chart+'/examples/monitoring.yaml'],false),/CRD is required/);
 const docs=render({},['-f',chart+'/examples/monitoring.yaml','--api-versions','monitoring.coreos.com/v1/ServiceMonitor','--api-versions','monitoring.coreos.com/v1/PrometheusRule']);
 assert.equal(docs.filter(d=>d.kind==='ServiceMonitor').length,2);assert.ok(docs.find(d=>d.kind==='PrometheusRule').spec.groups.length);
 const dashboards=docs.find(d=>d.kind==='ConfigMap'&&d.metadata.name.endsWith('dashboards'));assert.equal(JSON.parse(dashboards.data['operations.json']).uid,'vmcp-operations');
 for(const file of ['alerts.yml','dashboard.json','mcp-dashboard.json'])assert.equal(fs.readFileSync(chart+'/files/'+file,'utf8'),fs.readFileSync('deploy/observability/'+file,'utf8'));
});
test('all optional pinned dependencies render together',()=>{
 const docs=render({},['-f',chart+'/examples/bundled.yaml']);
 for(const app of ['postgresql','redis','kafka','rabbitmq'])assert.ok(docs.some(d=>d.kind==='StatefulSet'&&d.metadata.labels?.['app.kubernetes.io/name']===app),app+' StatefulSet absent');
});
test('rejects unsafe or conflicting configuration before installation',()=>{
 for(const values of [
  {controlPlane:{image:'vmcp:latest'}},
  {credentials:{masterKey:{value:'bad'}}},
  {runtime:{pool:{minConnections:30,maxConnections:16}}},
  {dataPlane:{terminationGracePeriodSeconds:90},runtime:{drainTimeout:'120s'}},
  {dataPlane:{env:{VMCP_SKIP_MIGRATIONS:false}}},
  {web:{replicas:1}},
  {dataPlane:{autoscaling:{enabled:true,minReplicas:1}}},
  {dataPlane:{podLabels:{'app.kubernetes.io/component':'other'}}},
  {dataPlane:{env:{VMCP_AUDIT_MODE:'best_effort'}}},
  {imagePolicy:{requireDigest:'false'}},
  {migration:{secretEnv:{VMCP_TENANT_ID:{existingSecret:'x',key:'id'}}}},
  {postgresql:{enabled:true}},
  {exporters:{http:{enabled:true,consumer:'bad name'}}},
  {workloadDefault:{}},
 ])render(values,[],false);
});

test('optional router and additional named Secrets render without HTTP health probes',()=>{
 const docs=render({tenantRouter:{enabled:true,image:'registry.example.com/router@sha256:'+'a'.repeat(64),env:{VMCP_TENANT_DIRECTORY:'[]'}},extraSecrets:[{name:'dependency-auth',data:{password:'test-only'}}]});
 const router=deployments(docs).find(d=>d.metadata.name.endsWith('tenant-router'));
 const c=router.spec.template.spec.containers[0];assert.equal(c.ports[0].containerPort,8443);assert.equal(c.readinessProbe,undefined);
 assert.equal(docs.find(d=>d.kind==='Secret'&&d.metadata.name==='dependency-auth').stringData.password,'test-only');
});

test('Omni is optional, has no database credentials and exposes only its MCP routes',()=>{
 const docs=render({omni:{enabled:true,image:'example/omni@sha256:'+'a'.repeat(64),secret:{existingSecret:'omni-secret'}},ingress:{enabled:true,webHost:'web.example',gatewayHost:'gateway.example'}});
 const d=deployments(docs).find(d=>d.metadata.name.endsWith('omni-mcp'));assert.ok(d);
 assert.ok(!env(d).some(e=>e.name==='VMCP_DATABASE_URL'||e.name==='VMCP_MASTER_KEY'));
 assert.equal(env(d).find(e=>e.name==='VMCP_OMNI_SECRET').valueFrom.secretKeyRef.name,'omni-secret');
 const ingress=docs.find(d=>d.kind==='Ingress');const paths=ingress.spec.rules.flatMap(r=>r.http.paths);
 assert.ok(paths.some(p=>p.path==='/mcp/v1/omni'&&p.pathType==='Exact'));
 assert.ok(!paths.some(p=>p.path.startsWith('/internal')));
 assert.match(render({omni:{enabled:true,image:'example/omni@sha256:'+'a'.repeat(64),secret:{existingSecret:'omni-secret'}},dataPlane:{enabled:false}},[],false),/omni requires dataPlane/);
});
