import type { AgentKey, IdentityBinding, Upstream, VirtualServer } from '../api'
export const server: VirtualServer = {
  slug:'demo',name:'Demo',description:'Team gateway',version:7,enabled:true,authRequired:true,endpoint:'/mcp/v1/virtual/demo',
  policy:{default:'allow',tools:{lookup:'deny'}},tools:[{upstream:'alpha',tool:'lookup',alias:'lookup',exposedName:'lookup',upstreamDescription:'',descriptionDrift:false,orphaned:false}],
  catalog:[{upstream:'alpha',kind:'resource',name:'doc://guide',alias:'doc://team',exposedName:'doc://team',orphaned:false,descriptionDrift:false,upstreamDescription:''},
    {upstream:'alpha',kind:'template',name:'doc://{name}',exposedName:'doc://{name}',orphaned:false,descriptionDrift:false,upstreamDescription:''},
    {upstream:'alpha',kind:'prompt',name:'review',exposedName:'review',orphaned:true,descriptionDrift:false,upstreamDescription:''}],
}
export const upstream: Upstream = {slug:'alpha',name:'Alpha',version:2,description:'',url:'https://example.com/mcp',fallbackUrls:[],enabled:true,transport:'streamable_http',headerNames:[],
  tls:{caPem:'',serverName:'',clientCertPem:'',hasClientKey:false,caCerts:[],clientCert:null},toolCount:1,lastRefreshAt:'2026-10-11T12:00:00Z',lastRefreshError:null,
  health:{state:'healthy',nodes:[{node:'node-a',url:'https://example.com/mcp',state:'healthy',breaker:'closed',latencyMs:12,error:null,checkedAt:'2026-10-11T12:00:00Z'}]},
}
export const key: AgentKey = {slug:'team-key',version:1,description:'',keyPrefix:'vmcp_',virtualServers:['demo'],ipAllowlist:[],rateLimitPerMinute:null,expiresAt:null,revokedAt:null,status:'active',createdAt:'2026-10-11T12:00:00Z'}
export const binding: IdentityBinding = {slug:'team-jwt',version:1,description:'',conditions:[{claim:'azp',op:'equals',value:'team'}],virtualServers:['demo'],ipAllowlist:[],rateLimitPerMinute:null,enabled:true}
