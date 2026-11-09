import type { AccessData } from './state'
export const ids = {
  admin: '10000000-0000-0000-0000-000000000001',
  observer: '10000000-0000-0000-0000-000000000002',
  custom: '10000000-0000-0000-0000-000000000003',
  assignment: '20000000-0000-0000-0000-000000000001',
  mapping: '30000000-0000-0000-0000-000000000001',
  token: '40000000-0000-0000-0000-000000000001',
  resource: '50000000-0000-0000-0000-000000000001',
  oldEpoch: '60000000-0000-0000-0000-000000000001',
  newEpoch: '60000000-0000-0000-0000-000000000002',
  owner: '70000000-0000-0000-0000-000000000001',
  newOwner: '70000000-0000-0000-0000-000000000002',
  event: '80000000-0000-0000-0000-000000000001',
}
export const accessFixture: AccessData = {
  permissions: [
    { id: 'upstreams.read', scopes: ['tenant', 'team', 'upstream'], captureRestricted: false },
    {
      id: 'audit.events.read',
      scopes: ['tenant', 'team', 'project', 'upstream', 'virtual_server', 'unmanaged'],
      captureRestricted: false,
    },
    {
      id: 'audit.payloads.request.read',
      scopes: ['tenant', 'team', 'project', 'upstream', 'virtual_server', 'unmanaged'],
      captureRestricted: true,
    },
    {
      id: 'audit.payloads.response.read',
      scopes: ['tenant', 'team', 'project', 'upstream', 'virtual_server', 'unmanaged'],
      captureRestricted: true,
    },
  ],
  roles: [
    {
      id: ids.admin,
      slug: 'platform-operator',
      name: 'Platform Operator',
      description: 'Protected built-in',
      builtin: true,
      version: 1,
      permissions: [
        'upstreams.read',
        'audit.events.read',
        'audit.payloads.request.read',
        'audit.payloads.response.read',
      ],
    },
    {
      id: ids.observer,
      slug: 'observer',
      name: 'Observer',
      description: 'Read configuration',
      builtin: true,
      version: 1,
      permissions: ['upstreams.read'],
    },
    {
      id: ids.custom,
      slug: 'request-reader',
      name: 'Request reader',
      description: 'Request only',
      builtin: false,
      version: 1,
      permissions: ['audit.events.read', 'audit.payloads.request.read'],
    },
  ],
  mappings: [
    {
      id: ids.mapping,
      issuer: 'https://trusted.example',
      principalKind: 'oidc',
      conditions: [{ path: 'scope', operator: 'scope_contains', value: 'audit:read' }],
      enabled: true,
      version: 1,
    },
  ],
  assignments: [
    {
      id: ids.assignment,
      roleId: ids.custom,
      principal: { kind: 'oidc', issuer: 'https://trusted.example', id: 'original-subject' },
      scope: { kind: 'upstream', id: ids.resource, epoch: ids.oldEpoch, ownerTeam: ids.owner },
      captureScopes: ['control'],
      enabled: true,
      version: 1,
    },
  ],
  status: {
    authorizationRevision: 7,
    schemaVersion: 39,
    trustedIssuer: 'https://trusted.example',
    claimMappings: 'live-audit',
    externalOAuthBearer: false,
    blockers: ['untrustedEnabledMappings'],
    counts: { roles: 3, customRoles: 1, assignments: 1, enabledMappings: 1, legacyAuditEvents: 4 },
  },
}
