import {act,cleanup,fireEvent,render,screen,within} from '@testing-library/react'
import {afterEach,beforeEach,expect,test,vi} from 'vitest'
import {MemoryRouter} from 'react-router-dom'
import AuditDetails from './AuditDetails'
import {api,type AuditEvent,type AuditCapabilities} from '../api'
import {RoleContext} from '../interaction'

beforeEach(()=>{
 Object.defineProperty(HTMLDialogElement.prototype,'showModal',{configurable:true,value:function(this:HTMLDialogElement){this.open=true}})
 Object.defineProperty(HTMLDialogElement.prototype,'close',{configurable:true,value:function(this:HTMLDialogElement){this.open=false}})
})
afterEach(()=>{cleanup();vi.restoreAllMocks();Reflect.deleteProperty(HTMLDialogElement.prototype,'showModal');Reflect.deleteProperty(HTMLDialogElement.prototype,'close')})
const caps=(request=false,response=false):AuditCapabilities=>({canReadMetadata:true,canReadIdentities:false,canReadDetails:false,canReadPayload:request||response,canReadRequestPayload:request,canReadResponsePayload:response,authorizationRevision:1})
const records=(request=false,response=false):AuditEvent[]=>[
 {id:1,eventId:'request-id',operationId:'operation',direction:'request',stage:'admission',time:'2026-11-04T10:00:00Z',status:'started',payloadState:'captured',capabilities:caps(request,false),actorId:'private-identity',clientIp:'private-ip',reason:'private-reason',target:'private-target'},
 {id:2,eventId:'response-id',operationId:'operation',direction:'response',stage:'outcome',time:'2026-11-04T10:00:01Z',status:'ok',payloadState:'captured',capabilities:caps(false,response),actorId:'private-identity',issuer:'private-issuer'},
] as AuditEvent[]
function view(events:AuditEvent[],role:'viewer'|'admin'='admin'){
 return <MemoryRouter><RoleContext.Provider value={role}><AuditDetails events={events} zone="UTC" close={()=>{}}/></RoleContext.Provider></MemoryRouter>
}
test('metadata-only including admins never downloads or hides payload bytes in the DOM',()=>{
 const payload=vi.spyOn(api,'auditPayload').mockResolvedValue({state:'captured',content:{secret:'must-not-download'}})
 render(view(records()))
 expect(payload).not.toHaveBeenCalled()
 expect(screen.queryByRole('button',{name:'Load audit payload'})).toBeNull()
 for(const secret of ['private-identity','private-ip','private-issuer','private-reason','private-target','must-not-download'])expect(document.body.textContent).not.toContain(secret)
 fireEvent.click(screen.getByText(/Technical context/))
 expect(payload).not.toHaveBeenCalled()
 expect(document.body.textContent).not.toContain('private-identity')
})
test.each(['request','response'] as const)('%s-only user fetches only the permitted event on explicit click',async direction=>{
 const payload=vi.spyOn(api,'auditPayload').mockResolvedValue({state:'captured',content:{message:'permitted-message'}})
 render(view(records(direction==='request',direction==='response'),'viewer'))
 expect(payload).not.toHaveBeenCalled()
 const allowed=screen.getByRole('region',{name:`${direction==='request'?'Request':'Response'} payload`})
 const denied=screen.getByRole('region',{name:`${direction==='request'?'Response':'Request'} payload`})
 expect(within(denied).queryByRole('button',{name:'Load audit payload'})).toBeNull()
 fireEvent.click(within(allowed).getByRole('button',{name:'Load audit payload'}))
 await screen.findByText(/permitted-message/)
 expect(payload).toHaveBeenCalledTimes(1)
 expect(payload).toHaveBeenCalledWith(`${direction}-id`,expect.any(AbortSignal))
})
test('missing capabilities fail closed even for an admin role',()=>{
 const payload=vi.spyOn(api,'auditPayload')
 const events=records(true,true).map(e=>({...e,capabilities:undefined}))
 render(view(events))
 expect(screen.queryByRole('button',{name:'Load audit payload'})).toBeNull()
 expect(payload).not.toHaveBeenCalled()
})
test('revocation aborts pending downloads and late responses cannot repopulate the panel',async()=>{
 let resolve!:(v:{state:string;content:unknown})=>void
 const payload=vi.spyOn(api,'auditPayload').mockImplementation(()=>new Promise(r=>{resolve=r}))
 const ui=render(view(records(true,false)))
 fireEvent.click(screen.getByRole('button',{name:'Load audit payload'}))
 const signal=payload.mock.calls[0][1]!
 ui.rerender(view(records(false,false)))
 expect(signal.aborted).toBe(true)
 await act(async()=>resolve({state:'captured',content:{secret:'late-private-data'}}))
 expect(document.body.textContent).not.toContain('late-private-data')
 expect(screen.queryByRole('button',{name:'Load audit payload'})).toBeNull()
})
test('closing details cancels payload fetch; reopening never restores an old message',async()=>{
 let resolve!:(v:{state:string;content:unknown})=>void
 const payload=vi.spyOn(api,'auditPayload').mockImplementation(()=>new Promise(r=>{resolve=r}))
 const ui=render(view(records(true,false)))
 fireEvent.click(screen.getByRole('button',{name:'Load audit payload'}))
 const signal=payload.mock.calls[0][1]!
 ui.unmount();expect(signal.aborted).toBe(true)
 await act(async()=>resolve({state:'captured',content:{secret:'closed-private-data'}}))
 render(view(records(true,false)))
 expect(payload).toHaveBeenCalledTimes(1)
 expect(document.body.textContent).not.toContain('closed-private-data')
})
test.each(['vmcp:session-expired','vmcp:claims-expired'])('%s clears an already loaded message',async(event)=>{
 vi.spyOn(api,'auditPayload').mockResolvedValue({state:'captured',content:{secret:'loaded-private-message'}})
 render(view(records(true,false)))
 fireEvent.click(screen.getByRole('button',{name:'Load audit payload'}))
 await screen.findByText(/loaded-private-message/)
 act(()=>window.dispatchEvent(new Event(event)))
 expect(document.body.textContent).not.toContain('loaded-private-message')
 fireEvent.click(screen.getByRole('button',{name:'Load audit payload'}))
 expect(api.auditPayload).toHaveBeenCalledTimes(1)
})

test('authorization revision change drops loaded content without another download',async()=>{
 const payload=vi.spyOn(api,'auditPayload').mockResolvedValue({state:'captured',content:{secret:'old-revision-message'}})
 const ui=render(view(records(true,false)))
 fireEvent.click(screen.getByRole('button',{name:'Load audit payload'}))
 await screen.findByText(/old-revision-message/)
 ui.rerender(view(records(true,false).map(e=>({...e,capabilities:{...e.capabilities!,authorizationRevision:2}}))))
 expect(document.body.textContent).not.toContain('old-revision-message')
 expect(payload).toHaveBeenCalledTimes(1)
})

test('raw metadata cannot render unexpected inline payload fields',()=>{
 const events=records().map(e=>({...e,payload:{secret:'unexpected-inline-secret'}}))
 render(view(events))
 fireEvent.click(screen.getByText(/Technical context/))
 expect(document.body.textContent).not.toContain('unexpected-inline-secret')
})

test('explicit metadata denial removes the detail view and cannot load a message',()=>{
 const payload=vi.spyOn(api,'auditPayload')
 render(view(records(true,true).map(e=>({...e,capabilities:{...e.capabilities!,canReadMetadata:false}}))))
 expect(screen.getByText('Audit metadata access is unavailable.')).toBeTruthy()
 expect(screen.queryByRole('button',{name:'Load audit payload'})).toBeNull()
 expect(payload).not.toHaveBeenCalled()
})

test('claim expiry aborts a pending payload and discards a late result',async()=>{
 let finish!:(value:{state:string;content:unknown})=>void
 const payload=vi.spyOn(api,'auditPayload').mockImplementation(()=>new Promise(resolve=>{finish=resolve}))
 render(view(records(true,false)))
 fireEvent.click(screen.getByRole('button',{name:'Load audit payload'}))
 act(()=>window.dispatchEvent(new Event('vmcp:claims-expired')))
 expect(payload.mock.calls[0][1]?.aborted).toBe(true)
 await act(async()=>finish({state:'captured',content:{secret:'late-claim-private'}}))
 expect(document.body.textContent).not.toContain('late-claim-private')
})
