"""Console grant provisioning must preserve user edits and runtime separation."""
import copy
from types import SimpleNamespace
import unittest
from unittest.mock import patch

from pathlib import Path
import sys
sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "deploy/helm/vmcp/quickstart"))
import playground


class FakeAPI:
    def __init__(self):
        self.data = {
            '/api/v1/roles': [dict(slug=s, id=s) for s in ['observer', 'upstream-maintainer', 'virtual-server-editor', 'credential-manager', 'access-approver', 'auditor']],
            '/api/v1/identity-mappings': [], '/api/v1/role-assignments': [],
            '/api/v1/teams': [dict(slug='one', group='one', issuer='http://localhost:8180/realms/vmcp-demo')],
            '/api/v1/access/scopes?kind=team': [dict(slug='one', id='team-uuid')],
        }
        self.calls = []

    def items(self, path): return copy.deepcopy(self.data[path])

    def request(self, method, path, body):
        assert method == 'POST'
        self.calls.append((path, copy.deepcopy(body)))
        result = dict(body, id=str(len(self.calls)))
        self.data[path].append(result)
        return copy.deepcopy(result)


class AuthorizationSeedTests(unittest.TestCase):
    def setUp(self):
        self.api, self.ledger = FakeAPI(), {}
        self.args = SimpleNamespace(tls_secret=None, keycloak_host=None, keycloak_port=8180)

    def seed(self):
        with patch.dict(playground.catalog.TEAM, {'one': {}}, clear=True):
            playground.seed_authorization(self.api, self.args, self.ledger, lambda _: None)

    def test_scoped_explicit_grants_and_metadata_only_auditor(self):
        self.seed()
        grants = self.api.data['/api/v1/role-assignments']
        self.assertEqual(len(grants), 6)
        for grant in grants:
            self.assertEqual(grant['captureScopes'], [])
            self.assertEqual(grant['scope'], {'kind': 'tenant'} if grant['roleId'] == 'auditor' else {'kind': 'team', 'id': 'team-uuid'})
        for mapping in self.api.data['/api/v1/identity-mappings']:
            self.assertEqual(mapping['issuer'], playground.issuer(self.args))
            self.assertEqual(mapping['conditions'][0]['operator'], 'array_contains')
        self.assertFalse(any('identity-bindings' in path for path, _ in self.api.calls))
        self.seed()
        self.assertEqual(len(self.api.calls), 8)

    def test_deleted_disabled_and_edited_grants_are_not_restored(self):
        self.seed()
        self.api.data['/api/v1/role-assignments'][0]['enabled'] = False
        self.api.data['/api/v1/role-assignments'].pop(1)
        self.api.data['/api/v1/identity-mappings'][0]['conditions'][0]['value'] = 'edited'
        self.seed()
        self.assertEqual(len(self.api.calls), 8)
        self.assertFalse(self.api.data['/api/v1/role-assignments'][0]['enabled'])

    def test_different_team_issuer_does_not_receive_team_grants(self):
        self.api.data['/api/v1/teams'][0]['issuer'] = 'https://other.example'
        self.seed()
        self.assertEqual([a['roleId'] for a in self.api.data['/api/v1/role-assignments']], ['auditor'])

    def test_edited_team_group_does_not_get_an_implicit_old_group_mapping(self):
        self.api.data['/api/v1/teams'][0]['group'] = 'changed-by-admin'
        self.seed()
        self.assertEqual([a['roleId'] for a in self.api.data['/api/v1/role-assignments']], ['auditor'])

    def test_query_pagination_preserves_scope_kind(self):
        api = playground.API('http://unused')
        seen = []
        def request(method, path):
            seen.append(path)
            return {'items': [], 'nextCursor': 'next'} if len(seen) == 1 else {'items': []}
        api.request = request
        api.items('/api/v1/access/scopes?kind=team')
        self.assertEqual(seen[-1], '/api/v1/access/scopes?kind=team&cursor=next')


if __name__ == '__main__': unittest.main()
