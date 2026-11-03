"""Import create-only playground fixtures into an existing stack; no Helm/kubectl."""
import argparse
import json
import os
from pathlib import Path
import sys
import tempfile
from types import SimpleNamespace
import urllib.error
import urllib.parse

import playground


def parser():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--api-url', default=os.environ.get('VMCP_API_URL'),
                   help='Control-plane base URL (or VMCP_API_URL)')
    p.add_argument('--state-file', type=Path, default=Path('.playground-import-state.json'),
                   help='Private runtime keys and resume ledger; use one file per stack')
    p.add_argument('--omni', action='store_true', help='Publish newly created servers to Omni and request reindex')
    p.add_argument('--issuer', default='http://localhost:8803/realms/vmcp-demo',
                   help='Team issuer URL; does not install Keycloak or create SSO bindings')
    return p


def persist(path, state):
    """Atomically persist each one-time key with owner-only file permissions."""
    name = None
    try:
        with tempfile.NamedTemporaryFile(mode='w', dir=path.parent, delete=False) as f:
            name = f.name
            json.dump(state, f, indent=2)
            f.flush()
            os.fsync(f.fileno())
        os.replace(name, path)
    finally:
        if name and os.path.exists(name):
            os.unlink(name)


def main(argv=None):
    p = parser()
    a = p.parse_args(argv)
    base = (a.api_url or '').rstrip('/')
    url = urllib.parse.urlsplit(base)
    if url.scheme not in ('http', 'https') or not url.netloc or url.username or url.password or url.query or url.fragment:
        p.error('--api-url must be an HTTP(S) base URL without credentials, query or fragment')
    token = os.environ.get('VMCP_ADMIN_TOKEN', '')
    if not token:
        p.error('set VMCP_ADMIN_TOKEN to the existing stack admin token')
    path = a.state_file.expanduser().resolve()
    state = json.loads(path.read_text()) if path.exists() else {'api_url': base, 'keys': {}, 'ledger': {}}
    if not isinstance(state, dict) or state.get('api_url') != base:
        raise ValueError('State file belongs to a different API URL; use a separate --state-file per stack')
    if not isinstance(state.get('keys'), dict) or not isinstance(state.get('ledger'), dict):
        raise ValueError('Invalid import state: keys and ledger must be objects')
    api = playground.API(base, token)
    provider = api.request('GET', '/api/v1/mock-provider')
    if not provider.get('enabled') or not provider.get('url'):
        raise ValueError('Configure VMCP_WIREMOCK_URL on the existing control plane before importing')
    # Check local persistence before creating any remote resources.
    persist(path, state)

    def save_key(slug, key):
        state['keys'][slug] = key
        persist(path, state)

    def save_ledger(ledger):
        state['ledger'] = ledger
        persist(path, state)

    args = SimpleNamespace(release='existing', wiremock_url=provider['url'], omni=a.omni,
                           keycloak=False, keycloak_host=None, keycloak_port=8803,
                           tls_secret=None, team_issuer=a.issuer)
    playground.seed(api, args, save_key, state['ledger'], save_ledger)
    if a.omni:
        api.request('POST', '/api/v1/omni/reindex', {})
    print(f'Runtime keys and resume ledger saved to {path}; keep this file private.')
    return 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (ValueError, OSError, urllib.error.URLError) as exc:
        print(f'Import failed: {exc}', file=sys.stderr)
        sys.exit(1)
