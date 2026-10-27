#!/usr/bin/env python3
"""Isolated MicroK8s REST acceptance run. Never targets an existing namespace."""
import secrets
import argparse, hashlib, json, os, pathlib, shutil, socket, subprocess, tempfile, time, signal
from publish import publish
from postgres_observe import DatabaseObserver
from postgres_profile import arguments as postgres_arguments, settings as postgres_settings

ROOT = pathlib.Path(__file__).resolve().parents[2]

def command(args, **kwargs):
    return subprocess.run(args, check=True, text=True, **kwargs)

def capture(args):
    return subprocess.check_output(args, text=True)

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--omni', action='store_true', help='1000-tool Omni acceptance, direct/Omni/discovery comparisons')
    parser.add_argument('--smoke', action='store_true')
    parser.add_argument('--output')
    parser.add_argument('--data-plane-cpus', type=int, choices=(2,4,8), default=2, help='GOMAXPROCS and Kubernetes CPU request; limit at least 4')
    parser.add_argument('--data-plane-memory-limit', choices=('3Gi','6Gi','12Gi'), default='3Gi', help='Kubernetes hard memory limit; independent of VMCP_ACCEPTANCE_GOMEMLIMIT')
    parser.add_argument('--postgres-explain', action='store_true', help='Sample 1%% of slow PostgreSQL execution plans for diagnosis')
    parser.add_argument('--postgres-tuned', action='store_true', help='Explicit bounded OLTP cache/WAL/autovacuum profile; preserves durability')
    parser.add_argument('--database-focus', action='store_true', help='1000 TPS for five minutes, expired cleanup fixtures, PostgreSQL diagnostics')
    args = parser.parse_args()
    if args.smoke and args.database_focus:
        parser.error("--smoke and --database-focus are mutually exclusive")
    stamp = time.strftime('%Y%m%d-%H%M%S')
    namespace = f'vmcp-load-{stamp}'
    output = pathlib.Path(args.output or ROOT / 'artifacts/benchmark-runs' / f'microk8s-{stamp}').resolve()
    output.mkdir(parents=True, exist_ok=False)
    published = ROOT/'benchmark/results'/f'microk8s-{stamp}'
    kubectl = ['microk8s', 'kubectl', '--namespace', namespace]
    temporary = pathlib.Path(tempfile.mkdtemp(prefix='vmcp-k8s-load-'))
    process = None
    forward = None
    created = False
    observer = None
    result = 1
    try:
        nodes = json.loads(capture(['microk8s','kubectl','get','nodes','-o','json']))['items']
        ready = [n for n in nodes if any(c['type']=='Ready' and c['status']=='True' for c in n['status']['conditions'])]
        if len(ready) != 1:
            raise RuntimeError('This local MicroK8s profile requires exactly one Ready node; inspect placement before using a multi-node cluster.')
        existing = capture(['microk8s','kubectl','get','pods','-n','vmcp-demo','-o','json'])
        (output/'existing-demo-before.json').write_text(existing)
        node = ready[0]
        address = next(a['address'] for a in node['status']['addresses'] if a['type']=='InternalIP')
        source_files = sorted(p for p in (ROOT/'core').rglob('*') if p.is_file() and p.suffix in ('.go','.sql') and not any(x in p.parts for x in ('node_modules','vendor')))
        digest = hashlib.sha256()
        for path in source_files:
            digest.update(str(path.relative_to(ROOT)).encode()); digest.update(path.read_bytes())
        for name in ('go.mod','go.sum'):digest.update((ROOT/'core'/name).read_bytes())
        runtime_image = [line.split()[1] for line in (ROOT/'core/Dockerfile').read_text().splitlines() if line.startswith('FROM ')][-1]
        digest.update(runtime_image.encode())
        source_hash = digest.hexdigest()
        (output/'environment.json').write_text(json.dumps({'namespace':namespace,'node':node,'sourceRevision':capture(['git','rev-parse','HEAD']).strip(),'workingTree':capture(['git','status','--short']),'sourceTreeSHA256':source_hash,'loadDriver':'native host Go process; fixtures bound to selected node IP','routing':('host test proxy -> direct PodIP/CNI' if os.environ.get('VMCP_ACCEPTANCE_ROUTING') == 'pod' else 'host test proxy -> dedicated Kubernetes NodePort Services -> data-plane pods'),'smoke':args.smoke,'databaseFocus':args.database_focus},indent=2))
        binary = temporary/'data-plane'
        env = dict(os.environ, CGO_ENABLED='0')
        command(['go','build','-trimpath','-o',str(binary),'./cmd/data-plane'],cwd=ROOT/'core',env=env)
        command(['go','build','-trimpath','-o',str(temporary/'control-plane'),'./cmd/control-plane'],cwd=ROOT/'core',env=env)
        images = {}
        components = ['data-plane','control-plane']
        if args.omni:
            components.append('omni-mcp')
            command(['go','build','-trimpath','-o',str(temporary/'omni-mcp'),'./cmd/omni-mcp'],cwd=ROOT/'core',env=env)
        for component in components:
            image = f'docker.io/vmcp-load/{component}:{source_hash[:16]}'
            (temporary/'Dockerfile').write_text(f'FROM {runtime_image}\nCOPY {component} /app\nUSER 65532:65532\nENTRYPOINT ["/app"]\n')
            command(['docker','build','-t',image,'-f',str(temporary/'Dockerfile'),str(temporary)])
            images[component] = image
        for image in ('postgres:17-alpine','redis:7-alpine'):
            if subprocess.run(['docker','image','inspect',image],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode:
                command(['docker','pull',image])
        archive=temporary/'images.tar'
        command(['docker','save','-o',str(archive),*images.values(),'postgres:17-alpine','redis:7-alpine'])
        command(['/snap/microk8s/current/bin/ctr','--address','/var/snap/microk8s/common/run/containerd.sock','--namespace','k8s.io','images','import',str(archive)])
        (output/'images.json').write_text(json.dumps({name:json.loads(capture(['docker','image','inspect',image]))[0]['Id'] for name,image in images.items()},indent=2))
        command(['microk8s','kubectl','create','namespace',namespace]);created=True
        items=[]
        omni_secret = secrets.token_hex(32) if args.omni else ''
        if args.omni:
            command(kubectl+['apply','-f','-'],input=json.dumps({'apiVersion':'v1','kind':'Secret','metadata':{'name':'omni'},'stringData':{'secret':omni_secret}}))
        def deployment(name,container,replicas=1):
            items.append({'apiVersion':'apps/v1','kind':'Deployment','metadata':{'name':name},'spec':{'replicas':replicas,'selector':{'matchLabels':{'app':name}},'strategy':{'type':'Recreate'},'template':{'metadata':{'labels':{'app':name}},'spec':{'automountServiceAccountToken':False,'terminationGracePeriodSeconds':20,'containers':[container]}}}})
        def service(name,port,node_port=False):
            items.append({'apiVersion':'v1','kind':'Service','metadata':{'name':name},'spec':{'type':'NodePort' if node_port else 'ClusterIP','selector':{'app':name},'ports':[{'port':port,'targetPort':port}]}})
        deployment('postgres',{'name':'postgres','image':'docker.io/library/postgres:17-alpine','imagePullPolicy':'Never','args':postgres_arguments(args.postgres_tuned) + (['-c','shared_preload_libraries=pg_stat_statements,auto_explain','-c','auto_explain.log_min_duration=10ms','-c','auto_explain.sample_rate=0.01','-c','auto_explain.log_analyze=on','-c','auto_explain.log_buffers=on','-c','auto_explain.log_timing=off'] if args.postgres_explain else []),'env':[{'name':k,'value':v} for k,v in {'POSTGRES_USER':'acceptance','POSTGRES_PASSWORD':'acceptance-only','POSTGRES_DB':'acceptance'}.items()],'resources':{'requests':{'cpu':'2','memory':'2Gi'},},'ports':[{'containerPort':5432}],'readinessProbe':{'exec':{'command':['pg_isready','-U','acceptance','-d','acceptance']},'periodSeconds':2},'volumeMounts':[{'name':'data','mountPath':'/var/lib/postgresql/data'}]})
        items[-1]['spec']['template']['spec']['volumes']=[{'name':'data','emptyDir':{}}]
        if args.postgres_tuned:
            items[-1]['spec']['template']['spec']['volumes'].append({'name':'shared-memory','emptyDir':{'medium':'Memory','sizeLimit':'512Mi'}})
            items[-1]['spec']['template']['spec']['containers'][0]['volumeMounts'].append({'name':'shared-memory','mountPath':'/dev/shm'})
        (output/'postgres-profile.json').write_text(json.dumps({'tuned':args.postgres_tuned,'sampled_slow_plans':args.postgres_explain,'settings':postgres_settings(args.postgres_tuned),'cpu_memory_limits':'none','data_storage':'ephemeral emptyDir on node filesystem; not production persistence'},indent=2))
        service('postgres',5432)
        deployment('redis',{'name':'redis','image':'docker.io/library/redis:7-alpine','imagePullPolicy':'Never','resources':{'requests':{'cpu':'250m','memory':'128Mi'},'limits':{'cpu':'1','memory':'512Mi'}},'readinessProbe':{'exec':{'command':['redis-cli','ping']},'periodSeconds':2},'ports':[{'containerPort':6379}]})
        service('redis',6379)
        for name,component,port in [('acceptance-node-1','data-plane',8080),('acceptance-node-2','data-plane',8080),('control-plane','control-plane',8081)]:
            envs={'GOMAXPROCS':str(args.data_plane_cpus if component=='data-plane' else 2),'GOMEMLIMIT':'768MiB','VMCP_TENANT_ID':'default','VMCP_SKIP_MIGRATIONS':'true','VMCP_AUDIT_MODE':'strict','VMCP_EGRESS_ALLOW_CIDRS':f'{address}/32','VMCP_NODE_ID':name,'VMCP_REDIS_URL':'redis://redis:6379/0','VMCP_REDIS_FAILURE_POLICY':'strict','VMCP_HEALTH_INTERVAL':'30s'}
            if component == 'data-plane':envs['GOMEMLIMIT']=os.environ.get('VMCP_ACCEPTANCE_GOMEMLIMIT','768MiB')
            if os.environ.get('VMCP_ACCEPTANCE_DIAGNOSTICS') == '1' and component == 'data-plane':envs['GODEBUG']='gctrace=1'
            envs['VMCP_DP_ADDR' if component=='data-plane' else 'VMCP_CP_ADDR']=f':{port}'
            if component=='control-plane':envs['VMCP_ADMIN_TOKEN']='synthetic-load-admin-only'
            container={'name':component,'image':images[component],'imagePullPolicy':'Never','env':[{'name':k,'value':v} for k,v in envs.items()]+[{'name':key,'valueFrom':{'secretKeyRef':{'name':'runtime','key':ref}}} for key,ref in [('VMCP_DATABASE_URL','database-url'),('VMCP_MASTER_KEY','master-key')]],'ports':[{'containerPort':port}],'resources':{'requests':{'cpu':str(args.data_plane_cpus) if component=='data-plane' else '250m','memory':'2Gi' if component=='data-plane' else '256Mi'},'limits':{'cpu':str(max(4,args.data_plane_cpus)) if component=='data-plane' else '1','memory':args.data_plane_memory_limit if component=='data-plane' else '1Gi'}},'securityContext':{'runAsNonRoot':True,'runAsUser':65532,'readOnlyRootFilesystem':True,'allowPrivilegeEscalation':False,'capabilities':{'drop':['ALL']}},'readinessProbe':{'httpGet':{'path':'/readyz','port':port},'periodSeconds':2,'timeoutSeconds':2},'startupProbe':{'httpGet':{'path':'/healthz','port':port},'periodSeconds':2,'failureThreshold':90}}
            if args.omni:
                container['env'].append({'name':'VMCP_OMNI_SECRET','valueFrom':{'secretKeyRef':{'name':'omni','key':'secret'}}})
            deployment(name,container,0);service(name,port,component=='data-plane')
        if args.omni:
            omni_env={'VMCP_LISTEN':':8090','VMCP_OMNI_DATA_PLANE_URL':'http://acceptance-node-1:8080','VMCP_OMNI_REFRESH_INTERVAL':'2s','VMCP_OMNI_SEARCH_LIMIT':'12000','GOMAXPROCS':'2','GOMEMLIMIT':'512MiB'}
            for key in ('VMCP_OMNI_EMBEDDING_URL','VMCP_OMNI_EMBEDDING_MODEL'):
                if os.environ.get(key): omni_env[key]=os.environ[key]
            deployment('omni-mcp',{'name':'omni-mcp','image':images['omni-mcp'],'imagePullPolicy':'Never','env':[{'name':k,'value':v} for k,v in omni_env.items()]+[{'name':'VMCP_OMNI_SECRET','valueFrom':{'secretKeyRef':{'name':'omni','key':'secret'}}}],'resources':{'requests':{'cpu':'250m','memory':'128Mi'},'limits':{'cpu':'2','memory':'1Gi'}},'readinessProbe':{'httpGet':{'path':'/readyz','port':8090},'periodSeconds':2}},0)
            service('omni-mcp',8090,True)
        manifest=output/'manifest.json';manifest.write_text(json.dumps({'apiVersion':'v1','kind':'List','items':items},indent=2))
        command(kubectl+['apply','-f',str(manifest)])
        for name in ('postgres','redis'):command(kubectl+['rollout','status','deployment/'+name,'--timeout=120s'])
        with socket.socket() as connection:
            connection.bind(('127.0.0.1',0));pg_port=connection.getsockname()[1]
        port_log=(output/'port-forward.log').open('w')
        forward=subprocess.Popen(kubectl+['port-forward','service/postgres',f'{pg_port}:5432','--address=127.0.0.1'],stdout=port_log,stderr=subprocess.STDOUT)
        for _ in range(100):
            if forward.poll() is not None:raise RuntimeError('test PostgreSQL port-forward failed')
            try:
                with socket.create_connection(('127.0.0.1',pg_port),timeout=.2):break
            except OSError:time.sleep(.1)
        else:raise RuntimeError('PostgreSQL port-forward not ready')
        run_env=dict(os.environ,VMCP_ACCEPTANCE_MEMORY_LIMIT=args.data_plane_memory_limit,VMCP_RUN_ACCEPTANCE='1',VMCP_ACCEPTANCE_KUBERNETES='1',VMCP_ACCEPTANCE_NAMESPACE=namespace,VMCP_ACCEPTANCE_NODE_IP=address,VMCP_ACCEPTANCE_KUBE_NODE=node['metadata']['name'],VMCP_ACCEPTANCE_BINARY=str(binary),VMCP_ACCEPTANCE_OUTPUT=str(output),VMCP_TEST_DATABASE_URL=f'postgres://acceptance:acceptance-only@127.0.0.1:{pg_port}/acceptance?sslmode=disable',VMCP_ACCEPTANCE_REDIS_URL='redis://redis:6379/0',VMCP_ACCEPTANCE_REST='1',GOMAXPROCS='8')
        if args.omni: run_env['VMCP_ACCEPTANCE_OMNI_SECRET']=omni_secret
        defaults={'SERVERS':'300','TOOLS':'200','WORKERS':'512','RPS':'600','BURST_RPS':'2000','WARMUP_SECONDS':'30','STEADY_SECONDS':'300','SPIKE_SECONDS':'60','SOAK_RPS':'400','SOAK_SECONDS':'1800','AUDIT_CHECK_SECONDS':'180'}
        if args.database_focus:defaults.update(RPS='1000',BURST_RPS='1000')
        if args.smoke:defaults.update(SERVERS='8',TOOLS='20',RPS='20',BURST_RPS='60')
        for key,value in defaults.items():run_env['VMCP_ACCEPTANCE_'+key]=os.environ.get('VMCP_ACCEPTANCE_'+key,value)
        run_env['VMCP_ACCEPTANCE_POSTGRES_TUNED']='1' if args.postgres_tuned else '0'
        run_env['VMCP_ACCEPTANCE_POLL_PROBE']='1' if args.database_focus else '0'
        run_env['VMCP_ACCEPTANCE_DP_CPUS']=str(args.data_plane_cpus)
        run_env['VMCP_ACCEPTANCE_DATABASE_FOCUS']='1' if args.database_focus else '0'
        run_env['VMCP_ACCEPTANCE_SMOKE']='1' if args.smoke else '0'
        (output/'profile.json').write_text(json.dumps({k:v for k,v in run_env.items() if k.startswith('VMCP_ACCEPTANCE_') and k not in ('VMCP_ACCEPTANCE_BINARY','VMCP_ACCEPTANCE_REDIS_URL','VMCP_ACCEPTANCE_OMNI_SECRET')},indent=2))
        if args.database_focus:
            with (output/'migration-delivery-tests.log').open('w') as log:
                command(['go','test','-count=1','-timeout=3m','-v','-run','TestDeliveryCleanupIndexesUpgradeAndRollback|TestDeliveryBackpressureReservesTerminalAndExpiresReplay|TestDurableRoutingFencingReplayAndIsolation|TestLocalWake|TestListChangePublication|TestDuplexReplay|TestNegotiatedClientRequests|TestCuratedCatalogProtocol|TestSubscriptionNotifications','./internal/store','./internal/delivery','./internal/virtual','./internal/e2e'],cwd=ROOT/'core',env=run_env,stdout=log,stderr=subprocess.STDOUT)
            observer = DatabaseObserver(kubectl, output, node['metadata']['name'])
            observer.start()
        with (output/'run.log').open('w') as log:
            process=subprocess.Popen(['go','test','-tags=acceptance','-count=1','-timeout=75m','-run','^TestOmniAcceptance$' if args.omni else '^TestScaleAcceptance$','-v','./internal/e2e'],cwd=ROOT/'core',env=run_env,stdout=log,stderr=subprocess.STDOUT,start_new_session=True)
            print(f'Load test running in {namespace}; evidence: {output}',flush=True)
            result=process.wait()
    finally:
        if process is not None and process.poll() is None:
            os.killpg(process.pid,signal.SIGTERM)
            try:process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid,signal.SIGKILL);process.wait()
        if observer:
            observer.stop()
        if created:
            for name in ('acceptance-node-1','acceptance-node-2','control-plane','postgres','redis','omni-mcp') if args.omni else ('acceptance-node-1','acceptance-node-2','control-plane','postgres','redis'):
                with (output/f'{name}.log').open('a') as log:
                    subprocess.run(kubectl+['logs','deployment/'+name,'--all-containers=true','--tail='+('-1' if os.environ.get('VMCP_ACCEPTANCE_DIAGNOSTICS') == '1' else '3000'),'--pod-running-timeout=3s'],stdout=log,stderr=subprocess.STDOUT)
            for resource in ('pods','events'):
                with (output/f'kubernetes-{resource}.json').open('w') as file:
                    subprocess.run(kubectl+['get',resource,'-o','json'],stdout=file,stderr=subprocess.STDOUT)
            if forward:
                forward.terminate()
                try:forward.wait(timeout=5)
                except subprocess.TimeoutExpired:forward.kill();forward.wait()
            with (output/'existing-demo-after.json').open('w') as file:
                subprocess.run(['microk8s','kubectl','get','pods','-n','vmcp-demo','-o','json'],stdout=file,stderr=subprocess.STDOUT)
            subprocess.run(['microk8s','kubectl','delete','namespace',namespace,'--wait=true','--timeout=120s'],check=False)
        if (output/'report.json').exists():
            subprocess.run(['python3',str(ROOT/'benchmark/acceptance/report.py'),str(output/'report.json')],check=False)
            subprocess.run(['python3',str(ROOT/'benchmark/kubernetes/compare.py'),str(output)],check=False)
        if any(output.iterdir()):
            try:
                archive = publish(output, published, ROOT/'artifacts/benchmark-archives')
                print(f'Curated results: {published}; raw archive: {archive["filename"]}', flush=True)
            except Exception as error:
                print(f'Could not export results; raw evidence remains in {output}: {error}', flush=True)
                result = 1
        shutil.rmtree(temporary,ignore_errors=True)
    return result

if __name__=='__main__':raise SystemExit(main())
