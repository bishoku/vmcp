"""Banking playground catalog consistency and demo.sh helper tests; no cluster access."""
from contextlib import redirect_stdout
import base64
import importlib.util
import io
import json
from pathlib import Path
import re
import sys
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
QUICKSTART = ROOT / 'deploy/helm/vmcp/quickstart'
if str(QUICKSTART) not in sys.path: sys.path.insert(0, str(QUICKSTART))
import demo_catalog as catalog  # noqa: E402
import demo_data as data  # noqa: E402
import demo_upstreams as upstreams  # noqa: E402

spec = importlib.util.spec_from_file_location('quickstart_demo_test', QUICKSTART / 'demo.py')
demo = importlib.util.module_from_spec(spec)
spec.loader.exec_module(demo)

SLUG = re.compile(r'^[a-z0-9]([a-z0-9-]*[a-z0-9])?$')
TYPES = {'string': str, 'boolean': bool, 'array': list, 'object': dict, 'null': type(None)}


def conforms(value, schema, path='$'):
    """Small JSON Schema subset used by the inferred response schemas."""
    kinds = schema.get('type')
    kinds = kinds if isinstance(kinds, list) else [kinds]
    def matches(kind):
        if kind == 'integer': return isinstance(value, int) and not isinstance(value, bool)
        if kind == 'number': return isinstance(value, (int, float)) and not isinstance(value, bool)
        return isinstance(value, TYPES[kind])
    if not any(matches(k) for k in kinds): return [f'{path}: {type(value).__name__} not in {kinds}']
    problems = []
    if isinstance(value, dict):
        problems += [f'{path}.{k}: required' for k in schema.get('required', []) if k not in value]
        for k, v in value.items():
            if k not in schema.get('properties', {}): problems.append(f'{path}.{k}: undocumented')
            else: problems += conforms(v, schema['properties'][k], path + '.' + k)
    if isinstance(value, list) and 'items' in schema:
        for i, item in enumerate(value): problems += conforms(item, schema['items'], f'{path}[{i}]')
    return problems


class CatalogTests(unittest.TestCase):
    def test_counts_and_unique_slugs(self):
        self.assertEqual([len(catalog.TEAMS), len(catalog.PROJECTS), len(upstreams.UPSTREAMS), len(catalog.SERVERS)], [30, 40, 20, 40])
        for kind, slugs in [('team', [t[0] for t in catalog.TEAMS]), ('project', [p[0] for p in catalog.PROJECTS]),
                            ('upstream', [u['slug'] for u in upstreams.UPSTREAMS]), ('server', [s['slug'] for s in catalog.SERVERS]),
                            ('key', [k[0] for k in catalog.AGENT_KEYS])]:
            with self.subTest(kind=kind):
                self.assertEqual(len(slugs), len(set(slugs)))
                self.assertTrue(all(SLUG.match(s) and len(s) <= 63 for s in slugs))

    def test_users_match_teams_and_keycloak_groups(self):
        usernames = [u['username'] for u in catalog.USERS]
        self.assertEqual(len(usernames), len(set(usernames)))
        self.assertEqual(len({u['email'] for u in catalog.USERS}), len(catalog.USERS))
        for team in catalog.TEAM:
            with self.subTest(team=team):
                members = catalog.team_members(team)
                self.assertIn(len(members), (3, 4))
                self.assertTrue(all(u['groups'] == [team] for u in members))
        admins = [u for u in catalog.USERS if catalog.ADMIN_GROUP in u['groups']]
        viewers = [u for u in catalog.USERS if catalog.VIEWER_GROUP in u['groups']]
        self.assertEqual([u['username'] for u in admins + viewers], ['platform.admin', 'security.auditor'])
        self.assertTrue(all(re.fullmatch(r'[a-z]+\.[a-z]+', u) for u in usernames))

    def test_references_are_consistent(self):
        for slug, _name, owner, _purpose in catalog.PROJECTS: self.assertIn(owner, catalog.TEAM, slug)
        for u in upstreams.UPSTREAMS: self.assertIn(u['ownerTeam'], catalog.TEAM, u['slug'])
        self.assertEqual(set(catalog.TEAM), {p[2] for p in catalog.PROJECTS}, 'every team owns a project')
        tools = [o['name'] for u in upstreams.UPSTREAMS for o in u['operations']]
        self.assertEqual(len(tools), len(set(tools)), 'tool names must be globally unique')
        used = {t for s in catalog.SERVERS for t in s['tools']}
        self.assertEqual(used, set(tools), 'every tool is exposed by a virtual server')
        for s in catalog.SERVERS:
            with self.subTest(server=s['slug']):
                self.assertIn(s['project'], catalog.PROJECT)
                self.assertLessEqual(len(s['name']), 120)
                self.assertLessEqual(len(s['description']), 1000)
                self.assertLessEqual(len(s['instructions']), 4000)
                self.assertEqual(len(s['tools']), len(set(s['tools'])))
                self.assertTrue(set(s.get('grants', {})) <= set(s['tools']))
                self.assertTrue(set(s.get('grants', {}).values()) <= {'pending', 'rejected'})
        covered = [s for _slug, _desc, servers in catalog.AGENT_KEYS for s in servers]
        self.assertEqual(sorted(covered), sorted(catalog.SERVER), 'each server belongs to exactly one agent key')
        for team, member in data.STAFF_REFS.values(): self.assertLess(member, len(catalog.team_members(team)))

    def test_english_only(self):
        text = json.dumps([catalog.TEAMS, catalog.PROJECTS, catalog.SERVERS, catalog.USERS, catalog.AGENT_KEYS, upstreams.UPSTREAMS], ensure_ascii=False)
        self.assertIsNone(re.search('[çğıöşüÇĞİÖŞÜ]', text))

    def test_operations_are_documented_and_mock_bodies_match_schemas(self):
        for u in upstreams.UPSTREAMS:
            self.assertLessEqual(len(json.dumps(u)), 400 * 1024, u['slug'])
            self.assertLessEqual(sum(len(o['mockRules']) for o in u['operations']), 1000)
            for o in u['operations']:
                with self.subTest(tool=o['name']):
                    self.assertGreater(len(o['description']), 40)
                    self.assertNotIn('{', o['path'], 'query parameters only')
                    self.assertEqual(o['readOnly'], o['method'] == 'GET')
                    self.assertLessEqual(len(o['mockRules']), 20)
                    self.assertTrue(all(p['schema'].get('description') for p in o['parameters']))
                    for prop in o.get('bodySchema', {}).get('properties', {}).values(): self.assertTrue(prop.get('description'))
                    names = [r['name'] for r in o['mockRules']]
                    self.assertEqual(len(names), len(set(names)))
                    for rule in o['mockRules']:
                        response = rule['response']
                        if 'jsonBody' in response:
                            self.assertNotIn('{{', json.dumps(response['jsonBody']))
                            if response['status'] < 300:
                                self.assertEqual(conforms(response['jsonBody'], o['responseSchema']), [], rule['name'])
                        if 'body' in response:
                            # Quoted templates render to strings, unquoted ones to numbers.
                            text = re.sub(r'\{\{[^}]*\}\}', 'TPL', response['body'])
                            json.loads(re.sub(r'(?<!")TPL(?!")', '1', text).replace('TPL', 'X'))

    def test_seeded_virtual_server_tools_resolve_to_upstreams(self):
        a = type('Args', (), dict(release='vmcp-demo', omni=False, keycloak_host=None, tls_secret=None, keycloak_port=8803))()
        sys.modules.setdefault('demo_catalog', catalog)
        import playground
        teams, projects, ups, servers = playground.fixtures(a)
        operations = {(u['slug'], o['name']) for u in ups for o in u['rest']['operations']}
        for s in servers:
            for t in s['tools']: self.assertIn((t['upstream'], t['tool']), operations)
        self.assertTrue(all(t['group'] == t['slug'] for t in teams))
        self.assertEqual({p['ownerTeam'] for p in projects}, {t['slug'] for t in teams})


class DemoHelperTests(unittest.TestCase):
    def run_demo(self, *argv, secret=None):
        stored = {'metadata': {'labels': {'vmcp.quickstart': 'vmcp-demo'}},
                  'data': {k: base64.b64encode(v.encode()).decode() for k, v in (secret or {}).items()}}
        calls = []
        def kubectl(cluster, *args):
            calls.append(args)
            if args[:2] == ('get', 'secret'): return json.dumps(stored)
            if args[:2] == ('get', 'ingress'): return json.dumps({'items': []})
            raise AssertionError(args)
        output = io.StringIO()
        with patch.object(demo.Cluster, 'kubectl', kubectl), redirect_stdout(output):
            demo.main(list(argv))
        return output.getvalue(), calls

    def test_catalog_commands_work_without_cluster(self):
        for argv in [['users'], ['teams'], ['projects'], ['servers'], ['upstreams'], ['team', 'fraud-strategy'], ['users', '--team', 'kyc-operations']]:
            with self.subTest(argv=argv):
                output, calls = self.run_demo(*argv)
                self.assertEqual(calls, [])
                self.assertTrue(output)
        output, _ = self.run_demo('--json', 'users')
        self.assertEqual(len(json.loads(output)), len(catalog.USERS))

    def test_password_and_keys_are_read_from_quickstart_secret(self):
        secret = {'keycloak-demo-password': 'pw-123', 'admin-token': 'admin-xyz', 'keycloak-admin-password': 'kc-admin', 'fraud-ops-agent': 'vmcp_key_1'}
        self.assertEqual(self.run_demo('password', secret=secret)[0], 'pw-123\n')
        self.assertEqual(self.run_demo('key', 'fraud-ops-agent', secret=secret)[0], 'vmcp_key_1\n')
        self.assertIn('pw-123', self.run_demo('user', 'oliver.harrison', secret=secret)[0])
        output, calls = self.run_demo('credentials', '--namespace', 'other', secret=secret)
        for value in secret.values(): self.assertIn(value, output)
        self.assertIn('http://localhost:3300', output)
        self.assertEqual(len([c for c in calls if c[:2] == ('get', 'secret')]), 1)
        self.assertEqual(json.loads(self.run_demo('server', 'fraud-triage', '--json')[0])['endpoint'],
                         'http://localhost:8801/mcp/v1/virtual/fraud-triage')

    def test_rejects_unknown_names_and_foreign_secrets(self):
        with self.assertRaises(SystemExit): self.run_demo('user', 'nobody')
        with self.assertRaises(SystemExit): self.run_demo('team', 'fraud')
        cluster = demo.Cluster(demo.parser().parse_args(['password']))
        with patch.object(demo.Cluster, 'kubectl', lambda *_: json.dumps({'metadata': {'labels': {}}, 'data': {}})), self.assertRaises(SystemExit):
            cluster.secret()

    def test_ingress_hosts_are_detected(self):
        ingress = {'items': [{'spec': {'tls': [{'hosts': ['vmcp.example']}], 'rules': [
            {'host': 'vmcp.example', 'http': {'paths': [{'backend': {'service': {'name': 'vmcp-demo-web'}}}]}},
            {'host': 'mcp.example', 'http': {'paths': [{'backend': {'service': {'name': 'vmcp-demo-data-plane'}}}]}}]}}]}
        cluster = demo.Cluster(demo.parser().parse_args(['urls']))
        with patch.object(demo.Cluster, 'kubectl', lambda *_: json.dumps(ingress)):
            urls = cluster.urls()
        self.assertEqual((urls['console'], urls['mcpBase'], urls['keycloak']),
                         ('https://vmcp.example', 'http://mcp.example/mcp/v1/virtual/', 'http://localhost:8803'))


if __name__ == '__main__': unittest.main()
