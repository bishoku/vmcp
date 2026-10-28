"""Create-only trial fixtures. Standard library only; never reset edited resources."""
from contextlib import contextmanager
import json
import socket
import re
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request


class API:
    def __init__(self, base, token=''):
        self.base, self.token = base, token
        self.http = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())

    def request(self, method, path, body=None, missing=False, form=False):
        raw = (urllib.parse.urlencode(body).encode() if form else json.dumps(body).encode()) if body is not None else None
        headers = {'Content-Type': 'application/x-www-form-urlencoded' if form else 'application/json'}
        if self.token: headers['Authorization'] = 'Bearer ' + self.token
        try:
            with self.http.open(urllib.request.Request(self.base + path, raw, headers, method=method), timeout=30) as r:
                payload = r.read(4 * 1024 * 1024)
                return json.loads(payload) if payload else None
        except urllib.error.HTTPError as exc:
            if missing and exc.code == 404: return None
            # Only validated field paths, never provider messages or body text.
            fields = []
            if exc.code == 422:
                try:
                    fields = [e['field'] for e in json.loads(exc.read(65536)).get('errors', [])
                              if re.fullmatch(r'[A-Za-z0-9_./-]{1,100}', str(e.get('field', '')))]
                except (ValueError, TypeError, KeyError): pass
            detail = ('; invalid fields: ' + ', '.join(fields[:20])) if fields else ''
            raise ValueError(f'Demo API {method} {path.split("?")[0]} returned HTTP {exc.code}{detail}; existing resources were not reset') from None

    def items(self, path):
        result, cursor = [], ''
        while True:
            response = self.request('GET', path + ('?cursor=' + urllib.parse.quote(cursor, safe='') if cursor else ''))
            if isinstance(response, list): return response
            result.extend(response['items'])
            cursor = response.get('nextCursor')
            if not cursor: return result


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *_args, **_kwargs):
        return None


@contextmanager
def forward(installer, service, remote_port=8080, local_port=None):
    # Reserve an ephemeral loopback port. If it is taken before kubectl binds,
    # fail explicitly rather than dispatching credentials to an unknown listener.
    if local_port is None:
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            port = sock.getsockname()[1]
    else:
        port = local_port
    with __import__('tempfile').TemporaryFile(mode='w+') as log:
        process = subprocess.Popen(installer.k + ['port-forward', '--address', '127.0.0.1', 'service/' + service, f'{port}:{remote_port}'], stdout=log, stderr=log)
        try:
            for _ in range(300):
                if process.poll() is not None: raise ValueError('Demo API port-forward exited before readiness')
                log.seek(0)
                if f'Forwarding from 127.0.0.1:{port}' in log.read(): break
                time.sleep(.1)
            else: raise ValueError('Demo API port-forward did not become ready')
            yield f'http://127.0.0.1:{port}'
        finally:
            if process.poll() is None: process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=10)


def issuer(a):
    base = (('https://' if a.tls_secret else 'http://') + a.keycloak_host) if a.keycloak_host else f'http://localhost:{a.keycloak_port}'
    return base + '/realms/vmcp-demo'


def fixtures(a):
    teams = [dict(slug='demo-' + name, name='Demo ' + name.upper(), issuer=issuer(a), group='demo-' + name) for name in ['crm', 'commerce', 'support']]
    projects = [dict(slug='demo-customer-chat', name='Demo Customer Chat', ownerTeam='demo-crm'),
                dict(slug='demo-operations', name='Demo Operations', ownerTeam='demo-commerce'),
                dict(slug='demo-helpdesk', name='Demo Helpdesk', ownerTeam='demo-support')]
    upstreams = []
    for service, team, operations in [
        ('crm', 'crm', [('get_customer', 'Read the demo customer profile', {'customerId': 'C-100', 'name': 'Deniz Demo', 'tier': 'gold'}), ('list_customers', 'List demo customers', {'customers': [{'id': 'C-100', 'name': 'Deniz Demo'}]})]),
        ('orders', 'commerce', [('get_order_status', 'Track demo order fulfillment', {'orderId': 'O-100', 'status': 'shipped'}), ('list_orders', 'List demo customer orders', {'orders': [{'id': 'O-100', 'customerId': 'C-100', 'status': 'shipped'}]})]),
        ('support', 'support', [('list_tickets', 'List demo support tickets', {'tickets': [{'id': 'T-100', 'status': 'open', 'customerId': 'C-100'}]}), ('get_ticket', 'Read a demo support ticket', {'id': 'T-100', 'subject': 'Delivery question', 'status': 'open'})])]:
        ops = [dict(name=name, description=desc, enabled=True, method='GET', path='/' + name,
                    parameters=[], readOnly=True, mockRules=[dict(name='demo-success', priority=1,
                    response={'status': 200, 'jsonBody': body})]) for name, desc, body in operations]
        upstreams.append(dict(slug='demo-' + service, name='Demo ' + service.upper(), description='Quickstart playground: ' + service,
                              kind='rest', ownerTeam='demo-' + team, enabled=True,
                              url=f'http://{a.release}-trial-wiremock:8080', transport='streamable_http',
                              rest=dict(mock={}, allowInsecureHttp=True, timeoutSeconds=10, operations=ops)))
    definitions = [('customer', 'demo-customer-chat', [('crm', 'get_customer'), ('crm', 'list_customers')]),
                   ('order-tracking', 'demo-operations', [('orders', 'get_order_status'), ('orders', 'list_orders')]),
                   ('support', 'demo-helpdesk', [('support', 'list_tickets'), ('support', 'get_ticket')]),
                   ('customer-assistant', 'demo-customer-chat', [('crm', 'get_customer'), ('orders', 'get_order_status'), ('support', 'list_tickets')]),
                   ('operations', 'demo-operations', [('crm', 'list_customers'), ('orders', 'list_orders'), ('support', 'list_tickets')])]
    servers = [dict(slug='demo-' + name, name='Demo ' + name.replace('-', ' ').title(), project=project,
                    description='Quickstart playground: ' + name.replace('-', ' '), enabled=True,
                    authRequired=True, omniEnabled=a.omni,
                    tools=[dict(upstream='demo-' + u, tool=t) for u, t in tools]) for name, project, tools in definitions]
    return teams, projects, upstreams, servers


def seed(api, a, save_key, ledger=None, save_ledger=lambda _state: None):
    teams, projects, upstreams, servers = fixtures(a)
    created = 0
    ledger = {} if ledger is None else ledger
    # Lists are read fully, including pagination. Only POST missing identifiers.
    for path, desired in [('teams', teams), ('projects', projects), ('upstreams', upstreams), ('virtual-servers', servers)]:
        records = api.items('/api/v1/' + path)
        existing = {r['slug'] for r in records}
        if path == 'teams' and a.keycloak:
            for row in records:
                if row['slug'] in {t['slug'] for t in teams} and row.get('issuer') != issuer(a):
                    print('Preserved team with a different issuer: ' + row['slug'] + '; review its group mapping in the console.')
        for row in desired:
            if row['slug'] in existing: continue
            result = api.request('POST', '/api/v1/' + path, row)
            version = result.get('version') if isinstance(result, dict) else None
            if version is None:
                version = next(r['version'] for r in api.items('/api/v1/' + path) if r['slug'] == row['slug'])
            ledger[path + '/' + row['slug']] = version
            save_ledger(ledger)
            created += 1
    # Never reapprove a grant revoked/rejected by the person exploring the demo.
    grants = {(r['virtualServer'], r['upstream'], r['tool']) for r in api.items('/api/v1/tool-grants')}
    current_upstreams = {r['slug']: r for r in api.items('/api/v1/upstreams')}
    owners = {slug: row['ownerTeam'] for slug, row in current_upstreams.items()}
    current_projects = {r['slug']: r for r in api.items('/api/v1/projects')}
    current_servers = {r['slug']: r for r in api.items('/api/v1/virtual-servers')}
    for expected in servers:
        current = current_servers[expected['slug']]
        # Only finish approvals for resources whose exact creation versions were
        # recorded by this installer. Never infer ownership from a low version.
        project = current_projects.get(current.get('project'), {})
        if (current.get('version') != ledger.get('virtual-servers/' + current['slug']) or
            project.get('version') != ledger.get('projects/' + expected['project']) or
            current.get('project') != expected['project']): continue
        expected_tools = {(t['upstream'], t['tool']) for t in expected['tools']}
        for selection in current.get('tools', []):
            key = (current['slug'], selection['upstream'], selection['tool'])
            if (selection['upstream'], selection['tool']) not in expected_tools: continue
            if current_upstreams.get(selection['upstream'], {}).get('version') != ledger.get('upstreams/' + selection['upstream']): continue
            if owners.get(selection['upstream']) and owners[selection['upstream']] != current.get('ownerTeam') and key not in grants:
                api.request('POST', '/api/v1/tool-grants', dict(virtualServer=key[0], upstream=key[1], tool=key[2], status='approved', reason='Quickstart demo approval'))
    # Runtime keys are one-time API responses. Persist immediately; never rotate on rerun.
    keys = {r['slug'] for r in api.items('/api/v1/agent-keys')}
    for slug, selected in [('demo-chat-agent', ['demo-customer', 'demo-customer-assistant']),
                           ('demo-operations-agent', ['demo-order-tracking', 'demo-operations', 'demo-support'])]:
        if slug not in keys:
            result = api.request('POST', '/api/v1/agent-keys', dict(slug=slug, description='Quickstart playground runtime key', virtualServers=selected))
            save_key(slug, result['key'])
    if a.keycloak:
        existing = {r['slug'] for r in api.items('/api/v1/identity-bindings')}
        for group, selected in [('crm', ['demo-customer', 'demo-customer-assistant']),
                                ('commerce', ['demo-order-tracking', 'demo-operations']), ('support', ['demo-support'])]:
            slug = 'demo-' + group + '-identity'
            if slug not in existing:
                api.request('POST', '/api/v1/identity-bindings', dict(slug=slug, description='Quickstart demo group identity',
                    enabled=True, conditions=[dict(claim='groups', op='contains', value='demo-' + group)], virtualServers=selected))
    print(f'Playground ready: {created} missing resources created; existing resources and grant decisions preserved.')


def seed_keycloak(api, credentials, a):
    auth = api.request('POST', '/realms/master/protocol/openid-connect/token',
                       dict(client_id='admin-cli', grant_type='password', username='admin', password=credentials['keycloak-admin-password']), form=True)
    api.token = auth['access_token']
    realm = '/admin/realms/vmcp-demo'
    if api.request('GET', realm, missing=True) is None:
        api.request('POST', '/admin/realms', dict(realm='vmcp-demo', enabled=True, sslRequired='none', registrationAllowed=False))
    groups = {g['name']: g['id'] for g in api.request('GET', realm + '/groups?max=1000')}
    for name in ['demo-admins', 'demo-viewers', 'demo-crm', 'demo-commerce', 'demo-support']:
        if name not in groups:
            api.request('POST', realm + '/groups', {'name': name})
    groups = {g['name']: g['id'] for g in api.request('GET', realm + '/groups?max=1000')}
    clients = api.request('GET', realm + '/clients?clientId=vmcp-console')
    if not clients:
        scheme = 'https' if a.tls_secret else 'http'
        web = f'{scheme}://{a.web_host}' if a.web_host else f'http://localhost:{a.web_port}'
        api.request('POST', realm + '/clients', dict(clientId='vmcp-console', protocol='openid-connect', enabled=True,
            publicClient=False, secret=credentials['keycloak-client-secret'], standardFlowEnabled=True,
            directAccessGrantsEnabled=False, redirectUris=[web + '/auth/callback'], webOrigins=[web],
            attributes={'pkce.code.challenge.method': 'S256'}, protocolMappers=[dict(name='groups', protocol='openid-connect',
                protocolMapper='oidc-group-membership-mapper', config={'claim.name': 'groups', 'full.path': 'false',
                'id.token.claim': 'true', 'access.token.claim': 'true', 'userinfo.token.claim': 'true'})]))
    if not api.request('GET', realm + '/clients?clientId=vmcp-demo-agent'):
        # Trial-only password grant lets the playground exercise real group JWTs.
        api.request('POST', realm + '/clients', dict(clientId='vmcp-demo-agent', protocol='openid-connect', enabled=True,
            publicClient=True, standardFlowEnabled=False, directAccessGrantsEnabled=True,
            protocolMappers=[dict(name='groups', protocol='openid-connect', protocolMapper='oidc-group-membership-mapper',
                config={'claim.name': 'groups', 'full.path': 'false', 'access.token.claim': 'true', 'id.token.claim': 'true'}),
                dict(name='runtime-audience', protocol='openid-connect', protocolMapper='oidc-audience-mapper',
                config={'included.custom.audience': 'vmcp-runtime', 'access.token.claim': 'true', 'id.token.claim': 'false'})]))
    for username, group in [('demo-admin', 'demo-admins'), ('demo-viewer', 'demo-viewers'),
                            ('demo-crm-owner', 'demo-crm'), ('demo-commerce-owner', 'demo-commerce'), ('demo-support-owner', 'demo-support')]:
        users = api.request('GET', realm + '/users?exact=true&username=' + username)
        if users: continue
        api.request('POST', realm + '/users', dict(username=username, enabled=True, emailVerified=True,
            firstName='Demo', lastName=username.removeprefix('demo-'), email=username + '@example.test',
            credentials=[dict(type='password', value=credentials['keycloak-demo-password'], temporary=False)], groups=[group]))
    print('Keycloak demo realm ready; existing clients, users, passwords and memberships preserved.')
