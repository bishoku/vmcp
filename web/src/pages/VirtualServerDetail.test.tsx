import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { api } from '../api'
import { server, upstream, key, binding } from '../connections/fixtures'
import VirtualServerDetail from './VirtualServerDetail'
import ConnectionSetup from './ConnectionSetup'
beforeEach(()=>{
 vi.spyOn(api,'listVirtualServers').mockResolvedValue([server]);vi.spyOn(api,'listUpstreams').mockResolvedValue([upstream]);vi.spyOn(api,'listAgentKeys').mockResolvedValue([key]);vi.spyOn(api,'listBindings').mockResolvedValue([binding]);vi.spyOn(api,'listAudit').mockResolvedValue({items:[]})
})
afterEach(()=>{cleanup();vi.restoreAllMocks();vi.unstubAllEnvs()})
const open=(path:string)=>render(<MemoryRouter initialEntries={[path]}><Routes><Route path="/virtual-servers/:slug/:view?" element={<VirtualServerDetail/>}/><Route path="/setup" element={<ConnectionSetup/>}/></Routes></MemoryRouter>)
test('deep links open saved capability views and display all four kinds',async()=>{
 open('/virtual-servers/demo/capabilities');await screen.findByText('Saved configuration · version 7 · enabled. Open the editor to make a draft.')
 expect(screen.getByRole('link',{name:'Capabilities'}).getAttribute('aria-current')).toBe('page')
 expect(screen.getByText('resource')).toBeTruthy();expect(screen.getByText('template')).toBeTruthy();expect(screen.getByText('prompt')).toBeTruthy();expect(screen.getByText('tool')).toBeTruthy()
 expect(screen.queryByRole('link',{name:'Edit configuration'})).toBeNull()
 expect(api.listAudit).not.toHaveBeenCalled()
})
test('connect uses the configured gateway and credential placeholders; verification is deliberate',async()=>{
 vi.stubEnv('VITE_PUBLIC_GATEWAY_URL','https://gateway.example')
 open('/virtual-servers/demo/connect');await screen.findByText('https://gateway.example/mcp/v1/virtual/demo')
 expect(screen.getByText(/<VMCP_AGENT_KEY_OR_ACCESS_TOKEN>/)).toBeTruthy()
 expect((screen.getByRole('button',{name:'Verify connection'}) as HTMLButtonElement).disabled).toBe(true)
 fireEvent.change(screen.getByLabelText('Agent key or OAuth access token'),{target:{value:'key'}})
 expect((screen.getByRole('button',{name:'Verify connection'}) as HTMLButtonElement).disabled).toBe(false)
 expect(api.listAudit).not.toHaveBeenCalled()
})
test('public connect settings omit credentials and missing gateway configuration blocks verification',async()=>{
 vi.mocked(api.listVirtualServers).mockResolvedValue([{...server,authRequired:false}]);vi.stubEnv('VITE_PUBLIC_GATEWAY_URL','')
 open('/virtual-servers/demo/connect');await screen.findByText('Anonymous access is enabled. Credentials are bypassed on this server.')
 expect(screen.queryByLabelText('Agent key or OAuth access token')).toBeNull()
 expect(screen.getByRole('alert').textContent).toContain('not configured')
 expect((screen.getByRole('button',{name:'Verify connection'}) as HTMLButtonElement).disabled).toBe(true)
})
test('activity is bounded by server and a recent time window and does not infer active sessions',async()=>{
 open('/virtual-servers/demo/activity');await waitFor(()=>expect(api.listAudit).toHaveBeenCalledTimes(1))
 const [query]=vi.mocked(api.listAudit).mock.calls[0]
 expect(query.virtualServer).toBe('demo');expect(Date.now()-Date.parse(query.from!)).toBeLessThan(901_000)
 expect(await screen.findByText(/No stored events in this window/)).toBeTruthy()
 expect(screen.getByRole('link',{name:'Open audit log'}).getAttribute('href')).toBe('/audit?virtualServer=demo')
})
test('guide includes all steps and separates configured grants from explicit tests',async()=>{
 open('/setup?server=demo');await screen.findByText('Configured grants exist; runtime authorization still applies')
 expect(screen.getAllByRole('listitem')).toHaveLength(7)
 expect(screen.getByText('Explicit client verification required')).toBeTruthy()
 expect(screen.getByText('Explicit test required; not inferred from configuration')).toBeTruthy()
 expect(screen.getByText(/Read-only access: inspect configuration/)).toBeTruthy()
})


test('overview presents composition and protection with capability navigation, without runtime requests',async()=>{
 open('/virtual-servers/demo/overview');await screen.findByRole('region',{name:'Server overview'})
 const overview=screen.getByRole('region',{name:'Server overview'})
 expect(within(overview).getByRole('link',{name:/Selected tools/}).getAttribute('href')).toBe('/virtual-servers/demo/capabilities')
 expect(within(overview).getByRole('region',{name:'Upstream composition'}).textContent).toContain('Alpha')
 expect(within(overview).getByRole('region',{name:'Access and protection'}).textContent).toContain('Agent key / OAuth token')
 expect(within(overview).getByRole('link',{name:'Connection settings'}).getAttribute('href')).toBe('/virtual-servers/demo/connect')
 expect(within(overview).getByRole('region',{name:'Gateway endpoint'}).textContent).toContain('/mcp/v1/virtual/demo')
 expect(api.listAudit).not.toHaveBeenCalled()
})


test.each([
 {enabled:false},
 {enabled:true},
 {enabled:true,detectors:[]},
 {enabled:true,customRules:[]},
 {enabled:true,detectors:[],customRules:[]},
])('protection handles omitted PII lists: %j',async(pii)=>{
 vi.mocked(api.listVirtualServers).mockResolvedValue([{...server,pii}])
 open('/virtual-servers/demo/protection')
 await screen.findByText('all built-in (default)')
 expect(within(screen.getByRole('region',{name:'Data masking'})).getByText('none')).toBeTruthy()
 expect(api.listAudit).not.toHaveBeenCalled()
})
test('protection displays configured detectors and custom rule names',async()=>{
 vi.mocked(api.listVirtualServers).mockResolvedValue([{...server,pii:{enabled:true,detectors:['email','phone'],customRules:[{name:'employee',pattern:'EMP-[0-9]+'}]}}])
 open('/virtual-servers/demo/protection')
 await screen.findByText('email, phone')
 expect(screen.getByText('employee')).toBeTruthy()
})

test('activity keeps admission and outcome records distinct and links to the stored operation',async()=>{
 const admission={eventId:'admit-1',operationId:'operation-1',requestId:'request-1',traceId:'trace-1',schemaVersion:1,actorType:'agent_key',actorId:'team-key',issuer:'',action:'tools/call',target:'lookup',configRevision:7,stage:'admission' as const,id:1,time:'2026-10-15T10:00:00Z',virtualServer:'demo',method:'tools/call',tool:'lookup',upstream:'alpha',status:'started' as const,durationMs:0}
 vi.mocked(api.listAudit).mockResolvedValue({items:[admission,{...admission,eventId:'outcome-1',id:2,stage:'outcome',status:'denied',reason:'Tool policy denied access',durationMs:12}],nextCursor:'more-events'})
 open('/virtual-servers/demo/activity')
 const events=await screen.findByRole('table',{name:'Recent events for demo'})
 expect(within(events).getAllByRole('row')).toHaveLength(3)
 expect(within(events).getByText('admission')).toBeTruthy()
 expect(within(events).getByText('denied')).toBeTruthy()
 expect(within(events).getByText('Tool policy denied access')).toBeTruthy()
 expect(within(events).getAllByRole('link',{name:'Inspect operation'})[0].getAttribute('href')).toBe('/audit?operationId=operation-1')
 expect(screen.getByRole('link',{name:'Continue in audit log'}).getAttribute('href')).toBe('/audit?virtualServer=demo')
})

test('failed activity refresh retains stored events and offers manual retry',async()=>{
 const event={eventId:'event-1',traceId:'trace-1',schemaVersion:1,actorType:'agent_key',actorId:'team-key',issuer:'',action:'tools/call',target:'lookup',configRevision:7,stage:'outcome' as const,id:1,time:'2026-10-15T10:00:00Z',virtualServer:'demo',method:'tools/call',status:'ok' as const,durationMs:5}
 vi.mocked(api.listAudit).mockResolvedValueOnce({items:[event]}).mockRejectedValueOnce(new Error('Audit storage offline')).mockResolvedValueOnce({items:[]})
 open('/virtual-servers/demo/activity')
 await screen.findByRole('table',{name:'Recent events for demo'})
 fireEvent.click(screen.getByRole('button',{name:'Check recent activity'}))
 await waitFor(()=>expect(screen.getByRole('alert').textContent).toContain('Showing events from the last successful check'))
 expect(screen.getByRole('table',{name:'Recent events for demo'})).toBeTruthy()
 fireEvent.click(screen.getByRole('button',{name:'Check recent activity'}))
 await screen.findByText('No stored events in this window')
 expect(screen.queryByRole('table')).toBeNull()
 expect(screen.queryByRole('alert')).toBeNull()
})

test('capability search combines kind and alias filters without changing summary counts',async()=>{
 open('/virtual-servers/demo/capabilities')
 await screen.findByRole('region',{name:'Selected capabilities'})
 fireEvent.change(screen.getByLabelText('Capability kind'),{target:{value:'resource'}})
 fireEvent.change(screen.getByLabelText('Search capabilities'),{target:{value:'doc://team'}})
 const panel=screen.getByRole('region',{name:'Selected capabilities'})
 expect(within(panel).getByText('doc://team')).toBeTruthy()
 expect(within(panel).queryByText('lookup')).toBeNull()
 fireEvent.change(screen.getByLabelText('Search capabilities'),{target:{value:'no such capability'}})
 expect(within(panel).getByText('No matching capabilities')).toBeTruthy()
 fireEvent.change(screen.getByLabelText('Search capabilities'),{target:{value:''}})
 fireEvent.change(screen.getByLabelText('Capability kind'),{target:{value:''}})
 expect(within(panel).getByText('Missing object')).toBeTruthy()
 expect(api.listAudit).not.toHaveBeenCalled()
})

test('access retains revoked and conditional grants with inspectable restrictions',async()=>{
 vi.mocked(api.listAgentKeys).mockResolvedValue([{...key,status:'revoked',ipAllowlist:['192.0.2.1'],rateLimitPerMinute:10}])
 open('/virtual-servers/demo/access')
 const panel=await screen.findByRole('region',{name:'Configured access relationships'})
 expect(within(panel).getByText('revoked')).toBeTruthy()
 expect(within(panel).getByText('conditional')).toBeTruthy()
 expect(within(panel).getByText(/IP allowlist: 192.0.2.1; key limit: 10/)).toBeTruthy()
 expect(within(panel).getByRole('link',{name:'team-key'}).getAttribute('href')).toBe('/agent-keys?inspect=team-key')
 fireEvent.change(screen.getByLabelText('Search access relationships'),{target:{value:'JWT'}})
 expect(within(panel).getByRole('link',{name:'team-jwt'})).toBeTruthy()
 expect(within(panel).queryByRole('link',{name:'team-key'})).toBeNull()
 expect(api.listAudit).not.toHaveBeenCalled()
})

test('anonymous access distinguishes saved credentials from public endpoint behavior',async()=>{
 vi.mocked(api.listVirtualServers).mockResolvedValue([{...server,authRequired:false}])
 open('/virtual-servers/demo/access')
 const behavior=await screen.findByRole('region',{name:'Authentication behavior'})
 expect(within(behavior).getByText(/Anonymous access bypasses credential authentication/)).toBeTruthy()
 expect(within(behavior).getByText(/credential restrictions do not protect an anonymous endpoint/)).toBeTruthy()
 expect(screen.getByRole('region',{name:'Configured access relationships'}).textContent).toContain('Anonymous')
})

test('setup provides connection settings and clears credentials when the selected server changes',async()=>{
 vi.stubEnv('VITE_PUBLIC_GATEWAY_URL','https://gateway.example')
 vi.mocked(api.listVirtualServers).mockResolvedValue([server,{...server,slug:'other',name:'Other',endpoint:'/mcp/v1/virtual/other',authRequired:false}])
 open('/setup?server=demo')
 await screen.findByRole('region',{name:'Client configuration'})
 expect(screen.getByText('https://gateway.example/mcp/v1/virtual/demo')).toBeTruthy()
 fireEvent.change(screen.getByLabelText('Agent key or OAuth access token'),{target:{value:'unsent-credential'}})
 fireEvent.change(screen.getByLabelText('Guide for virtual server'),{target:{value:'other'}})
 expect(await screen.findByText('https://gateway.example/mcp/v1/virtual/other')).toBeTruthy()
 expect(screen.queryByLabelText('Agent key or OAuth access token')).toBeNull()
 fireEvent.change(screen.getByLabelText('Guide for virtual server'),{target:{value:'demo'}})
 expect((screen.getByLabelText('Agent key or OAuth access token') as HTMLInputElement).value).toBe('')
 expect(api.listAudit).not.toHaveBeenCalled()
})

test('setup does not treat partial upstream inventory as complete discovery',async()=>{
 vi.mocked(api.listVirtualServers).mockResolvedValue([{...server,tools:[...server.tools,{...server.tools[0],upstream:'missing'}]}])
 open('/setup?server=demo')
 await screen.findByText('Discovery needed or not yet known')
 expect(screen.queryByText('Referenced upstreams have discovery snapshots')).toBeNull()
 expect(await within(screen.getByRole('region',{name:'Saved connection path'})).findByText('Not loaded')).toBeTruthy()
 expect(api.listAudit).not.toHaveBeenCalled()
})
