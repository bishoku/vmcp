import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
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
 expect(screen.getByRole('link',{name:'Inspect configuration'}).getAttribute('href')).toBe('/virtual-servers?inspect=demo')
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
 expect(screen.getByRole('status').textContent).toContain('Read-only access')
})
