"""Bounded, closed-loop HTTP diagnostic for granular control-plane read paths.

Called only by authorization_acceptance.py against its disposable database.
Results are diagnostic service latencies, not open-loop capacity or an SLA.
"""
from concurrent.futures import ThreadPoolExecutor
import json
import math
import time


def measure(call, duration=10, concurrency=8):
    deadline = time.monotonic() + duration
    started = time.monotonic()

    def worker(_):
        samples, errors = [], 0
        while time.monotonic() < deadline:
            begin = time.monotonic()
            try:
                valid = call()
            except Exception:
                valid = False
            elapsed = (time.monotonic() - begin) * 1000
            if valid:
                samples.append(elapsed)
            else:
                errors += 1
        return samples, errors

    with ThreadPoolExecutor(max_workers=concurrency) as executor:
        results = list(executor.map(worker, range(concurrency)))
    elapsed = time.monotonic() - started
    samples = sorted(value for values, _ in results for value in values)
    errors = sum(count for _, count in results)
    def percentile(q):
        return samples[max(0, math.ceil(len(samples) * q) - 1)] if samples else None
    return {'durationSeconds': elapsed, 'concurrency': concurrency, 'successful': len(samples),
            'errors': errors, 'successfulRequestsPerSecond': len(samples) / elapsed,
            'p50Ms': percentile(.5), 'p95Ms': percentile(.95), 'p99Ms': percentile(.99),
            'passed': len(samples) > 0 and errors == 0}


def run_profile(api, sql, check, report, bootstrap, issuer):
    # All identifiers below are fixture constants or UUIDs returned by this isolated API.
    def admin(path, method='GET', body=None):
        return api(0, path, method, body, token=bootstrap)

    for name in ('profile-a', 'profile-b'):
        check('create profile team ' + name, admin('/teams', 'POST', {'slug': name, 'name': name, 'issuer': issuer, 'group': name})[0], 200)
    team = sql("SELECT id FROM teams WHERE slug='profile-a'").strip()
    status, settings = admin('/audit-settings')
    check('read capture settings', status, 200)
    settings['mode'] = 'strict'
    settings['scopes']['control'] = {'request': True, 'response': True}
    check('enable control payload capture', admin('/audit-settings', 'PUT', settings)[0], 200)
    for i in range(20):
        status, _ = admin('/upstreams', 'POST', {'slug': f'profile-{i:02}', 'name': f'Profile {i}',
            'url': 'http://127.0.0.1:9/mcp', 'enabled': False, 'ownerTeam': 'profile-a' if i < 10 else 'profile-b'})
        if status != 201:
            raise AssertionError('profile upstream seed failed')
    for i in range(20):
        status, _ = admin(f'/upstreams/profile-{i:02}', 'PATCH', {'description': 'synthetic-profile-message'})
        if status != 200:
            raise AssertionError('profile audit seed failed')
    status, credential = admin('/admin-tokens', 'POST', {'slug': 'profile-reader', 'scope': 'read'})
    check('create profile reader', status, 201)
    who = sql("SELECT id FROM admin_tokens WHERE slug='profile-reader'").strip()
    permissions = ['upstreams.read', 'audit.events.read', 'audit.identities.read', 'audit.details.read', 'audit.payloads.request.read', 'audit.payloads.response.read']
    status, role = admin('/roles', 'POST', {'slug': 'profile-reader', 'name': 'Profile reader', 'permissions': permissions})
    check('create profile permissions', status, 201)
    status, grant = admin('/role-assignments', 'POST', {'roleId': role['id'], 'principal': {'kind': 'control_token', 'id': who}, 'scope': {'kind': 'team', 'id': team}, 'captureScopes': ['control'], 'enabled': True})
    check('grant team-scoped profile access', status, 201)
    secret = credential['token']
    def read(path):
        return api(0, path, token=secret)
    check('cross-team profile detail denied', read('/upstreams/profile-10')[0], 403)
    status, rows = read('/upstreams?limit=200')
    check('profile list excludes other team', (status, len(rows.get('items', []))), (200, 10))
    # Audit persistence may finish asynchronously after the response.
    events = {}
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        status, page = read('/audit-events?activity=control&limit=50')
        if status == 200:
            for event in page['items']:
                if event.get('payloadState') == 'captured' and event.get('direction') in ('request', 'response') and event.get('action', '').startswith('PATCH ') and '/upstreams' in event.get('action', ''):
                    events[event['direction']] = event['eventId']
        if len(events) == 2:
            break
        time.sleep(.2)
    check('captured request and response fixtures visible', len(events), 2)
    report['httpProfile'] = {'mode': 'closed-loop diagnostic; fresh connection per request; one native CP process GOMAXPROCS=2',
        'inventory': {'teams': 2, 'upstreams': 20, 'visibleUpstreams': 10},
        'audit': 'strict, control payload capture enabled; read capture disabled',
        'threshold': 'all responses correct, no HTTP/transport/validation errors; latency descriptive only',
        'notMeasured': ['open-loop saturation', 'large audit history', 'production ingress', 'payload capture throughput'], 'stages': []}
    def list_ok():
        status, body = read('/upstreams?limit=200')
        return status == 200 and len(body.get('items', [])) == 10 and all(int(v['slug'][-2:]) < 10 for v in body['items'])
    def audit_ok():
        status, body = read('/audit-events?activity=control&limit=50')
        return status == 200 and bool(body.get('items')) and all(e.get('capabilities', {}).get('canReadMetadata') for e in body['items'])
    def payload_ok(direction):
        status, body = read('/audit-events/' + events[direction] + '/payload')
        return status == 200 and body.get('state') == 'captured' and 'synthetic-profile-message' in json.dumps(body)
    paths = {'scoped-list': list_ok, 'scoped-detail': lambda: read('/upstreams/profile-00')[0] == 200,
             'cross-team-denial': lambda: read('/upstreams/profile-10')[0] == 403,
             'audit-list-details': audit_ok,
             'request-payload': lambda: payload_ok('request'), 'response-payload': lambda: payload_ok('response')}
    for count in (10, 1000):
        # Unrelated subjects stress full-policy loading without increasing this caller's rights.
        sql("DELETE FROM authorization_assignments WHERE principal_id LIKE 'profile-noise-%'; "
            "INSERT INTO authorization_assignments(role_id,principal_kind,issuer,principal_id,scope_kind,scope_id) "
            f"SELECT '{role['id']}'::uuid,'oidc','{issuer}','profile-noise-'||i,'team','{team}'::uuid FROM generate_series(1,{count}) i")
        stage = {'unrelatedAssignments': count, 'totalAssignments': int(sql('SELECT count(*) FROM authorization_assignments').strip()), 'paths': {}}
        report['httpProfile']['stages'].append(stage)
        for name, call in paths.items():
            result = measure(call)
            stage['paths'][name] = result
            check(f'HTTP profile {name}, unrelated assignments={count}', result['passed'], True)
    check('revoke profile grant', api(0, '/role-assignments/' + grant['id'], 'DELETE', token=bootstrap, headers={'If-Match': '"1"'})[0], 204)
    for i in range(2):
        check(f'profile payload revoked on replica {i}', api(i, '/audit-events/' + events['request'] + '/payload', token=secret)[0], 404)
