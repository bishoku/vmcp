"""Create-only playground and Keycloak regression tests; no cluster mutations."""
from contextlib import redirect_stdout
import copy
import importlib.util
import io
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('quickstart_playground_test', ROOT / 'deploy/helm/vmcp/quickstart/run.py')
q = importlib.util.module_from_spec(spec)
spec.loader.exec_module(q)
p = q.playground


def args(*extra):
    a = q.parser().parse_args(['--version', 'v1.2.2', '--mock', '--omni', *extra])
    q.validate(a)
    return a


class MemoryAPI:
    def __init__(self):
        self.data = {key: [] for key in ['teams', 'projects', 'upstreams', 'virtual-servers', 'tool-grants', 'agent-keys', 'identity-bindings']}
        self.writes = []

    def items(self, path): return copy.deepcopy(self.data[path.rsplit('/', 1)[-1]])

    def request(self, method, path, body):
        assert method == 'POST'
        kind = path.rsplit('/', 1)[-1]
        row = copy.deepcopy(body)
        row['version'] = 2 if kind == 'virtual-servers' else 1
        if kind == 'virtual-servers':
            project = next(v for v in self.data['projects'] if v['slug'] == row['project'])
            row['ownerTeam'] = project['ownerTeam']
        self.data[kind].append(row)
        self.writes.append((kind, copy.deepcopy(body)))
        return dict(row, key='opaque-runtime-key') if kind == 'agent-keys' else row


class PlaygroundTests(unittest.TestCase):
    def test_create_once_and_preserve_all_edits_and_revocations(self):
        api, saved = MemoryAPI(), {}
        with redirect_stdout(io.StringIO()): p.seed(api, args(), lambda k, v: saved.update({k: v}))
        self.assertEqual([len(api.data[k]) for k in ['teams', 'projects', 'upstreams', 'virtual-servers', 'agent-keys']], [30, 40, 20, 40, 10])
        owners = {u['slug']: u['ownerTeam'] for u in api.data['upstreams']}
        cross = {(s['slug'], t['upstream'], t['tool']) for s in api.data['virtual-servers'] for t in s['tools'] if owners[t['upstream']] != s['ownerTeam']}
        self.assertEqual({(g['virtualServer'], g['upstream'], g['tool']) for g in api.data['tool-grants']}, cross)
        decisions = {(g['virtualServer'], g['tool']): g['status'] for g in api.data['tool-grants']}
        self.assertEqual(decisions.pop(('genai-sandbox', 'initiate_domestic_payment')), 'pending')
        self.assertEqual(decisions.pop(('genai-sandbox', 'screen_party')), 'rejected')
        self.assertEqual(set(decisions.values()), {'approved'})
        self.assertEqual(set(saved), {k[0] for k in p.catalog.AGENT_KEYS})
        server = next(s for s in api.data['virtual-servers'] if s['slug'] == 'payments-copilot')
        self.assertIn('validate_payee_name', server['guidance']['instructions'])
        api.data['upstreams'][0].update(name='My edited name', version=2)
        api.data['virtual-servers'][0].update(omniEnabled=False, enabled=False, version=3)
        api.data['teams'][0].update(group='custom-group', version=2)
        api.data['agent-keys'][0].update(virtualServers=[], version=2)
        api.data['tool-grants'][0].update(status='revoked', version=2)
        before = copy.deepcopy(api.data)
        api.writes.clear()
        with redirect_stdout(io.StringIO()): p.seed(api, args(), lambda *_: self.fail('Key rotated on rerun'))
        self.assertEqual(api.data, before)
        self.assertEqual(api.writes, [])

    def test_interrupted_seed_uses_recorded_versions_and_never_approves_edited_sources(self):
        api, ledger = MemoryAPI(), {}
        with redirect_stdout(io.StringIO()): p.seed(api, args(), lambda *_: None, ledger)
        self.assertEqual(ledger['virtual-servers/payments-copilot'], 2)
        api.data['tool-grants'].clear()
        # Simulate an interruption before grants, then a user editing an upstream.
        next(r for r in api.data['upstreams'] if r['slug'] == 'payments-hub')['version'] += 1
        with redirect_stdout(io.StringIO()): p.seed(api, args(), lambda *_: None, ledger)
        self.assertFalse(any(g['upstream'] == 'payments-hub' for g in api.data['tool-grants']))
        self.assertTrue(any(g['upstream'] == 'card-management' for g in api.data['tool-grants']))

    def test_group_identity_bindings_are_create_only(self):
        api = MemoryAPI()
        with redirect_stdout(io.StringIO()): p.seed(api, args('--keycloak'), lambda *_: None)
        bindings = {b['slug']: b for b in api.data['identity-bindings']}
        self.assertEqual(len(bindings), len({p.catalog.server_team(s) for s in p.catalog.SERVERS}))
        binding = bindings['retail-digital-banking-sso']
        self.assertEqual(binding['conditions'], [{'claim': 'groups', 'op': 'contains', 'value': 'retail-digital-banking'}])
        self.assertEqual(binding['virtualServers'], ['retail-banking-assistant', 'payments-copilot'])
        binding.update(enabled=False, virtualServers=[], version=2)
        before = copy.deepcopy(api.data)
        api.writes.clear()
        with redirect_stdout(io.StringIO()): p.seed(api, args('--keycloak'), lambda *_: None)
        self.assertEqual(api.data, before)
        self.assertEqual(api.writes, [])

    def test_partial_seed_finishes_without_replacing_existing_resources(self):
        api = MemoryAPI()
        first = p.fixtures(args())[0][0]
        api.data['teams'].append(dict(first, name='Preserved', version=8))
        with redirect_stdout(io.StringIO()): p.seed(api, args(), lambda *_: None)
        self.assertEqual(api.data['teams'][0]['name'], 'Preserved')
        self.assertFalse(any(kind == 'teams' and row['slug'] == first['slug'] for kind, row in api.writes))

    def test_optional_infrastructure_and_oidc_configuration(self):
        base = q.parser().parse_args(['--version', 'v1.2.2'])
        q.validate(base)
        self.assertFalse(any(r['metadata']['name'].endswith(('wiremock', 'keycloak')) for r in q.infrastructure(base)))
        a = args('--keycloak', '--web-host', 'vmcp.kube.local', '--gateway-host', 'mcp.kube.local', '--keycloak-host', 'sso.kube.local')
        resources = q.infrastructure(a)
        kc = next(r for r in resources if r['kind'] == 'Deployment' and r['metadata']['name'].endswith('keycloak'))
        container = kc['spec']['template']['spec']['containers'][0]
        self.assertIn('@sha256:', container['image'])
        env = {e['name']: e for e in container['env']}
        self.assertEqual(env['KC_DB_URL']['value'], 'jdbc:postgresql://vmcp-demo-trial-postgres:5432/keycloak')
        self.assertEqual(env['KC_DB_PASSWORD']['valueFrom']['secretKeyRef']['key'], 'keycloak-db-password')
        self.assertEqual(env['KC_HOSTNAME_BACKCHANNEL_DYNAMIC']['value'], 'true')
        script = kc['spec']['template']['spec']['initContainers'][0]['command'][2]
        self.assertNotIn('ALTER ROLE', script)
        self.assertIn('WHERE NOT EXISTS', script)
        refs = q.image_values(json.loads((ROOT / 'tests/helm/fixtures/v1.2.2-images.json').read_text()), a.version)
        cfg = q.values(a, refs)
        self.assertEqual(cfg['runtime']['oidc']['issuer'], 'http://sso.kube.local/realms/vmcp-demo')
        self.assertEqual(cfg['runtime']['oidc']['discoveryURL'], 'http://vmcp-demo-trial-keycloak:8080/realms/vmcp-demo')
        self.assertEqual(cfg['runtime']['oidc']['redirectURL'], 'http://vmcp.kube.local/auth/callback')
        self.assertEqual(cfg['controlPlane']['env']['VMCP_WIREMOCK_URL'], 'http://vmcp-demo-trial-wiremock:8080')
        self.assertEqual(cfg['runtime']['oidc']['editorGroups'], [])
        self.assertEqual(p.fixtures(a)[0][0]['issuer'], cfg['runtime']['oidc']['issuer'])
        for options in [['--keycloak', '--web-host', 'vmcp.kube.local', '--gateway-host', 'mcp.kube.local'],
                        ['--keycloak', '--keycloak-port', '8802'], ['--keycloak', '--keycloak-host', 'https://bad']]:
            with self.subTest(options=options), self.assertRaises(ValueError): args(*options)

    def test_external_https_new_install_urls_and_keycloak_client(self):
        from unittest.mock import Mock
        hosts = ['--keycloak', '--web-host', 'console.example.com',
                 '--gateway-host', 'mcp.example.com', '--keycloak-host', 'sso.example.com']
        for mode, scheme in [([], 'http'), (['--external-https'], 'https'),
                             (['--tls-secret', 'demo-tls'], 'https')]:
            with self.subTest(mode=mode):
                a = args(*hosts, *mode)
                refs = q.image_values(json.loads((ROOT / 'tests/helm/fixtures/v1.2.2-images.json').read_text()), a.version)
                cfg = q.values(a, refs)
                issuer = scheme + '://sso.example.com/realms/vmcp-demo'
                self.assertEqual(cfg['runtime']['oidc']['issuer'], issuer)
                self.assertEqual(cfg['runtime']['agentOIDC']['issuer'], issuer)
                self.assertEqual(cfg['runtime']['oidc']['redirectURL'], scheme + '://console.example.com/auth/callback')
                self.assertEqual(cfg['runtime']['allowedOrigins'], [scheme + '://console.example.com'])
                self.assertEqual(cfg['runtime']['publicGatewayURL'], scheme + '://mcp.example.com')
                self.assertEqual(cfg['omni']['publicURL'], scheme + '://mcp.example.com')
                self.assertEqual(cfg['runtime']['oidc']['discoveryURL'], 'http://vmcp-demo-trial-keycloak:8080/realms/vmcp-demo')
                self.assertEqual({t['issuer'] for t in p.fixtures(a)[0]}, {issuer})
                resources = q.infrastructure(a)
                kc = next(r for r in resources if r['kind'] == 'Deployment' and r['metadata']['name'].endswith('keycloak'))
                env = {e['name']: e.get('value') for e in kc['spec']['template']['spec']['containers'][0]['env']}
                self.assertEqual(env['KC_HOSTNAME'], scheme + '://sso.example.com')
                self.assertEqual(env['KC_HTTP_ENABLED'], 'true')
                self.assertEqual(env['KC_PROXY_HEADERS'], 'xforwarded')
                ingress = next(r for r in resources if r['kind'] == 'Ingress')
                self.assertEqual(bool(ingress['spec'].get('tls')), bool(a.tls_secret))
                self.assertEqual(bool(cfg['ingress']['tls']), bool(a.tls_secret))
                if a.external_https:
                    self.assertEqual(ingress['metadata']['annotations']['nginx.ingress.kubernetes.io/ssl-redirect'], 'false')
                    self.assertEqual(cfg['ingress']['annotations']['nginx.ingress.kubernetes.io/ssl-redirect'], 'false')
                api, posts = Mock(), []
                def request(method, path, body=None, **kwargs):
                    if path.endswith('/token'): return {'access_token': 'not-logged'}
                    if method == 'GET': return {'realm': 'vmcp-demo'} if path == '/admin/realms/vmcp-demo' else []
                    posts.append((path, body))
                api.request.side_effect = request
                with redirect_stdout(io.StringIO()):
                    p.seed_keycloak(api, {'keycloak-admin-password': 'a', 'keycloak-client-secret': 'b', 'keycloak-demo-password': 'c'}, a)
                client = next(body for path, body in posts if path.endswith('/clients') and body['clientId'] == 'vmcp-console')
                self.assertEqual(client['redirectUris'], [cfg['runtime']['oidc']['redirectURL']])
                self.assertEqual(client['webOrigins'], cfg['runtime']['allowedOrigins'])

    def test_external_https_requires_hosts_and_a_single_tls_mode(self):
        for options in [[], ['--web-host', 'console.example.com'],
                        ['--web-host', 'console.example.com', '--gateway-host', 'mcp.example.com', '--tls-secret', 'tls']]:
            with self.subTest(options=options), self.assertRaises(ValueError):
                args('--external-https', *options)

    def test_keycloak_first_run_creates_catalog_groups_and_users(self):
        from unittest.mock import Mock
        api, posts = Mock(), []
        def request(method, path, body=None, **kwargs):
            if path.endswith('/token'): return {'access_token': 'not-logged'}
            if method == 'GET': return {'realm': 'vmcp-demo'} if path == '/admin/realms/vmcp-demo' else []
            posts.append((path, body))
        api.request.side_effect = request
        with redirect_stdout(io.StringIO()):
            p.seed_keycloak(api, {'keycloak-admin-password': 'a', 'keycloak-client-secret': 'b', 'keycloak-demo-password': 'shared'}, args('--keycloak'))
        groups = [b['name'] for path, b in posts if path.endswith('/groups')]
        self.assertEqual(groups, [p.catalog.ADMIN_GROUP, p.catalog.VIEWER_GROUP] + list(p.catalog.TEAM))
        users = {b['username']: b for path, b in posts if path.endswith('/users')}
        self.assertEqual(len(users), len(p.catalog.USERS))
        self.assertEqual(users['platform.admin']['groups'], [p.catalog.ADMIN_GROUP])
        member = p.catalog.team_members('fraud-strategy')[0]
        self.assertEqual(users[member['username']]['groups'], ['fraud-strategy'])
        self.assertEqual(users[member['username']]['email'], member['email'])
        self.assertTrue(all(u['credentials'][0]['value'] == 'shared' for u in users.values()))

    def test_keycloak_rerun_never_updates_users_groups_or_client(self):
        from unittest.mock import Mock
        api = Mock()
        groups = [dict(name=name, id=name) for name in [p.catalog.ADMIN_GROUP, p.catalog.VIEWER_GROUP] + list(p.catalog.TEAM)]
        def request(method, path, body=None, **kwargs):
            if path.endswith('/token'): return {'access_token': 'not-logged'}
            if path.endswith('/groups?max=1000'): return groups
            if '/clients?' in path: return [{'id': 'edited-client', 'clientId': 'vmcp-console'}]
            if '/users?' in path: return [{'id': 'edited-user'}]
            if path == '/admin/realms/vmcp-demo': return {'realm': 'vmcp-demo'}
            self.fail('Unexpected Keycloak mutation: ' + method + ' ' + path)
        api.request.side_effect = request
        output = io.StringIO()
        with redirect_stdout(output): p.seed_keycloak(api, {'keycloak-admin-password': 'private'}, args('--keycloak'))
        self.assertEqual([call.args[0] for call in api.request.call_args_list].count('POST'), 1)
        self.assertNotIn('private', output.getvalue())

    def test_existing_keycloak_credentials_and_runtime_keys_are_retained(self):
        a = args('--keycloak')
        installer = q.Installer(a)
        writes = []
        installer.run = lambda cmd, data=None, capture=False: writes.append(json.loads(data)) if data else ''
        original = installer.secret()
        stored = writes[-1]
        stored['metadata']['resourceVersion'] = '5'
        import base64
        stored['data']['demo-chat-agent'] = base64.b64encode(b'preserved-runtime-key').decode()
        def run(cmd, data=None, capture=False):
            if 'get' in cmd: return json.dumps(stored)
            writes.append(json.loads(data))
            return ''
        installer.run = run
        again = installer.secret()
        self.assertTrue(all(again[k] == v for k, v in original.items()))
        self.assertEqual(again['demo-chat-agent'], 'preserved-runtime-key')
        self.assertNotIn(original['keycloak-client-secret'], json.dumps(q.values(a, {'web':'w','control-plane':'c','data-plane':'d','migrate':'m','omni-mcp':'o'})))

    def test_api_diagnostic_never_contains_response_or_credentials(self):
        from unittest.mock import patch
        import urllib.error
        api = p.API('http://localhost', 'private-key')
        error = urllib.error.HTTPError('http://localhost', 400, 'private-key', {}, io.BytesIO(b'private-password'))
        with patch.object(api.http, 'open', side_effect=error), self.assertRaises(ValueError) as caught:
            api.request('POST', '/test', {'secret': 'private-password'})
        self.assertNotIn('private-', str(caught.exception))

    def test_pagination_reads_all_pages(self):
        api = p.API('http://localhost')
        from unittest.mock import patch
        with patch.object(api, 'request', side_effect=[{'items': [{'slug': 'a'}], 'nextCursor': 'x/y'}, {'items': [{'slug': 'b'}]}]) as request:
            self.assertEqual([r['slug'] for r in api.items('/api/v1/upstreams')], ['a', 'b'])
            self.assertEqual(request.call_args_list[1].args[1], '/api/v1/upstreams?cursor=x%2Fy')


if __name__ == '__main__': unittest.main()
