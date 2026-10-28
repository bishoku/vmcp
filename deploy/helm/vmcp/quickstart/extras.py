"""Optional, digest-pinned trial infrastructure (not production defaults)."""
KEYCLOAK_IMAGE = 'quay.io/keycloak/keycloak@sha256:9409c59bdfb65dbffa20b11e6f18b8abb9281d480c7ca402f51ed3d5977e6007'  # 26.4.7
WIREMOCK_IMAGE = 'wiremock/wiremock@sha256:0d4ecb3e4dc8213fd7a4d37d6a78f6e6b553a6d2e15bd51b0999781282ac61b3'  # 3.13.2


def resources(a, postgres_image, secret_env):
    from playground import issuer
    output = []
    for component, enabled, image in [('wiremock', a.mock, WIREMOCK_IMAGE), ('keycloak', a.keycloak, KEYCLOAK_IMAGE)]:
        if not enabled: continue
        name = a.release + '-trial-' + component
        labels = {'vmcp.quickstart': a.release, 'app': name}
        container = dict(name=component, image=image, ports=[{'containerPort': 8080}],
            resources={'requests': {'cpu': '100m', 'memory': '256Mi'}, 'limits': {'cpu': '2', 'memory': '1Gi'}},
            readinessProbe={'httpGet': {'path': '/__admin/health' if component == 'wiremock' else '/realms/master', 'port': 8080}, 'periodSeconds': 5},
            startupProbe={'httpGet': {'path': '/__admin/health' if component == 'wiremock' else '/realms/master', 'port': 8080}, 'periodSeconds': 5, 'failureThreshold': 120})
        pod = {'automountServiceAccountToken': False, 'containers': [container]}
        if component == 'wiremock':
            container['args'] = ['--global-response-templating', '--disable-gzip']
        else:
            secret = a.release + '-quickstart'
            container['args'] = ['start-dev']
            container['env'] = [dict(name=k, value=v) for k, v in {
                'KC_DB': 'postgres', 'KC_DB_URL': f'jdbc:postgresql://{a.release}-trial-postgres:5432/keycloak',
                'KC_DB_USERNAME': 'vmcp_keycloak', 'KC_BOOTSTRAP_ADMIN_USERNAME': 'admin',
                'KC_HOSTNAME': issuer(a).split('/realms/')[0], 'KC_HOSTNAME_BACKCHANNEL_DYNAMIC': 'true',
                'KC_PROXY_HEADERS': 'xforwarded', 'KC_HTTP_ENABLED': 'true'}.items()]
            container['env'] += [secret_env('KC_DB_PASSWORD', secret, 'keycloak-db-password'),
                                 secret_env('KC_BOOTSTRAP_ADMIN_PASSWORD', secret, 'keycloak-admin-password')]
            # Creation only. Existing database/role credentials are not reset.
            script = '''set -eu
until pg_isready -h "$PGHOST" -U postgres >/dev/null 2>&1; do sleep 2; done
case "$KC_DB_PASSWORD" in *[!a-f0-9]*|'') exit 1;; esac
psql -v ON_ERROR_STOP=1 <<SQL
SELECT 'CREATE ROLE vmcp_keycloak LOGIN PASSWORD ' || quote_literal('$KC_DB_PASSWORD') WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname='vmcp_keycloak') \\gexec
SELECT 'CREATE DATABASE keycloak OWNER vmcp_keycloak' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname='keycloak') \\gexec
SQL
'''
            pod['initContainers'] = [dict(name='keycloak-database', image=postgres_image, command=['sh', '-ec', script],
                env=[dict(name='PGHOST', value=a.release + '-trial-postgres'), dict(name='PGUSER', value='postgres'), dict(name='PGDATABASE', value='postgres'),
                     secret_env('PGPASSWORD', secret, 'postgres-password'), secret_env('KC_DB_PASSWORD', secret, 'keycloak-db-password')],
                resources={'requests': {'cpu': '50m', 'memory': '64Mi'}, 'limits': {'cpu': '1', 'memory': '256Mi'}})]
        output.extend([dict(apiVersion='v1', kind='Service', metadata={'name': name}, spec={'selector': labels, 'ports': [{'name': 'http', 'port': 8080, 'targetPort': 8080}]}),
            dict(apiVersion='apps/v1', kind='Deployment', metadata={'name': name}, spec={'replicas': 1, 'strategy': {'type': 'Recreate'},
                'selector': {'matchLabels': labels}, 'template': {'metadata': {'labels': labels}, 'spec': pod}})])
    if a.keycloak and a.keycloak_host:
        spec = {'ingressClassName': a.ingress_class, 'rules': [{'host': a.keycloak_host, 'http': {'paths': [{'path': '/', 'pathType': 'Prefix',
                'backend': {'service': {'name': a.release + '-trial-keycloak', 'port': {'number': 8080}}}}]}}]}
        if a.tls_secret: spec['tls'] = [{'hosts': [a.keycloak_host], 'secretName': a.tls_secret}]
        output.append(dict(apiVersion='networking.k8s.io/v1', kind='Ingress', metadata={'name': a.release + '-trial-keycloak',
            'annotations': {'nginx.ingress.kubernetes.io/ssl-redirect': 'true' if a.tls_secret else 'false',
                            'nginx.ingress.kubernetes.io/force-ssl-redirect': 'true' if a.tls_secret else 'false'}}, spec=spec))
    return output
