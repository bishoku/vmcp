export const auditSettings = {
  mode: 'strict',
  retentionDays: 7,
  maxPayloadBytes: 65536,
  revision: 1,
  scopes: {},
  rules: [],
  virtualServers: {},
  upstreams: {},
  exportPayloads: {},
}
export const auditMetadataCapabilities = {
  canReadMetadata: true,
  canReadIdentities: true,
  canReadDetails: true,
  canReadPayload: false,
  canReadRequestPayload: false,
  canReadResponsePayload: false,
  authorizationRevision: 1,
}
