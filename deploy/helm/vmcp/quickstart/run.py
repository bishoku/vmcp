"""Helm trial installer. No third-party Python modules required."""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request

HERE = Path(__file__).resolve().parent
IMAGES = {
    'postgres': 'postgres@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24',  # 17-alpine
    'redis': 'redis@sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499',  # 7.4-alpine
    'rabbitmq': 'rabbitmq@sha256:b586ea9784425774ac56f3d4919e4f723edb3287ef8a6c4c8f0df8fe569747c3',  # 4.1-management-alpine
    'kafka': 'apache/kafka@sha256:4ceccc577f03f51f6af8dbfda55194d0d892f4fa7913ffbded567ce3895622ed',  # 3.9.1
}
SERVICES = {'web', 'control-plane', 'data-plane', 'migrate', 'audit-exporter',
            'audit-exporter-kafka', 'audit-exporter-rabbitmq', 'tenant-router', 'rotate-secrets'}


def parser():
    p = argparse.ArgumentParser(description='Install a vMCP trial stack into the selected Kubernetes context. NOT a production installer.',
        epilog='Secrets and PVCs survive --uninstall. --uninstall --purge explicitly deletes quickstart Secrets and PVCs. No namespace or cluster is deleted.')
    p.add_argument('--context', help='Kubernetes context (defaults to current context; never changes kubeconfig)')
    p.add_argument('--namespace', default='vmcp-demo')
    p.add_argument('--release', default='vmcp-demo', help='Helm release prefix (maximum 30 characters)')
    p.add_argument('--version', default='v0.0.2', help='Published GitHub Release version (default: v0.0.2)')
    p.add_argument('--kafka', action='store_true', help='Install a single Kafka broker and create vmcp-audit topic')
    p.add_argument('--rabbitmq', action='store_true', help='Install RabbitMQ with a durable vmcp-audit queue')
    p.add_argument('--exporter', action='append', choices=['http', 'kafka', 'rabbitmq'], default=[], help='Enable an audit exporter; repeat for multiple destinations. Broker exporters imply their broker.')
    p.add_argument('--http-export-url', help='HTTP exporter destination URL (requires --exporter http)')
    p.add_argument('--monitoring', action='store_true', help='Enable ServiceMonitors, rules and dashboard ConfigMap; requires Prometheus Operator and Grafana')
    p.add_argument('--monitoring-release', default='monitoring', help='Prometheus resource selection label')
    p.add_argument('--storage-class', help='PVC storage class (otherwise requires a default StorageClass)')
    p.add_argument('--ephemeral', action='store_true', help='Use emptyDir instead of PVCs; ALL dependency data is lost on pod replacement')
    p.add_argument('--data-plane-replicas', type=int, default=2)
    p.add_argument('--web-port', type=int, default=3300)
    p.add_argument('--gateway-port', type=int, default=8801)
    p.add_argument('--no-port-forward', action='store_true', help='Print port-forward commands instead of keeping them running')
    p.add_argument('--web-host', help='Console ingress hostname; requires --gateway-host and --tls-secret')
    p.add_argument('--gateway-host', help='MCP ingress hostname')
    p.add_argument('--tls-secret', help='Existing TLS Secret in the target namespace')
    p.add_argument('--ingress-class', default='nginx', help='Ingress class; default annotations target nginx')
    p.add_argument('--timeout', default='15m', help='Helm timeout, e.g. 15m')
    p.add_argument('--chart', type=Path, help='Local application chart directory/archive; requires --images-file')
    p.add_argument('--images-file', type=Path, help='Local published images.json; requires --chart (offline/local validation)')
    p.add_argument('--uninstall', action='store_true', help='Uninstall the two quickstart releases; keep Secrets/PVCs')
    p.add_argument('--purge', action='store_true', help='With --uninstall, delete quickstart Secrets/PVCs, including stored data')
    return p


def validate(a):
    for value, maximum in [(a.namespace, 63), (a.release, 30)]:
        if len(value) > maximum or not re.fullmatch(r'[a-z][a-z0-9]*(?:-[a-z0-9]+)*', value):
            raise ValueError('namespace/release must be lowercase DNS names within their length limits')
    if not re.fullmatch(r'v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[A-Za-z0-9]+(?:[.-][A-Za-z0-9]+)*)?', a.version):
        raise ValueError('version must be a version tag such as v0.0.2')
    if a.purge and not a.uninstall:
        raise ValueError('--purge requires --uninstall')
    if a.data_plane_replicas < 1 or not all(1024 <= v <= 65535 for v in [a.web_port, a.gateway_port]) or a.web_port == a.gateway_port:
        raise ValueError('replicas must be positive and local ports must be distinct, between 1024 and 65535')
    if not re.fullmatch(r'[1-9][0-9]*[ms]', a.timeout):
        raise ValueError('timeout must be a positive duration such as 15m or 900s')
    if bool(a.chart) != bool(a.images_file):
        raise ValueError('--chart and --images-file must be supplied together')
    if any([a.web_host, a.gateway_host, a.tls_secret]):
        if not all([a.web_host, a.gateway_host, a.tls_secret]) or a.web_host == a.gateway_host:
            raise ValueError('ingress requires distinct --web-host/--gateway-host and --tls-secret')
        for host in [a.web_host, a.gateway_host]:
            if not re.fullmatch(r'[a-z0-9]+(?:[.-][a-z0-9]+)*', host):
                raise ValueError('ingress hosts must be DNS hostnames without scheme or path')
    if ('http' in a.exporter) != bool(a.http_export_url):
        raise ValueError('--exporter http and --http-export-url must be supplied together')
    if a.http_export_url:
        endpoint = urllib.parse.urlsplit(a.http_export_url)
        if endpoint.scheme not in ['http', 'https'] or not endpoint.hostname or endpoint.username or endpoint.fragment:
            raise ValueError('HTTP exporter requires an http(s) endpoint without userinfo or fragment')
    a.kafka = a.kafka or 'kafka' in a.exporter
    a.rabbitmq = a.rabbitmq or 'rabbitmq' in a.exporter


def image_values(manifest, version):
    if manifest.get('version') != version:
        raise ValueError('image inventory version does not match requested version')
    refs = {}
    for record in manifest['images']:
        name = record['service']
        if name not in SERVICES or name in refs or record['version'] != version or record['image'] != f'barishoku/vmcp-{name}' or not re.fullmatch(r'sha256:[a-f0-9]{64}', record['digest']):
            raise ValueError('invalid or duplicate published image record')
        refs[name] = record['image'] + '@' + record['digest']
    if set(refs) != SERVICES:
        raise ValueError('published image inventory is incomplete')
    return refs


def obj(kind, name, **fields):
    return dict(apiVersion='apps/v1' if kind == 'Deployment' else 'v1', kind=kind, metadata={'name': name}, **fields)


def env_secret(variable, secret, key):
    return {'name': variable, 'valueFrom': {'secretKeyRef': {'name': secret, 'key': key}}}


def infrastructure(a):
    prefix, secret = a.release + '-trial', a.release + '-quickstart'
    resources = []
    selected = ['postgres', 'redis'] + (['kafka'] if a.kafka else []) + (['rabbitmq'] if a.rabbitmq else [])
    for name in selected:
        service = prefix + '-' + name
        labels = {'vmcp.quickstart': a.release, 'app': service}
        ports = {'postgres': [5432], 'redis': [6379], 'kafka': [9092, 9093], 'rabbitmq': [5672, 15672]}[name]
        c = {'name': name, 'image': IMAGES[name], 'ports': [{'containerPort': p} for p in ports],
             'resources': {'requests': {'cpu': '100m', 'memory': '128Mi' if name != 'kafka' else '512Mi'},
                           'limits': {'cpu': '2', 'memory': '512Mi' if name != 'kafka' else '1Gi'}}}
        c['env'] = []
        if name == 'postgres':
            c['env'] = [env_secret('POSTGRES_PASSWORD', secret, 'postgres-password'), {'name': 'POSTGRES_DB', 'value': 'vmcp'}, {'name': 'PGDATA', 'value': '/var/lib/postgresql/data/pgdata'}]
            c['readinessProbe'] = {'exec': {'command': ['pg_isready', '-U', 'postgres', '-d', 'vmcp']}, 'periodSeconds': 3}
        elif name == 'redis':
            c['env'] = [env_secret('REDIS_PASSWORD', secret, 'redis-password')]
            c['command'] = ['sh', '-ec', 'exec redis-server --appendonly yes --requirepass "$REDIS_PASSWORD"']
            c['readinessProbe'] = {'exec': {'command': ['sh', '-ec', 'REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli ping']}, 'periodSeconds': 3}
        elif name == 'kafka':
            settings = {'KAFKA_NODE_ID': '1', 'KAFKA_PROCESS_ROLES': 'broker,controller', 'KAFKA_LISTENERS': 'PLAINTEXT://:9092,CONTROLLER://:9093',
                        'KAFKA_ADVERTISED_LISTENERS': f'PLAINTEXT://{service}:9092', 'KAFKA_CONTROLLER_QUORUM_VOTERS': '1@localhost:9093',
                        'KAFKA_LISTENER_SECURITY_PROTOCOL_MAP': 'CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT', 'KAFKA_CONTROLLER_LISTENER_NAMES': 'CONTROLLER',
                        'KAFKA_INTER_BROKER_LISTENER_NAME': 'PLAINTEXT', 'KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR': '1',
                        'KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR': '1', 'KAFKA_TRANSACTION_STATE_LOG_MIN_ISR': '1',
                        'KAFKA_LOG_DIRS': '/var/lib/kafka/data', 'CLUSTER_ID': 'MkU3OEVBNTcwNTJENDM2Qk', 'KAFKA_HEAP_OPTS': '-Xms256m -Xmx512m'}
            c['env'] = [{'name': k, 'value': v} for k, v in settings.items()]
            c['readinessProbe'] = {'tcpSocket': {'port': 9092}, 'periodSeconds': 5}
        else:
            c['env'] = [{'name': 'RABBITMQ_DEFAULT_USER', 'value': 'audit'}, env_secret('RABBITMQ_DEFAULT_PASS', secret, 'rabbit-password'),
                        {'name': 'RABBITMQ_NODENAME', 'value': 'rabbit@localhost'}]
            c['readinessProbe'] = {'exec': {'command': ['rabbitmq-diagnostics', '-q', 'ping']}, 'periodSeconds': 5, 'timeoutSeconds': 10}
        mount = {'postgres': '/var/lib/postgresql/data', 'redis': '/data', 'kafka': '/var/lib/kafka/data', 'rabbitmq': '/var/lib/rabbitmq'}[name]
        c['volumeMounts'] = [{'name': 'data', 'mountPath': mount}]
        volume = {'name': 'data', 'emptyDir': {}}
        if not a.ephemeral:
            pvc = obj('PersistentVolumeClaim', service + '-data', spec={'accessModes': ['ReadWriteOnce'], 'resources': {'requests': {'storage': '2Gi'}}})
            pvc['metadata'].update(labels=labels, annotations={'helm.sh/resource-policy': 'keep'})
            if a.storage_class: pvc['spec']['storageClassName'] = a.storage_class
            resources.append(pvc)
            volume = {'name': 'data', 'persistentVolumeClaim': {'claimName': service + '-data'}}
        pod = {'automountServiceAccountToken': False, 'containers': [c], 'volumes': [volume]}
        if name == 'kafka': pod['securityContext'] = {'fsGroup': 1000}
        resources += [obj('Service', service, spec={'selector': labels, 'ports': [{'name': f'p{p}', 'port': p, 'targetPort': p} for p in ports]}),
                      obj('Deployment', service, spec={'replicas': 1, 'strategy': {'type': 'Recreate'}, 'selector': {'matchLabels': labels}, 'template': {'metadata': {'labels': labels}, 'spec': pod}})]
    for exporter in set(a.exporter):
        host = urllib.parse.urlsplit(a.http_export_url).hostname if exporter == 'http' else prefix + '-' + ('rabbitmq' if exporter == 'rabbitmq' else 'kafka')
        resources.append(obj('ConfigMap', prefix + '-egress-' + exporter, data={
            'VMCP_EGRESS_ALLOW_HOSTS': host,
            'VMCP_EGRESS_ALLOW_CIDRS': '10.0.0.0/8,172.16.0.0/12,192.168.0.0/16'}))
    return resources


def values(a, refs):
    secret = a.release + '-quickstart'
    source = lambda key: {'existingSecret': secret, 'key': key}
    cfg = {'fullnameOverride': a.release, 'credentials': {'masterKey': source('master-key'), 'adminToken': source('admin-token')},
           'database': {k: source(k + '-url') for k in ['controlPlane', 'dataPlane', 'migration', 'listener']},
           'rateLimit': {'redisURL': source('redis-url')}, 'networkPolicy': {'enabled': False},
           'workloadDefaults': {'replicas': 1, 'pdb': {'enabled': False}, 'resources': {'requests': {'cpu': '50m', 'memory': '64Mi'}, 'limits': {'cpu': '1', 'memory': '512Mi'}}},
           'runtime': {'publicGatewayURL': f'http://localhost:{a.gateway_port}', 'allowedOrigins': [f'http://localhost:{a.web_port}'],
                       'egressAllowCIDRs': ['10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16'], 'pool': {'maxConnections': 8, 'minConnections': 1}},
           'exporters': {}}
    for key, name in [('web', 'web'), ('controlPlane', 'control-plane'), ('dataPlane', 'data-plane'), ('migration', 'migrate')]:
        cfg[key] = {'image': refs[name]}
    cfg['dataPlane']['replicas'] = a.data_plane_replicas
    for exporter in set(a.exporter):
        e = {'enabled': True, 'image': refs['audit-exporter' + ('' if exporter == 'http' else '-' + exporter)],
             'consumer': 'quickstart-' + exporter, 'database': source('export-url'),
             'envFrom': [{'configMapRef': {'name': a.release + '-trial-egress-' + exporter}}], 'env': {'VMCP_AUDIT_EXPORT_ALLOW_PLAINTEXT': 'true'}, 'secretEnv': {}}
        if exporter == 'http':
            e['secretEnv']['VMCP_AUDIT_EXPORT_URL'] = source('http-export-url')
            e['podAnnotations'] = {'vmcp.quickstart/endpoint-hash': hashlib.sha256(a.http_export_url.encode()).hexdigest()}
        if exporter == 'kafka': e['env'].update(VMCP_AUDIT_KAFKA_BROKERS=a.release + '-trial-kafka:9092', VMCP_AUDIT_KAFKA_TOPIC='vmcp-audit')
        if exporter == 'rabbitmq':
            e['secretEnv']['VMCP_AUDIT_RABBITMQ_URL'] = source('rabbit-url')
            e['env']['VMCP_AUDIT_RABBITMQ_ROUTING_KEY'] = 'vmcp-audit'
        cfg['exporters'][exporter] = e
    if a.monitoring:
        cfg['monitoring'] = {'serviceMonitor': {'enabled': True, 'labels': {'release': a.monitoring_release}},
                            'prometheusRule': {'enabled': True, 'labels': {'release': a.monitoring_release}}, 'dashboards': {'enabled': True}}
    if a.web_host:
        cfg['runtime'].update(publicGatewayURL='https://' + a.gateway_host, allowedOrigins=['https://' + a.web_host])
        cfg['ingress'] = {'enabled': True, 'className': a.ingress_class, 'webHost': a.web_host, 'gatewayHost': a.gateway_host,
                          'tls': [{'hosts': [a.web_host, a.gateway_host], 'secretName': a.tls_secret}],
                          'annotations': {'nginx.ingress.kubernetes.io/proxy-read-timeout': '3600', 'nginx.ingress.kubernetes.io/proxy-send-timeout': '3600', 'nginx.ingress.kubernetes.io/proxy-buffering': 'off'}}
    return cfg


class Installer:
    def __init__(self, a):
        self.a = a
        self.k = ['kubectl', '--context', a.context, '-n', a.namespace]
        self.h = ['helm', '--kube-context', a.context, '-n', a.namespace]

    def run(self, cmd, data=None, capture=False):
        try:
            return subprocess.run(cmd, input=data, text=True, check=True, stdout=subprocess.PIPE if capture else None).stdout
        except subprocess.CalledProcessError:
            if cmd[0] == 'helm' or 'wait' in cmd:
                subprocess.run(self.k + ['get', 'pods,jobs,pvc', '-o', 'wide'], check=False)
                subprocess.run(self.k + ['get', 'events', '--sort-by=.lastTimestamp'], check=False)
            raise

    def apply(self, resource):
        self.run(self.k + ['apply', '-f', '-'], json.dumps(resource))

    def secret(self):
        a = self.a
        name = a.release + '-quickstart'
        raw = self.run(self.k + ['get', 'secret', name, '--ignore-not-found', '-o', 'json'], capture=True)
        if raw.strip():
            stored = json.loads(raw)
            if stored['metadata'].get('labels', {}).get('vmcp.quickstart') != a.release:
                raise ValueError('refusing to reuse a Secret not owned by this quickstart')
            data = {k: base64.b64decode(v).decode() for k, v in stored['data'].items()}
            for key in ['master-key', 'admin-token', 'postgres-password', 'owner-password', 'control-password', 'data-password', 'export-password', 'redis-password', 'rabbit-password']:
                if not data.get(key): raise ValueError('existing quickstart Secret is incomplete; refusing to rotate credentials')
        else:
            data = {key: secrets.token_hex(24) for key in ['admin-token', 'postgres-password', 'owner-password', 'control-password', 'data-password', 'export-password', 'redis-password', 'rabbit-password']}
            data['master-key'] = base64.b64encode(secrets.token_bytes(32)).decode()
        if len(base64.b64decode(data['master-key'], validate=True)) != 32:
            raise ValueError('retained encryption key must decode to 32 bytes')
        for field, user, pw in [('migration', 'vmcp_owner', 'owner'), ('controlPlane', 'vmcp_control', 'control'), ('dataPlane', 'vmcp_data', 'data'), ('listener', 'vmcp_data', 'data'), ('export', 'vmcp_export', 'export')]:
            data[field + '-url'] = f'postgres://{user}:{data[pw + "-password"]}@{a.release}-trial-postgres:5432/vmcp?sslmode=disable'
        data['redis-url'] = f'redis://:{data["redis-password"]}@{a.release}-trial-redis:6379/0'
        data['rabbit-url'] = f'amqp://audit:{data["rabbit-password"]}@{a.release}-trial-rabbitmq:5672/'
        if a.http_export_url: data['http-export-url'] = a.http_export_url
        resource = obj('Secret', name, type='Opaque', stringData=data)
        resource['metadata']['labels'] = {'vmcp.quickstart': a.release}
        # Replace using stdin: avoid persisting credentials in kubectl's last-applied annotation.
        resource.pop('stringData')
        resource['data'] = {k: base64.b64encode(v.encode()).decode() for k, v in data.items()}
        if raw.strip():
            resource['metadata']['resourceVersion'] = stored['metadata']['resourceVersion']
            self.run(self.k + ['replace', '-f', '-'], json.dumps(resource))
        else: self.run(self.k + ['create', '-f', '-'], json.dumps(resource))
        return data

    def sql(self, sql):
        self.run(self.k + ['exec', '-i', 'deployment/' + self.a.release + '-trial-postgres', '--', 'psql', '-U', 'postgres', '-d', 'vmcp', '-v', 'ON_ERROR_STOP=1'], sql)

    def bootstrap(self, credentials, migrate_image):
        for role, key in [('vmcp_owner', 'owner'), ('vmcp_control', 'control'), ('vmcp_data', 'data'), ('vmcp_export', 'export')]:
            # Secret-derived passwords are hex; verify before putting them in SQL literals.
            password = credentials[key + '-password']
            if not re.fullmatch(r'[0-9a-f]{48}', password): raise ValueError('invalid retained database password')
            self.sql(f"DO $$ BEGIN IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='{role}') THEN CREATE ROLE {role} LOGIN PASSWORD '{password}'; END IF; END $$;\n")
        self.sql('ALTER DATABASE vmcp OWNER TO vmcp_owner; ALTER SCHEMA public OWNER TO vmcp_owner;\n'
                 'ALTER DEFAULT PRIVILEGES FOR ROLE vmcp_owner IN SCHEMA public GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO vmcp_control,vmcp_data;\n'
                 'ALTER DEFAULT PRIVILEGES FOR ROLE vmcp_owner IN SCHEMA public GRANT USAGE,SELECT ON SEQUENCES TO vmcp_control,vmcp_data;\n')
        name = self.a.release + '-bootstrap-' + secrets.token_hex(3)
        job = {'apiVersion': 'batch/v1', 'kind': 'Job', 'metadata': {'name': name, 'labels': {'vmcp.quickstart': self.a.release}},
               'spec': {'ttlSecondsAfterFinished': 3600, 'activeDeadlineSeconds': 600, 'backoffLimit': 2,
                        'template': {'spec': {'restartPolicy': 'Never', 'automountServiceAccountToken': False,
                        'containers': [{'name': 'migrate', 'image': migrate_image, 'env': [env_secret('VMCP_DATABASE_URL', self.a.release + '-quickstart', 'migration-url')]}]}}}}
        self.apply(job)
        self.run(self.k + ['wait', '--for=condition=complete', 'job/' + name, '--timeout=' + self.a.timeout])
        self.sql('GRANT USAGE ON SCHEMA public TO vmcp_control,vmcp_data,vmcp_export;\n'
                 'GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO vmcp_control,vmcp_data;\n'
                 'GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO vmcp_control,vmcp_data;\n'
                 'GRANT SELECT ON tenant_runtime_identity,tenants,audit_events TO vmcp_export;\n'
                 'GRANT SELECT,INSERT,UPDATE ON audit_export_state,audit_deliveries TO vmcp_export;\n')

    def brokers(self):
        if self.a.kafka:
            self.run(self.k + ['exec', 'deployment/' + self.a.release + '-trial-kafka', '--', '/opt/kafka/bin/kafka-topics.sh', '--bootstrap-server', 'localhost:9092', '--create', '--if-not-exists', '--topic', 'vmcp-audit', '--partitions', '1', '--replication-factor', '1'])
        if self.a.rabbitmq:
            # Local CLI authenticates using the node cookie; no password in process arguments.
            self.run(self.k + ['exec', '-i', 'deployment/' + self.a.release + '-trial-rabbitmq', '--', 'sh', '-ec', 'cat > /tmp/vmcp-queues.json; rabbitmqctl import_definitions /tmp/vmcp-queues.json'], json.dumps({'queues': [{'name': 'vmcp-audit', 'vhost': '/', 'durable': True, 'auto_delete': False, 'arguments': {}}]}))
            for attempt in range(30):
                queues = self.run(self.k + ['exec', 'deployment/' + self.a.release + '-trial-rabbitmq', '--', 'rabbitmqctl', 'list_queues', 'name', '--quiet'], capture=True)
                if 'vmcp-audit' in queues.splitlines(): break
                time.sleep(1)
            else: raise ValueError('RabbitMQ audit queue did not become available')

    def access(self):
        a = self.a
        print('\nStack ready. Trial only: plaintext dependencies, broad private upstream access, no NetworkPolicies.')
        print('Admin token (retrieve locally; not printed by installer):')
        print(f"kubectl --context {a.context} -n {a.namespace} get secret {a.release}-quickstart -o jsonpath='{{.data.admin-token}}' | base64 --decode; echo")
        commands = [self.k + ['port-forward', '--address', '127.0.0.1', 'service/' + a.release + '-' + component, f'{port}:8080'] for component, port in [('web', a.web_port), ('data-plane', a.gateway_port)]]
        print(f'Console: https://{a.web_host}' if a.web_host else f'Console: http://localhost:{a.web_port}')
        print(f'Gateway: https://{a.gateway_host}/mcp/v1/virtual/<slug>' if a.gateway_host else f'Gateway: http://localhost:{a.gateway_port}/mcp/v1/virtual/<slug>')
        import shlex
        for command in commands: print(shlex.join(command))
        if a.no_port_forward or a.web_host: return
        processes = []
        try:
            for command in commands: processes.append(subprocess.Popen(command))
            print('Port forwards running. Ctrl-C stops forwarding; workloads remain installed.', flush=True)
            while True:
                for process in processes:
                    if process.poll() is not None: raise RuntimeError('port-forward exited; check local port availability and pod status')
                time.sleep(1)
        finally:
            for process in processes:
                process.terminate()
            for process in processes: process.wait(timeout=10)


def download_release(a, directory):
    def fetch(url):
        with urllib.request.urlopen(urllib.request.Request(url, headers={'User-Agent': 'vmcp-quickstart'}), timeout=60) as response:
            return response.read()
    release = json.loads(fetch('https://api.github.com/repos/bishoku/vmcp/releases/tags/' + a.version))
    if release['draft']: raise ValueError('release is still a draft')
    assets = {item['name']: item['browser_download_url'] for item in release['assets']}
    charts = [name for name in assets if re.fullmatch(r'vmcp-[0-9][A-Za-z0-9.+-]*\.tgz', name)]
    if len(charts) != 1: raise ValueError('release must contain one packaged Helm chart')
    checksums = {line.split()[1]: line.split()[0] for line in fetch(assets['SHA256SUMS']).decode().splitlines()}
    for name in ['images.json', charts[0]]:
        content = fetch(assets[name])
        if hashlib.sha256(content).hexdigest() != checksums.get(name): raise ValueError('release asset checksum mismatch')
        (directory / name).write_bytes(content)
    return directory / charts[0], directory / 'images.json'


def main(argv=None):
    a = parser().parse_args(argv)
    validate(a)
    for command in ['kubectl', 'helm']:
        if not shutil.which(command): raise ValueError(f'{command} is required')
    if not a.context:
        a.context = subprocess.check_output(['kubectl', 'config', 'current-context'], text=True).strip()
    installer = Installer(a)
    print(f'Context: {a.context}; namespace: {a.namespace}; releases: {a.release}, {a.release}-infra', flush=True)
    installer.run(['kubectl', '--context', a.context, 'get', 'nodes'])
    kube_version = json.loads(installer.run(['kubectl', '--context', a.context, 'version', '-o', 'json'], capture=True))['serverVersion']
    helm_version = installer.run(['helm', 'version', '--template', '{{.Version}}'], capture=True).strip()
    if (int(kube_version['major']), int(re.match(r'\d+', kube_version['minor'])[0])) < (1, 27):
        raise ValueError('Kubernetes 1.27+ is required')
    match = re.match(r'v(\d+)\.(\d+)', helm_version)
    if not match or tuple(map(int, match.groups())) < (3, 17):
        raise ValueError('Helm 3.17+ is required')
    if a.uninstall:
        for release in [a.release, a.release + '-infra']:
            installer.run(installer.h + ['uninstall', release, '--ignore-not-found', '--wait', '--timeout', a.timeout])
        if a.purge:
            installer.run(installer.k + ['delete', 'secret,pvc,job', '-l', 'vmcp.quickstart=' + a.release, '--ignore-not-found'])
        print('Uninstalled. Namespace retained.' + (' Quickstart data and credentials deleted.' if a.purge else ' Secrets and PVCs retained.'))
        return
    if not a.ephemeral:
        storage = json.loads(installer.run(['kubectl', '--context', a.context, 'get', 'storageclasses', '-o', 'json'], capture=True))['items']
        if not any(s['metadata']['name'] == a.storage_class if a.storage_class else any(s['metadata'].get('annotations', {}).get(k) == 'true' for k in ['storageclass.kubernetes.io/is-default-class', 'storageclass.beta.kubernetes.io/is-default-class']) for s in storage):
            raise ValueError('no selected/default StorageClass; specify --storage-class or --ephemeral')
    if a.monitoring:
        installer.run(['kubectl', '--context', a.context, 'get', 'crd', 'servicemonitors.monitoring.coreos.com', 'prometheusrules.monitoring.coreos.com'])
    if a.web_host: installer.run(installer.k + ['get', 'secret', a.tls_secret])
    with tempfile.TemporaryDirectory(prefix='vmcp-quickstart-') as temp:
        directory = Path(temp)
        chart, manifest = (a.chart.resolve(), a.images_file.resolve()) if a.chart else download_release(a, directory)
        refs = image_values(json.loads(manifest.read_text()), a.version)
        if chart.is_dir():
            installer.run(['helm', 'repo', 'add', 'bitnami', 'https://charts.bitnami.com/bitnami', '--force-update'])
            installer.run(['helm', 'dependency', 'build', str(chart)])
        # Validate application configuration before mutating the cluster.
        cfg = values(a, refs)
        file = directory / 'values.json'
        file.write_text(json.dumps(cfg))
        installer.run(['helm', 'lint', str(chart), '-f', str(file)])
        namespace = obj('Namespace', a.namespace)
        installer.apply(namespace)
        credentials = installer.secret()
        infra = directory / 'infra.json'
        infra.write_text(json.dumps({'resources': infrastructure(a)}))
        installer.run(installer.h + ['upgrade', '--install', a.release + '-infra', str(HERE / 'infra'), '-f', str(infra), '--wait', '--timeout', a.timeout])
        installer.bootstrap(credentials, refs['migrate'])
        installer.brokers()
        installer.run(installer.h + ['upgrade', '--install', a.release, str(chart), '-f', str(file), '--wait', '--wait-for-jobs', '--timeout', a.timeout])
    installer.access()


if __name__ == '__main__':
    try:
        main()
    except KeyboardInterrupt:
        print('\nStopped. Installed workloads and data are retained.')
    except Exception as exc:
        # Do not echo subprocess command/input, which may contain operator credentials.
        detail = str(exc) if isinstance(exc, ValueError) else 'Check the command output above.'
        if isinstance(exc, urllib.error.HTTPError):
            detail = f'Release download returned HTTP {exc.code}; check --version, GitHub API limits and network access.'
        elif isinstance(exc, urllib.error.URLError):
            detail = 'Cannot download release assets; check network access and trusted CA certificates.'
        print(f'Quickstart failed: {type(exc).__name__}. {detail}', file=sys.stderr)
        print('Inspect namespace pods, jobs and events with kubectl; no automatic cleanup was performed.', file=sys.stderr)
        sys.exit(1)
