#!/usr/bin/env python3
"""Isolated Compose quickstart for invocation security, built from this checkout."""
import argparse, json, os, pathlib, secrets, subprocess, time, ssl, urllib.request, urllib.error
from realm import realm
ROOT=pathlib.Path(__file__).resolve().parents[5]
HERE=pathlib.Path(__file__).resolve().parent

def request(method,path,body=None,version=None):
    data=None if body is None else json.dumps(body).encode()
    headers={'Authorization':'Bearer demo-admin-token','Content-Type':'application/json'}
    if version is not None:headers['If-Match']=str(version)
    r=urllib.request.Request('http://localhost:8841/api/v1'+path,data,headers,method=method)
    with urllib.request.urlopen(r,timeout=10) as response:
        raw=response.read();return json.loads(raw) if raw else None

def generate(directory):
    directory.mkdir(parents=True,exist_ok=True)
    secret_file=directory/'approval-secret'
    if not secret_file.exists(): secret_file.write_text(secrets.token_hex(32));secret_file.chmod(0o600)
    secret=secret_file.read_text()
    cert,key=directory/'cert.pem',directory/'key.pem'
    if not cert.exists():
        subprocess.run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','30','-subj','/CN=localhost',
                        '-addext','subjectAltName=DNS:localhost,DNS:security-demo','-addext','keyUsage=critical,digitalSignature,keyEncipherment,keyCertSign','-addext','extendedKeyUsage=serverAuth','-keyout',str(key),'-out',str(cert)],check=True,capture_output=True)
        # Isolated development certificate, readable by the non-root image.
        key.chmod(0o644)
    (directory/'realm.json').write_text(json.dumps(realm(),indent=2))
    build=lambda service:dict(context=str(ROOT/'core'),args={'SERVICE':service})
    database='postgres://vmcp:vmcp@postgres:5432/vmcp?sslmode=disable'
    common={'VMCP_DATABASE_URL':database,'VMCP_MASTER_KEY':'OsOj5XCz4oi+W3+L3o0am9uZGfYkNmsXHMu5uCDEP04=',
            'VMCP_EGRESS_ALLOW_HOSTS':'keycloak,security-demo,localhost','VMCP_EGRESS_ALLOW_CIDRS':'127.0.0.0/8,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16'}
    issuer='http://localhost:8843/realms/vmcp-security'
    services={
      'postgres':dict(image='postgres:17-alpine@sha256:eee6c6e1b0894fa4fe0075eb054af9fd1cf39a35321c2d4110626cfe506411fb',environment={'POSTGRES_USER':'vmcp','POSTGRES_PASSWORD':'vmcp','POSTGRES_DB':'vmcp'},
                      volumes=['database:/var/lib/postgresql/data'],healthcheck={'test':['CMD-SHELL','pg_isready -U vmcp'],'interval':'2s','retries':30}),
      'keycloak':dict(image='quay.io/keycloak/keycloak@sha256:9409c59bdfb65dbffa20b11e6f18b8abb9281d480c7ca402f51ed3d5977e6007',command=['start-dev','--import-realm'],
                      environment={'KC_HOSTNAME':'http://localhost:8843','KC_HOSTNAME_BACKCHANNEL_DYNAMIC':'true'},
                      ports=['127.0.0.1:8843:8080'],volumes=[str(directory/'realm.json')+':/opt/keycloak/data/import/realm.json:ro']),
      'control-plane':dict(build=build('control-plane'),environment={**common,'VMCP_ADMIN_TOKEN':'demo-admin-token','VMCP_CP_ADDR':':8081'},
                           ports=['127.0.0.1:8841:8081'],depends_on={'postgres':{'condition':'service_healthy'}}),
      'data-plane':dict(build=build('data-plane'),restart='on-failure',environment={**common,'VMCP_DP_ADDR':':8080',
                        'VMCP_AGENT_OIDC_ISSUER':issuer,'VMCP_AGENT_OIDC_DISCOVERY_URL':'http://keycloak:8080/realms/vmcp-security',
                        'VMCP_AGENT_OIDC_AUDIENCE':'vmcp-runtime'},ports=['127.0.0.1:8840:8080'],depends_on=['control-plane','keycloak']),
      'security-demo':dict(build=build('security-demo'),restart='on-failure',environment={**common,'DEMO_ISSUER':issuer,
                           'DEMO_DISCOVERY_URL':'http://keycloak:8080/realms/vmcp-security','DEMO_ORIGIN':'https://localhost:8844',
                           'DEMO_APPROVAL_SECRET':secret,'DEMO_AGENT_KEY_FILE':'/tls/agent-key','DEMO_GATEWAY':'http://data-plane:8080','DEMO_TLS_CERT':'/tls/cert.pem','DEMO_TLS_KEY':'/tls/key.pem'},
                           ports=['127.0.0.1:8844:8443'],volumes=[str(directory)+':/tls:ro'],depends_on=['keycloak'])}
    compose=directory/'compose.json';compose.write_text(json.dumps({'name':'vmcp-security-demo','services':services,'volumes':{'database':{}}},indent=2));compose.chmod(0o600)
    return compose,secret,cert.read_text()

def wait_for_demo(cert, attempts=120):
    # Discovery must not start while Keycloak/verifier initialization is pending.
    context=ssl.create_default_context(cafile=str(cert))
    for _ in range(attempts):
        try:
            with urllib.request.urlopen('https://localhost:8844/config',context=context,timeout=2) as response:
                if response.status==200:return
        except (OSError,ValueError):pass
        time.sleep(2)
    raise RuntimeError('Demo identity and approval service did not become ready')

def seed(secret,ca,directory):
    tools=['echo','step_up','delete_records','export_records','execution_counts']
    existing=request('GET','/upstreams')['items']
    if not any(u['slug']=='security-demo' for u in existing):
        request('POST','/upstreams',dict(slug='security-demo',name='Security demo',url='https://security-demo:8443/backend',transport='streamable_http',enabled=True,headers={'Authorization':'Bearer '+secret},tls={'caPem':ca}))
    request('POST','/upstreams/security-demo/refresh-tools',{})
    if not any(v['slug']=='security-demo' for v in request('GET','/virtual-servers')['items']):
        request('POST','/virtual-servers',dict(slug='security-demo',name='Security demo',enabled=True,authRequired=True,tools=[dict(upstream='security-demo',tool=t) for t in tools]))
    if not any(b['slug']=='security-demo' for b in request('GET','/identity-bindings')['items']):
        request('POST','/identity-bindings',dict(slug='security-demo',enabled=True,conditions=[dict(claim='azp',op='equals',value='vmcp-security-demo')],virtualServers=['security-demo']))
    scopes=request('GET','/access/scopes?kind=upstream');scopes=scopes.get('items',[]) if isinstance(scopes,dict) else scopes
    upstream=next(s['id'] for s in scopes if s['slug']=='security-demo')
    settings=request('GET','/invocation-security')
    changed=False
    if upstream not in settings['upstreams'] or 'identity' not in settings['upstreams'][upstream]:
        settings['services']['demo']=dict(url='https://security-demo:8443/check',token=secret,approvalOrigin='https://localhost:8844',caPem=ca)
        settings['upstreams'][upstream]={'identity':{'source':'oauth_subject'},'tools':{
            'step_up':{'stepUp':{'acceptedAcrValues':['2'],'maxAgeSeconds':60}},
            'delete_records':{'approvalServices':['demo'],'shareFields':['dataset','count']},
            'export_records':{'stepUp':{'acceptedAcrValues':['2'],'maxAgeSeconds':60},'approvalServices':['demo'],'shareFields':['dataset','count']}}}
        changed=True
    if not any(u['slug']=='security-context' for u in request('GET','/upstreams')['items']):
        request('POST','/upstreams',dict(slug='security-context',name='Shared application / customer context',url='https://security-demo:8443/backend',transport='streamable_http',enabled=True,headers={'Authorization':'Bearer '+secret},tls={'caPem':ca},callerContext={'enabled':True,'metaKey':'demo/customer','fields':[{'name':'customerId','required':True},{'name':'sessionId','required':True}]}))
    request('POST','/upstreams/security-context/refresh-tools',{})
    if not any(v['slug']=='security-context' for v in request('GET','/virtual-servers')['items']):
        request('POST','/virtual-servers',dict(slug='security-context',name='Shared application / customer context',enabled=True,authRequired=True,tools=[dict(upstream='security-context',tool=t) for t in tools]))
    key_file=directory/'agent-key'
    if not key_file.exists():
        keys=request('GET','/agent-keys')['items']
        if any(k['slug']=='security-chat-application' for k in keys):
            raise RuntimeError('Application agent key exists but local receipt is missing; recover it or explicitly rotate it before rerunning')
        created=request('POST','/agent-keys',dict(slug='security-chat-application',description='Shared demo chat application identity',virtualServers=['security-context']))
        key_file.write_text(created['key']);key_file.chmod(0o644)
    scopes=request('GET','/access/scopes?kind=upstream')['items']
    context_id=next(s['id'] for s in scopes if s['slug']=='security-context')
    if context_id not in settings['upstreams']:
        settings['upstreams'][context_id]={'identity':{'source':'caller_context','service':'demo','fields':['customerId','sessionId']},'tools':{
            'step_up':{'stepUp':{'acceptedAcrValues':['2'],'maxAgeSeconds':60}},
            'delete_records':{'approvalServices':['demo'],'shareFields':['dataset','count']},
            'export_records':{'stepUp':{'acceptedAcrValues':['2'],'maxAgeSeconds':60},'approvalServices':['demo'],'shareFields':['dataset','count']}}}
        changed=True
    if changed:request('PUT','/invocation-security',settings)
    for slug in ['security-demo','security-context']:
        server=request('GET','/virtual-servers/'+slug)
        if not server.get('catalog'):
            request('PATCH','/virtual-servers/'+slug,{'catalog':[{'upstream':slug,'kind':'resource','name':'demo://guide'},{'upstream':slug,'kind':'prompt','name':'review'}]},version=server['version'])

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--directory',type=pathlib.Path,default=ROOT/'local/security-demo');p.add_argument('--generate-only',action='store_true');a=p.parse_args()
    compose,secret,ca=generate(a.directory.resolve())
    if a.generate_only: print(compose);return
    subprocess.run(['docker','compose','-f',str(compose),'up','-d','--build'],check=True)
    for _ in range(120):
        try: request('GET','/info');break
        except (OSError,ValueError):time.sleep(2)
    else:raise RuntimeError('Control plane did not become ready')
    wait_for_demo(a.directory.resolve()/'cert.pem')
    for _ in range(60):
        try:seed(secret,ca,a.directory.resolve());break
        except urllib.error.HTTPError as e:
            if e.code not in (502,503):raise
            time.sleep(2)
    else:raise RuntimeError('Demo upstream did not become ready')
    print('Open https://localhost:8844 and trust the generated local certificate.\nUsers: alice / bob; password: demo-password.\nOTP enrollment is requested at the first step-up. This stack uses development credentials only.')
if __name__=='__main__':main()
