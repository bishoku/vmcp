"""Trial orchestration regressions. No cluster is touched by these tests."""
from contextlib import redirect_stdout
import io
import importlib.util
import json
from pathlib import Path
import subprocess
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('quickstart', ROOT / 'deploy/helm/vmcp/quickstart/run.py')
q = importlib.util.module_from_spec(spec)
spec.loader.exec_module(q)


def args(*extra):
    a = q.parser().parse_args(['--context', 'test', *extra])
    q.validate(a)
    return a


def inventory():
    return {'version': 'v0.0.2', 'images': [dict(service=s, version='v0.0.2', image=f'barishoku/vmcp-{s}', digest='sha256:' + 'a' * 64) for s in sorted(q.SERVICES)]}


class QuickstartTests(unittest.TestCase):
    def test_help_does_not_require_cluster_or_helm(self):
        r = subprocess.run(['bash', str(ROOT / 'deploy/helm/vmcp/quickstart.sh'), '--help'], capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        for flag in ['--kafka', '--rabbitmq', '--exporter', '--uninstall', '--purge', '--ephemeral']:
            self.assertIn(flag, r.stdout)

    def test_rejects_invalid_combinations_before_any_mutation(self):
        for options in [('--purge',), ('--exporter', 'http'), ('--http-export-url', 'https://sink'),
                        ('--chart', '/tmp/chart'), ('--release', 'Bad!'), ('--web-host', 'console.test'),
                        ('--tls-secret', 'demo-tls'), ('--gateway-host', 'mcp.test'),
                        ('--web-host', 'same.test', '--gateway-host', 'same.test'),
                        ('--web-port', '8801'), ('--data-plane-replicas', '0')]:
            with self.subTest(options=options), self.assertRaises(ValueError): args(*options)

    def test_ingress_urls_tls_and_access_output(self):
        for tls in [False, True]:
            with self.subTest(tls=tls):
                options = ['--web-host', 'vmcp.kube.local', '--gateway-host', 'mcp.kube.local']
                if tls:
                    options += ['--tls-secret', 'demo-tls']
                a = args(*options)
                cfg = q.values(a, q.image_values(inventory(), a.version))
                scheme = 'https' if tls else 'http'
                self.assertEqual(cfg['runtime']['publicGatewayURL'], scheme + '://mcp.kube.local')
                self.assertEqual(cfg['runtime']['allowedOrigins'], [scheme + '://vmcp.kube.local'])
                self.assertEqual(cfg['ingress']['className'], 'nginx')
                annotations = cfg['ingress']['annotations']
                self.assertEqual(annotations['nginx.ingress.kubernetes.io/proxy-buffering'], 'off')
                if tls:
                    self.assertEqual(cfg['ingress']['tls'], [{
                        'hosts': ['vmcp.kube.local', 'mcp.kube.local'], 'secretName': 'demo-tls'}])
                    self.assertNotIn('nginx.ingress.kubernetes.io/ssl-redirect', annotations)
                else:
                    self.assertEqual(cfg['ingress']['tls'], [])
                    for name in ['ssl-redirect', 'force-ssl-redirect']:
                        self.assertEqual(annotations['nginx.ingress.kubernetes.io/' + name], 'false')
                output = io.StringIO()
                with redirect_stdout(output), patch.object(q.subprocess, 'Popen') as popen:
                    q.Installer(a).access()
                popen.assert_not_called()
                self.assertIn('Console: ' + scheme + '://vmcp.kube.local', output.getvalue())
                self.assertIn('Gateway: ' + scheme + '://mcp.kube.local/mcp/v1/virtual/<slug>', output.getvalue())

    def test_validates_image_versions_repositories_and_completeness(self):
        self.assertEqual(len(q.image_values(inventory(), 'v0.0.2')), 9)
        for mutate in [lambda m: m['images'].pop(), lambda m: m['images'].append(m['images'][0]),
                       lambda m: m['images'][0].update(image='attacker/image'), lambda m: m.update(version='v0.0.3')]:
            m = inventory(); mutate(m)
            with self.assertRaises(ValueError): q.image_values(m, 'v0.0.2')

    def test_exporters_imply_brokers_and_have_independent_consumers(self):
        a = args('--exporter', 'kafka', '--exporter', 'rabbitmq')
        self.assertTrue(a.kafka and a.rabbitmq)
        cfg = q.values(a, q.image_values(inventory(), a.version))
        consumers = [c['consumer'] for c in cfg['exporters'].values()]
        self.assertEqual(len(set(consumers)), 2)
        self.assertTrue(all(c['env']['VMCP_AUDIT_EXPORT_ALLOW_PLAINTEXT'] == 'true' for c in cfg['exporters'].values()))
        self.assertFalse(cfg['networkPolicy']['enabled'])
        self.assertEqual(cfg['dataPlane']['replicas'], 2)
        self.assertNotIn('value', cfg['credentials']['masterKey'])
        maps = {r['metadata']['name']: r['data'] for r in q.infrastructure(a) if r['kind'] == 'ConfigMap'}
        for kind, exporter in cfg['exporters'].items():
            policy = maps[exporter['envFrom'][0]['configMapRef']['name']]
            self.assertEqual(policy['VMCP_EGRESS_ALLOW_HOSTS'], a.release + '-trial-' + kind)
            self.assertIn('10.0.0.0/8', policy['VMCP_EGRESS_ALLOW_CIDRS'])

    def test_persistent_volumes_are_retained_and_ephemeral_mode_has_no_pvcs(self):
        persistent = q.infrastructure(args('--kafka', '--rabbitmq'))
        claims = [r for r in persistent if r['kind'] == 'PersistentVolumeClaim']
        self.assertEqual(len(claims), 4)
        self.assertTrue(all(c['metadata']['annotations']['helm.sh/resource-policy'] == 'keep' for c in claims))
        ephemeral = q.infrastructure(args('--ephemeral'))
        self.assertFalse(any(r['kind'] == 'PersistentVolumeClaim' for r in ephemeral))
        for workload in (r for r in persistent if r['kind'] == 'Deployment'):
            self.assertEqual(workload['spec']['strategy']['type'], 'Recreate')
            self.assertIn('@sha256:', workload['spec']['template']['spec']['containers'][0]['image'])
        kafka = next(r for r in persistent if r['kind'] == 'Deployment' and r['metadata']['name'].endswith('-kafka'))
        env = {e['name']: e.get('value') for e in kafka['spec']['template']['spec']['containers'][0]['env']}
        self.assertEqual(env['KAFKA_CONTROLLER_QUORUM_VOTERS'], '1@localhost:9093', 'controller must start before its own Service has ready endpoints')

    def test_existing_credentials_are_preserved_and_secret_not_written_to_argv(self):
        a = args()
        installer = q.Installer(a)
        calls = []
        def first(cmd, data=None, capture=False):
            calls.append((cmd, data))
            return ''
        installer.run = first
        original = installer.secret()
        stored = json.loads(calls[-1][1])
        stored['metadata']['resourceVersion'] = '5'
        calls.clear()
        def again(cmd, data=None, capture=False):
            calls.append((cmd, data))
            return json.dumps(stored) if 'get' in cmd else ''
        installer.run = again
        self.assertEqual(installer.secret(), original)
        self.assertIn('replace', calls[-1][0])
        for cmd, _ in calls: self.assertNotIn(original['admin-token'], ' '.join(cmd))
        stored['metadata']['labels'] = {}
        with self.assertRaises(ValueError): installer.secret()

    def test_bootstrap_orders_roles_migration_then_grants(self):
        installer = q.Installer(args())
        actions = []
        installer.sql = lambda sql: actions.append(('sql', sql))
        installer.apply = lambda resource: actions.append(('job', resource))
        installer.run = lambda cmd, **kw: actions.append(('wait', cmd))
        installer.bootstrap({k + '-password': 'a' * 48 for k in ['owner', 'control', 'data', 'export']}, 'barishoku/vmcp-migrate@sha256:' + 'a' * 64)
        self.assertEqual([a[0] for a in actions], ['sql'] * 5 + ['job', 'wait', 'sql'])
        self.assertIn('tenant_runtime_identity', actions[-1][1])
        self.assertNotIn('GRANT ALL', actions[-1][1])
        self.assertEqual(actions[5][1]['spec']['template']['spec']['containers'][0]['env'][0]['valueFrom']['secretKeyRef']['key'], 'migration-url')

    def test_http_export_url_is_only_a_secret_reference_in_values(self):
        a = args('--exporter', 'http', '--http-export-url', 'https://sink.test/audit?token=sensitive')
        cfg = q.values(a, q.image_values(inventory(), a.version))
        self.assertNotIn('sensitive', json.dumps(cfg))
        self.assertEqual(cfg['exporters']['http']['secretEnv']['VMCP_AUDIT_EXPORT_URL']['key'], 'http-export-url')


if __name__ == '__main__': unittest.main()
