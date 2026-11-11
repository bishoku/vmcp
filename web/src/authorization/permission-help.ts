/** UI guidance only. The server catalog remains authoritative for grants and scopes. */
export type PermissionHelp = { title: string; description: string }
const help: Record<string, PermissionHelp> = {}
function group(namespace: string, entries: Record<string, [string, string]>) {
  for (const [action, [title, description]] of Object.entries(entries)) {
    help[`${namespace}.${action}`] = { title, description }
  }
}
group('admission', {
  read: ['View concurrency limits', 'View per-pod total, server and principal concurrency limits.'],
  update: [
    'Change concurrency limits',
    'Change per-pod admission limits and therefore how much work the gateway accepts.',
  ],
})
group('platform', {
  'info.read': ['View platform information', 'View platform version and deployment information.'],
})
group('invocation_security', {
  read: ['View global invocation security', 'View global verification and approval configuration.'],
  manage: [
    'Manage global invocation security',
    'Change global invocation verification and approval configuration.',
  ],
})
group('teams', {
  read: ['View teams', 'List teams and view their configuration.'],
  manage: ['Manage teams', 'Create, change and remove teams within the assigned scope.'],
})
group('projects', {
  read: ['View projects', 'List projects and view their team ownership.'],
  manage: ['Manage projects', 'Create, change and remove projects within the assigned scope.'],
})
group('ownership', {
  transfer: [
    'Transfer ownership',
    'Move resources between owners or projects, changing which scoped assignments apply.',
  ],
})
group('upstreams', {
  read: ['View upstreams', 'View upstream configuration; stored secrets remain write-only.'],
  create: ['Create upstreams', 'Register upstream connections within the assigned scope.'],
  update: [
    'Edit upstream details',
    'Change upstream configuration subject to additional field-specific permissions.',
  ],
  delete: ['Delete upstreams', 'Remove upstream connections within the assigned scope.'],
  'connection.update': [
    'Change upstream destinations',
    'Change connection URLs, fallback URLs, transport and upstream kind.',
  ],
  'activation.update': ['Enable or disable upstreams', 'Change whether an upstream is active.'],
  'credentials.update': [
    'Change upstream credentials',
    'Replace authentication credentials; this does not reveal stored secrets.',
  ],
  'tls.update': ['Change upstream TLS', 'Change certificate and TLS verification settings.'],
  'identity_forwarding.update': [
    'Change forwarded identity',
    'Change how caller identity is forwarded to an upstream.',
  ],
  'invocation_security.read': [
    'View upstream invocation security',
    'View verification, approval and execution policies for an upstream and its tools.',
  ],
  'invocation_security.manage': [
    'Manage upstream invocation security',
    'Change verification, approval and execution policies for an upstream and its tools.',
  ],
  'rest.manage': [
    'Manage REST operations',
    'Configure REST operations and their generated MCP tools.',
  ],
  'catalog.read': [
    'View discovered upstream tools',
    'Inspect the discovered tool catalog for an upstream.',
  ],
  'catalog.refresh': [
    'Refresh upstream tools',
    'Contact an upstream to refresh its discovered tool catalog.',
  ],
  test: ['Test saved upstreams', 'Send connection tests to an existing upstream.'],
  'rest.preview': ['Preview REST requests', 'Preview the request generated from a REST operation.'],
  'rest.execute': [
    'Execute REST requests',
    'Send a real REST operation to an upstream; it may have side effects.',
  ],
  'mock.read': [
    'View upstream mock configuration',
    'View the mock configuration associated with an upstream.',
  ],
  'mock.sync': [
    'Synchronize upstream mocks',
    'Synchronize upstream mock definitions with the configured mock provider.',
  ],
})
group('virtual_servers', {
  read: ['View virtual servers', 'View virtual-server configuration and exposed tools.'],
  create: ['Create virtual servers', 'Create virtual MCP servers within the assigned scope.'],
  update: [
    'Edit virtual servers',
    'Change virtual-server configuration subject to field-specific permissions.',
  ],
  delete: ['Delete virtual servers', 'Remove virtual MCP servers within the assigned scope.'],
  'publication.update': [
    'Change server publication',
    'Change whether a virtual server is published for discovery.',
  ],
  'policy.update': [
    'Change server policies',
    'Change virtual-server tool exposure and execution policies.',
  ],
  'client_features.update': [
    'Change client features',
    'Change the MCP client features allowed by a virtual server.',
  ],
})
for (const [ns, noun] of [
  ['agent_keys', 'agent keys'],
  ['identity_bindings', 'identity bindings'],
  ['control_tokens', 'control tokens'],
]) {
  group(ns, {
    read: [`View ${noun}`, `View ${noun} and their configuration; stored secrets remain hidden.`],
    create: [`Create ${noun}`, `Create ${noun} within the assigned scope.`],
    update: [`Edit ${noun}`, `Change ${noun} subject to additional field-specific permissions.`],
    delete: [`Delete ${noun}`, `Remove ${noun} within the assigned scope.`],
    'access.update': [
      `Change access for ${noun}`,
      `Change the access configuration attached to ${noun}.`,
    ],
    'constraints.update': [
      `Change constraints for ${noun}`,
      `Change restrictions on where and how ${noun} may be used.`,
    ],
  })
}
for (const ns of ['agent_keys', 'control_tokens']) {
  const noun = ns === 'agent_keys' ? 'agent keys' : 'control tokens'
  group(ns, {
    rotate: [`Rotate ${noun}`, `Issue replacement credentials for ${noun}.`],
    revoke: [`Revoke ${noun}`, `Revoke ${noun}, preventing further authenticated use.`],
  })
}
group('agent_keys', {
  'audit_correlation.update': [
    'Change agent audit correlation',
    'Change how agent-key activity can be correlated in audit records.',
  ],
})
group('identity_bindings', {
  'rules.update': [
    'Change identity matching rules',
    'Change the identity claims used to match a binding.',
  ],
  evaluate: ['Evaluate identity bindings', 'Test identity-binding rules against supplied claims.'],
})
group('tool_catalog', {
  read: ['View available tools', 'Browse the tool catalog within the assigned scope.'],
})
group('tool_grants', {
  read: ['View tool grants', 'View tool access grants and requests.'],
  request: ['Request tool access', 'Submit requests for access to tools.'],
  approve: ['Approve tool access', 'Approve tool access requests within the assigned scope.'],
  reject: ['Reject tool access', 'Reject pending tool access requests.'],
  revoke: ['Revoke tool access', 'Remove previously granted tool access.'],
})
group('connections', {
  test_unsaved: [
    'Test unsaved connections',
    'Contact a destination using connection settings that have not been saved.',
  ],
})
group('rest', {
  import: ['Import REST definitions', 'Import API definitions to prepare REST operations.'],
})
group('mock.provider', {
  read: ['View mock provider', 'View the configured mock provider information.'],
})
group('runtime_jwts', {
  revoke: ['Revoke runtime tokens', 'Revoke runtime JWTs to prevent their further use.'],
})
group('sessions', {
  revoke: ['Revoke sessions', 'Invalidate sessions so they can no longer be used.'],
})
group('omni', {
  'status.read': ['View Omni status', 'View the Omni discovery service status.'],
  'status.details.read': [
    'View detailed Omni status',
    'Inspect detailed Omni indexing and provider status.',
  ],
  preview: ['Preview Omni discovery', 'Run a discovery preview against the Omni tool index.'],
  reindex: [
    'Rebuild the Omni index',
    'Trigger a refresh of the Omni discovery index and its embeddings.',
  ],
})
group('config', {
  export: [
    'Export configuration',
    'Export configuration through the GitOps API; secret visibility remains restricted.',
  ],
  validate: [
    'Validate configuration',
    'Check a proposed GitOps configuration without applying it.',
  ],
  apply: [
    'Apply configuration',
    'Apply GitOps configuration subject to resource and field permissions.',
  ],
  prune: [
    'Prune configuration',
    'Remove resources omitted from a GitOps configuration when pruning is requested.',
  ],
})
for (const family of ['roles', 'assignments', 'mappings']) {
  group(`access.${family}`, {
    read: [
      `View access ${family}`,
      `View Access Management ${family}. These endpoints still require platform administrator access.`,
    ],
    manage: [
      `Manage access ${family}`,
      `Create, change and remove Access Management ${family}. This permission alone does not delegate platform administrator access.`,
    ],
  })
}
group('access', {
  evaluate: [
    'Simulate access decisions',
    'Evaluate sample access decisions without granting access or revealing payloads. Platform administrator access is still required.',
  ],
})
group('audit', {
  'events.read': [
    'View audit events',
    'View event metadata, timelines and summaries. Does not reveal request or response payloads.',
  ],
  'identities.read': [
    'View audit identities',
    'View and filter actor, subject, agent and client IP details. Does not reveal payloads.',
  ],
  'details.read': [
    'View audit event details',
    'Inspect event details subject to separate identity and payload permissions.',
  ],
  'settings.read': [
    'View audit settings',
    'View capture, masking, retention and other audit configuration. Does not grant payload access.',
  ],
  'redaction.local.update': [
    'Change local audit masking',
    'Change audit masking rules for resources within the assigned scope.',
  ],
  'redaction.preview': [
    'Preview audit masking',
    'Test how audit masking rules transform sample content.',
  ],
  'status.read': [
    'View audit storage status',
    'View audit storage, durability and operational status.',
  ],
  'durability.update': [
    'Change audit durability',
    'Change audit persistence and durability settings.',
  ],
  'retention.update': ['Change audit retention', 'Change how long audit records are retained.'],
  'capture.update': [
    'Change payload capture',
    'Change which request and response payloads are recorded. Does not grant access to read them.',
  ],
  'redaction.global.update': [
    'Change global audit masking',
    'Change masking rules applied across the deployment.',
  ],
  'export_policy.update': [
    'Change audit export policy',
    'Change which audit data may be exported.',
  ],
  'diagnostics.update': [
    'Change audit diagnostics',
    'Change diagnostic capture settings, including Omni diagnostics.',
  ],
  'diagnostics.denied.read': [
    'View denied discovery candidates',
    'View otherwise denied candidate samples in authorized Omni diagnostics. Other payload and event restrictions still apply.',
  ],
  'legacy.events.read': [
    'View legacy audit events',
    'View historical events through legacy audit authorization.',
  ],
  'legacy.payloads.read': [
    'Read legacy payloads',
    'Read legacy stored payloads under legacy authorization checks. This is sensitive historical content access.',
  ],
  'legacy.payloads.unclassified.read': [
    'Read unclassified legacy payloads',
    'Read historical payloads without a classified direction. Treat this as sensitive content access.',
  ],
})
group('audit.payloads', {
  'request.read': [
    'Read captured requests',
    'Read stored request messages for authorized events. Does not grant response access; the assignment must explicitly allow the capture scope.',
  ],
  'response.read': [
    'Read captured responses',
    'Read stored response messages for authorized events. Does not grant request access; the assignment must explicitly allow the capture scope.',
  ],
})
export const permissionHelp = help
export function describePermission(id: string): PermissionHelp {
  return (
    permissionHelp[id] ?? {
      title: id,
      description:
        'No description is available for this permission. Confirm its behavior in the server documentation before granting it.',
    }
  )
}
export const permissionGroupNames: Record<string, string> = {
  admission: 'Concurrency limits',
  platform: 'Platform',
  invocation_security: 'Global invocation security',
  teams: 'Teams',
  projects: 'Projects',
  ownership: 'Ownership',
  upstreams: 'Upstreams',
  virtual_servers: 'Virtual servers',
  agent_keys: 'Agent keys',
  identity_bindings: 'Identity bindings',
  control_tokens: 'Control tokens',
  tool_catalog: 'Tool catalog',
  tool_grants: 'Tool access',
  connections: 'Connection tests',
  rest: 'REST import',
  mock: 'Mock provider',
  runtime_jwts: 'Runtime tokens',
  sessions: 'Sessions',
  omni: 'Omni discovery',
  config: 'GitOps configuration',
  access: 'Access administration',
  audit: 'Audit',
}
