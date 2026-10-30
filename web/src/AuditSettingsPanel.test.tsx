import { cleanup,fireEvent,screen,waitFor,within } from '@testing-library/react'
import { afterEach,beforeEach,expect,test,vi } from 'vitest'
import { render } from './test-utils'
import { api,type AuditSettings } from './api'
import AuditSettingsPanel from './AuditSettingsPanel'
const settings:AuditSettings={mode:'strict',retentionDays:7,maxPayloadBytes:65536,revision:3,scopes:{virtual:{request:true,response:true}},rules:[{name:'email',pattern:'example'}],virtualServers:{support:[{name:'customer',pattern:'CUST-[0-9]+'}]},upstreams:{crm:[{name:'employee',pattern:'EMP-[0-9]+'}]},exportPayloads:{}}
beforeEach(()=>{vi.spyOn(api,'auditSettings').mockResolvedValue(structuredClone(settings));vi.spyOn(api,'listVirtualServers').mockResolvedValue([]);vi.spyOn(api,'listUpstreams').mockResolvedValue([])})
afterEach(()=>{cleanup();vi.restoreAllMocks()})
test('local helper submits inherited rules, unsaved edits and both directions without saving samples',async()=>{
 const preview=vi.spyOn(api,'previewAudit').mockResolvedValue({state:'redacted',content:{text:'[REDACTED]'},originalBytes:30,storedBytes:20,redactions:1,revision:3,matches:[{source:'global',name:'email',count:1}]})
 const save=vi.spyOn(api,'saveAuditSettings')
 render(<AuditSettingsPanel scope="virtual" slug="support"/>);await screen.findByDisplayValue('CUST-[0-9]+')
 fireEvent.change(screen.getByLabelText('Audit rule 1 regex'),{target:{value:'CUST-[A-Z]+'}})
 fireEvent.click(screen.getByText('Test request / response audit payload'))
 fireEvent.click(screen.getByRole('button',{name:'Preview audit payloads'}))
 await waitFor(()=>expect(preview).toHaveBeenCalledTimes(2))
 for(const [body]of preview.mock.calls){expect(body.settings.rules).toEqual(settings.rules);expect(body.settings.virtualServers.support[0].pattern).toBe('CUST-[A-Z]+');expect(body.virtualServer).toBe('support')}
 expect(preview.mock.calls.map(([b])=>b.direction)).toEqual(['request','response']);expect(save).not.toHaveBeenCalled()
 expect(await screen.findAllByText('global / email: 1')).toHaveLength(2)
})
test('save conflicts keep the unsaved global policy',async()=>{
 vi.spyOn(api,'saveAuditSettings').mockRejectedValue(new Error('Reload audit settings before saving'))
 render(<AuditSettingsPanel/>);await screen.findByLabelText('Audit mode')
 fireEvent.change(screen.getByLabelText('Audit mode'),{target:{value:'relax'}})
 fireEvent.click(screen.getByRole('button',{name:'Save audit settings'}))
 await screen.findByText('Reload audit settings before saving');expect((screen.getByLabelText('Audit mode') as HTMLSelectElement).value).toBe('relax')
})
test('sample JSON errors do not call the preview API',async()=>{
 const preview=vi.spyOn(api,'previewAudit')
 render(<AuditSettingsPanel scope="upstream" slug="crm"/>);await screen.findByDisplayValue('EMP-[0-9]+')
 fireEvent.click(screen.getByText('Test request / response audit payload'))
 fireEvent.change(screen.getByLabelText('Example audit response'),{target:{value:'invalid JSON'}})
 fireEvent.click(screen.getByRole('button',{name:'Preview audit payloads'}));await waitFor(()=>expect(within(screen.getByRole('alert')).getByText(/JSON/)).toBeTruthy());expect(preview).not.toHaveBeenCalled()
})

test('Omni capture and bounded discovery details save independently of model payload capture',async()=>{
 const save=vi.spyOn(api,'saveAuditSettings').mockImplementation(async s=>s)
 render(<AuditSettingsPanel/>);await screen.findByLabelText('Audit mode')
 fireEvent.click(screen.getByRole('checkbox',{name:'Omni MCP calls request capture'}))
 fireEvent.click(screen.getByRole('checkbox',{name:'Omni discovery pipeline response capture'}))
 fireEvent.click(screen.getByLabelText('Include candidate identities, rankings and scores'))
 fireEvent.click(screen.getByLabelText('Include unauthorized candidate samples (administrator payload access only)'))
 fireEvent.change(screen.getByLabelText('Maximum Omni candidate samples'),{target:{value:'50'}})
 fireEvent.click(screen.getByRole('button',{name:'Save audit settings'}))
 await waitFor(()=>expect(save).toHaveBeenCalledOnce())
 const draft=save.mock.calls[0][0]
 expect(draft.scopes.omni.request).toBe(true)
 expect(draft.scopes.omni_diagnostics.response).toBe(true)
 expect(draft.scopes.omni_embedding?.response??false).toBe(false)
 expect(draft.omni).toEqual({candidates:true,includeDenied:true,maxCandidates:50})
})
