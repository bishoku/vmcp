#!/usr/bin/env python3
"""Real Keycloak + two native control-plane processes against disposable PostgreSQL.

Runs sequentially after capacity tests. Uses only synthetic credentials and fresh
Docker containers; never connects to an existing application database or IdP.
This is authentication/recovery acceptance, not a Kubernetes rollout/HA benchmark.
"""
import argparse
import base64
import hashlib
import http.cookiejar
from html.parser import HTMLParser
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[2]


def run(args, **kwargs):
    try:
        return subprocess.run(args, check=True, text=True, **kwargs)
    except subprocess.CalledProcessError as error:
        raise RuntimeError(f'{args[0]} command failed with exit {error.returncode}; arguments omitted') from None


def port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


class LoginForm(HTMLParser):
    def __init__(self):
        super().__init__()
        self.action = None

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag == 'form' and attrs.get('id') == 'kc-form-login':
            self.action = attrs.get('action')


def request(url, method='GET', body=None, token=None, opener=None, headers=None):
    headers = dict(headers or {})
    if token:
        headers['Authorization'] = 'Bearer ' + token
    if body is not None:
        headers['Content-Type'] = 'application/merge-patch+json' if method == 'PATCH' else 'application/json'
        body = json.dumps(body).encode()
    req = urllib.request.Request(url, data=body, headers=headers, method=method)
    try:
        response = (opener or urllib.request.build_opener()).open(req, timeout=15)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read()
        try:
            value = json.loads(raw)
        except (ValueError, UnicodeDecodeError):
            value = raw.decode(errors='replace')
        return response.status, value


def wait(url, process=None, timeout=120):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if process is not None and process.poll() is not None:
            raise RuntimeError('control-plane exited before readiness; see process log')
        try:
            if request(url)[0] == 200:
                return
        except (OSError, urllib.error.URLError):
            pass
        time.sleep(.5)
    raise RuntimeError('readiness deadline exceeded: ' + url)


def login(base, username):
    # Keycloak treats loopback as a secure context; urllib otherwise withholds
    # its Secure authentication cookie over this local HTTP fixture.
    class LoopbackCookiePolicy(http.cookiejar.DefaultCookiePolicy):
        def return_ok_secure(self, cookie, request):
            if urllib.parse.urlparse(request.full_url).hostname == '127.0.0.1':
                return True
            return super().return_ok_secure(cookie, request)
    jar = http.cookiejar.CookieJar(policy=LoopbackCookiePolicy())
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
    status, html = request(base + '/auth/login', opener=opener)
    if status != 200 or not isinstance(html, str):
        raise RuntimeError('Keycloak authorization page not received')
    form = LoginForm()
    form.feed(html)
    if not form.action:
        raise RuntimeError('Keycloak password form missing')
    req = urllib.request.Request(form.action, data=urllib.parse.urlencode({
        'username': username, 'password': username, 'credentialId': ''}).encode())
    try:
        with opener.open(req, timeout=20) as response:
            response.read()
    except urllib.error.HTTPError as error:
        # Successful callback redirects to /, which the API-only server does not serve.
        if error.code != 404 or urllib.parse.urlparse(error.url).path != '/':
            body = error.read().decode(errors='replace')
            reason = next((s for s in ('sign-in expired', 'invalid state', 'Cookie not found', 'Invalid parameter', 'HTTPS required') if s.lower() in body.lower()), 'unclassified')
            raise RuntimeError(f'OIDC callback failed: HTTP {error.code}, path={urllib.parse.urlparse(error.url).path}, reason={reason}') from None
    if not any(cookie.name == 'vmcp_session_v2' for cookie in jar):
        raise RuntimeError('verified server session cookie missing')
    return opener, jar


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', required=True)
    parser.add_argument('--http-profile', action='store_true', help='also measure bounded scoped configuration/audit reads')
    args = parser.parse_args()
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=False)
    report = {'sourceRevision': run(['git', 'rev-parse', 'HEAD'], cwd=ROOT, capture_output=True).stdout.strip(),
              'profile': 'Keycloak authorization code + PKCE; two native CP replicas; shared PostgreSQL',
              'limitations': ['Not a Kubernetes Secret-store/RBAC or rolling Deployment drill',
                              'Synthetic local IdP; not an external production IdP'], 'checks': [], 'passed': False}
    source_digest = hashlib.sha256()
    for path in sorted((ROOT / 'core').rglob('*')):
        if path.is_file() and (path.suffix in ('.go', '.sql') or path.name in ('go.mod', 'go.sum')) and not any(part in ('node_modules', 'vendor') for part in path.parts):
            source_digest.update(str(path.relative_to(ROOT)).encode())
            source_digest.update(path.read_bytes())
    report['sourceFilesSHA256'] = source_digest.hexdigest()
    report['sourceFilesHashRecipe'] = 'sorted core .go/.sql/go.mod/go.sum; relative path followed by bytes; excludes vendor/node_modules'
    report['driverSHA256'] = {name: hashlib.sha256((Path(__file__).parent / name).read_bytes()).hexdigest() for name in ('authorization_acceptance.py', 'authorization_profile.py')}
    containers, processes, logs = [], [None, None], []
    suffix = secrets.token_hex(5)
    pg_name, kc_name = 'vmcp-authz-pg-' + suffix, 'vmcp-authz-idp-' + suffix
    pg_port, kc_port, *cp_ports = [port() for _ in range(4)]
    bases = ['http://127.0.0.1:' + str(p) for p in cp_ports]
    issuer = 'http://127.0.0.1:' + str(kc_port) + '/realms/vmcp'
    bootstrap = secrets.token_urlsafe(32)
    db_password = secrets.token_urlsafe(24)
    master = base64.b64encode(secrets.token_bytes(32)).decode()

    def check(name, actual, expected):
        ok = actual == expected
        report['checks'].append({'name': name, 'passed': ok})
        print(('PASS ' if ok else 'FAIL ') + name, flush=True)
        if not ok:
            raise AssertionError(name + ': unexpected result (credentials/response omitted)')

    def api(index, path, method='GET', body=None, token=None, opener=None, headers=None):
        return request(bases[index] + '/api/v1' + path, method, body, token, opener, headers)

    def stop(index):
        process = processes[index]
        if process is not None:
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=20)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
            processes[index] = None

    try:
        with tempfile.TemporaryDirectory(prefix='vmcp-authz-acceptance-') as temp:
            temp = Path(temp)
            binary = temp / 'control-plane'
            run(['go', 'build', '-trimpath', '-o', str(binary), './cmd/control-plane'], cwd=ROOT / 'core')
            report['binarySHA256'] = hashlib.sha256(binary.read_bytes()).hexdigest()
            realm = json.loads((ROOT / 'benchmark/keycloak/vmcp-realm.json').read_text())
            client = next(c for c in realm['clients'] if c['clientId'] == 'vmcp')
            client['redirectUris'] = [bases[0] + '/auth/callback']
            realm_path = temp / 'realm.json'
            realm_path.write_text(json.dumps(realm))
            realm_path.chmod(0o644)
            # Directory traversal is needed by the unprivileged Keycloak container.
            temp.chmod(0o755)
            containers.append(pg_name)
            run(['docker', 'run', '-d', '--name', pg_name, '-p', f'127.0.0.1:{pg_port}:5432',
                 '-e', 'POSTGRES_PASSWORD=' + db_password, '-e', 'POSTGRES_DB=acceptance', 'postgres:17-alpine'], capture_output=True)
            containers.append(kc_name)
            run(['docker', 'run', '-d', '--name', kc_name, '-p', f'127.0.0.1:{kc_port}:8080',
                 '-v', str(realm_path) + ':/opt/keycloak/data/import/realm.json:ro',
                 'quay.io/keycloak/keycloak:26.8', 'start-dev', '--import-realm'], capture_output=True)
            report['images'] = {image: json.loads(run(['docker', 'image', 'inspect', image], capture_output=True).stdout)[0]['Id'] for image in ('postgres:17-alpine', 'quay.io/keycloak/keycloak:26.8')}
            wait(issuer + '/.well-known/openid-configuration', timeout=180)
            for _ in range(60):
                if subprocess.run(['docker', 'exec', pg_name, 'pg_isready', '-U', 'postgres'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
                    break
                time.sleep(.5)
            else:
                raise RuntimeError('PostgreSQL not ready')
            env = {k: v for k, v in os.environ.items() if not k.startswith('VMCP_')}
            env.update(VMCP_DATABASE_URL=f'postgres://postgres:{db_password}@127.0.0.1:{pg_port}/acceptance?sslmode=disable',
                       VMCP_MASTER_KEY=master, VMCP_ADMIN_TOKEN=bootstrap, VMCP_TENANT_ID='default',
                       VMCP_OIDC_ISSUER=issuer, VMCP_OIDC_CLIENT_ID='vmcp', VMCP_OIDC_CLIENT_SECRET='vmcp-dev-secret',
                       VMCP_OIDC_REDIRECT_URL=bases[0] + '/auth/callback', VMCP_OIDC_ADMIN_GROUPS='vmcp-admins',
                       VMCP_EGRESS_ALLOW_CIDRS='127.0.0.0/8', GOMAXPROCS='2')

            def start(index):
                log = (output / f'control-plane-{index}-{len(logs)}.log').open('w')
                logs.append(log)
                processes[index] = subprocess.Popen([str(binary)], env={**env, 'VMCP_CP_ADDR': '127.0.0.1:' + str(cp_ports[index])}, stdout=log, stderr=subprocess.STDOUT)
                wait(bases[index] + '/readyz', processes[index])

            start(0)
            start(1)
            admin, admin_jar = login(bases[0], 'alice')
            for i in range(2):
                status, me = api(i, '/me', opener=admin)
                check(f'IdP admin identity on replica {i}', (status, me.get('role'), me.get('via')), (200, 'admin', 'oidc'))
                check(f'IdP admin management on replica {i}', api(i, '/roles', opener=admin)[0], 200)
            check('OIDC mutation requires CSRF', api(1, '/roles', 'POST', {'slug': 'csrf-denied', 'name': 'Denied', 'permissions': []}, opener=admin)[0], 403)
            status, role = api(0, '/roles', 'POST', {'slug': 'idp-reader', 'name': 'IdP reader', 'permissions': ['upstreams.read']}, token=bootstrap)
            check('create narrow role', status, 201)
            mapping_body = {'issuer': issuer, 'principalKind': 'oidc', 'conditions': [{'path': 'groups', 'operator': 'array_contains', 'value': 'vmcp-editors'}], 'enabled': True}
            status, mapping = api(0, '/identity-mappings', 'POST', mapping_body, token=bootstrap)
            check('create verified claim mapping', status, 201)
            grant_body = {'roleId': role['id'], 'mappingId': mapping['id'], 'scope': {'kind': 'tenant'}, 'captureScopes': [], 'enabled': True}
            status, grant = api(0, '/role-assignments', 'POST', grant_body, token=bootstrap)
            check('create mapping grant', status, 201)
            reader, _ = login(bases[0], 'bob')
            for i in range(2):
                check(f'custom-only IdP login reads on replica {i}', api(i, '/upstreams', opener=reader)[0], 200)
                check(f'custom role cannot administer on replica {i}', api(i, '/roles', opener=reader)[0], 403)
            grant_body['enabled'] = False
            check('disable mapping grant', api(0, '/role-assignments/' + grant['id'], 'PUT', grant_body, token=bootstrap, headers={'If-Match': '"1"'})[0], 200)
            for i in range(2):
                check(f'grant revocation visible on replica {i}', api(i, '/upstreams', opener=reader)[0], 403)
            # Preserve a copy to verify server-side invalidation, not just cookie deletion.
            saved_cookie = '; '.join(c.name + '=' + c.value for c in admin_jar if c.name == 'vmcp_session_v2')
            status, _ = request(bases[1] + '/auth/logout', 'POST', opener=admin, headers={'X-VMCP-CSRF': '1'})
            check('logout through second replica', status, 200)
            for i in range(2):
                check(f'logged-out session rejected on replica {i}', api(i, '/roles', headers={'Cookie': saved_cookie})[0], 401)
            status, named = api(0, '/admin-tokens', 'POST', {'slug': 'recovery-named', 'scope': 'admin'}, token=bootstrap)
            check('create named administrator', status, 201)
            old_bootstrap = bootstrap
            bootstrap = secrets.token_urlsafe(32)
            env['VMCP_ADMIN_TOKEN'] = bootstrap
            for i in range(2):
                stop(i)
                start(i)
                check(f'old bootstrap rejected after restart {i}', api(i, '/roles', token=old_bootstrap)[0], 401)
                check(f'replacement bootstrap works on replica {i}', api(i, '/roles', token=bootstrap)[0], 200)
                check(f'named token independent of bootstrap rotation {i}', api(i, '/roles', token=named['token'])[0], 200)
            check('revoke last named admin', api(0, '/admin-tokens/recovery-named/revoke', 'POST', token=bootstrap)[0], 200)
            for i in range(2):
                check(f'named revocation visible on replica {i}', api(i, '/roles', token=named['token'])[0], 401)
            oidc_env = {key: value for key, value in env.items() if key.startswith('VMCP_OIDC_')}
            # Simulate unavailable IdP, then the documented deployment-controlled token-only recovery.
            for i in range(2):
                stop(i)
            run(['docker', 'stop', kc_name], capture_output=True)
            for key in list(env):
                if key.startswith('VMCP_OIDC_'):
                    del env[key]
            for i in range(2):
                start(i)
                check(f'token-only recovery without IdP on replica {i}', api(i, '/roles', token=bootstrap)[0], 200)
                check(f'revoked named token remains rejected {i}', api(i, '/roles', token=named['token'])[0], 401)
            check('recovery can repair roles', api(1, '/roles', 'POST', {'slug': 'recovered', 'name': 'Recovered', 'permissions': ['upstreams.read']}, token=bootstrap)[0], 201)
            run(['docker', 'start', kc_name], capture_output=True)
            wait(issuer + '/.well-known/openid-configuration', timeout=180)
            temporary_bootstrap = bootstrap
            bootstrap = secrets.token_urlsafe(32)
            env.update(oidc_env)
            env['VMCP_ADMIN_TOKEN'] = bootstrap
            for i in range(2):
                stop(i)
                start(i)
                check(f'temporary recovery credential retired on replica {i}', api(i, '/roles', token=temporary_bootstrap)[0], 401)
                check(f'final bootstrap works on replica {i}', api(i, '/roles', token=bootstrap)[0], 200)
            restored, _ = login(bases[0], 'alice')
            for i in range(2):
                check(f'normal IdP administration restored on replica {i}', api(i, '/roles', opener=restored)[0], 200)
                check(f'old logged-out session remains revoked after recovery {i}', api(i, '/roles', headers={'Cookie': saved_cookie})[0], 401)
            if args.http_profile:
                from authorization_profile import run_profile
                def sql(query):
                    return run(['docker', 'exec', pg_name, 'psql', '-U', 'postgres', '-d', 'acceptance', '-At', '-v', 'ON_ERROR_STOP=1', '-c', query], capture_output=True).stdout
                run_profile(api, sql, check, report, bootstrap, issuer)
            report['passed'] = True
    except Exception as error:
        report['error'] = type(error).__name__ + (': command failed (arguments omitted)' if isinstance(error, subprocess.CalledProcessError) else ': ' + str(error))
        raise
    finally:
        for i in range(2):
            stop(i)
        for name in reversed(containers):
            subprocess.run(['docker', 'rm', '-f', '-v', name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        for log in logs:
            log.close()
        (output / 'report.json').write_text(json.dumps(report, indent=2) + '\n')


if __name__ == '__main__':
    main()
