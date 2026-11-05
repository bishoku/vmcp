import {act,cleanup,fireEvent,render,screen,waitFor,within} from '@testing-library/react'
import {MemoryRouter} from 'react-router-dom'
import {afterEach,beforeEach,expect,test,vi} from 'vitest'
import {ApiError,api,type Me} from '../api'
import {RoleContext} from '../interaction'
import Authorization from './Authorization'
import {accessFixture as fixture,ids} from '../authorization/fixtures'
import type {Evaluation} from '../authorization/client'
function view(role:Me['role']='admin',section='roles'){return <MemoryRouter initialEntries={[`/access?section=${section}`]}><RoleContext.Provider value={role}><Authorization/></RoleContext.Provider></MemoryRouter>}
function mockData(){
 vi.spyOn(api,'accessPermissions').mockResolvedValue({items:fixture.permissions})
 vi.spyOn(api,'accessRoles').mockResolvedValue({items:fixture.roles,authorizationRevision:7})
 vi.spyOn(api,'accessAssignments').mockResolvedValue({items:fixture.assignments,authorizationRevision:7})
 vi.spyOn(api,'accessMappings').mockResolvedValue({items:fixture.mappings,authorizationRevision:7})
 vi.spyOn(api,'accessAccessStatus').mockResolvedValue(fixture.status)
 vi.spyOn(api,'accessScopes').mockImplementation(async kind=>({items:kind==='control_token'?[{id:ids.token,slug:'reader-token',name:'Reader token'}]:[{id:ids.resource,slug:'crm',name:'CRM'}]}))
 vi.spyOn(api,'accessEpochs').mockResolvedValue({items:[{id:ids.oldEpoch,kind:'upstream',resourceId:ids.resource,ownerTeam:ids.owner,startRevision:1,endRevision:3},{id:ids.newEpoch,kind:'upstream',resourceId:ids.resource,ownerTeam:ids.newOwner,startRevision:4,endRevision:null}]})
 vi.spyOn(window,'confirm').mockReturnValue(true)
}
beforeEach(()=>{
 HTMLDialogElement.prototype.showModal=function(){this.open=true}
 HTMLDialogElement.prototype.close=function(){this.open=false}
 mockData()
})
afterEach(()=>{cleanup();vi.restoreAllMocks();vi.unstubAllGlobals()})
const loaded=()=>screen.findByText('Platform Operator')
const fill=(name:string,value:string)=>fireEvent.change(screen.getByLabelText(name),{target:{value}})

test.each(['viewer','editor','team','custom'] as const)('%s cannot load management data through the page',role=>{
 render(view(role));expect(screen.getByRole('alert').textContent).toContain('administrator')
 for(const name of ['accessPermissions','accessRoles','accessAssignments','accessMappings','accessAccessStatus'] as const)expect(api[name]).not.toHaveBeenCalled()
})
test('builtin roles are readonly and copied permissions stay explicit',async()=>{
 const create=vi.spyOn(api,'createAccessRole').mockResolvedValue({...fixture.roles[1],builtin:false})
 render(view());await loaded()
 const observer=screen.getByText('Observer').closest('article')!
 fireEvent.click(within(observer).getByRole('button',{name:'View'}))
 const dialog=screen.getByRole('dialog',{name:'Builtin role'})
 expect(within(dialog).queryByRole('button',{name:'Save role'})).toBeNull();expect(within(dialog).getByLabelText('Role name').matches(':disabled')).toBe(true)
 fireEvent.click(within(dialog).getByRole('button',{name:'Close'}))
 fireEvent.click(within(observer).getByRole('button',{name:'Copy'}))
 fill('Role identifier','copied-observer');fill('Role name','Copied observer')
 fireEvent.click(screen.getByLabelText('audit.payloads.request.read'))
 fireEvent.click(screen.getByRole('button',{name:'Save role'}))
 await waitFor(()=>expect(create).toHaveBeenCalledTimes(1))
 const body=create.mock.calls[0][0];expect(body.permissions).toEqual(['upstreams.read','audit.payloads.request.read']);expect(body).not.toHaveProperty('builtin');expect(body).not.toHaveProperty('id');expect(body).not.toHaveProperty('version')
})
test('stale role edits are preserved, never retried and require an explicit reload',async()=>{
 const update=vi.spyOn(api,'updateAccessRole').mockRejectedValueOnce(new ApiError(412,'Changed')).mockResolvedValue(fixture.roles[2])
 const reload=vi.spyOn(api,'accessRole').mockResolvedValue({...fixture.roles[2],name:'Latest role',version:3})
 render(view());await loaded();fireEvent.click(within(screen.getByText('Request reader').closest('article')!).getByRole('button',{name:'Edit'}))
 fill('Role name','Draft name');fireEvent.click(screen.getByRole('button',{name:'Save role'}))
 await screen.findByRole('button',{name:'Reload role and discard draft'})
 expect(screen.getByLabelText('Role name')).toHaveProperty('value','Draft name');expect(screen.getByRole('button',{name:'Save role'})).toHaveProperty('disabled',true);expect(update).toHaveBeenCalledTimes(1)
 fireEvent.click(screen.getByRole('button',{name:'Reload role and discard draft'}));await waitFor(()=>expect(screen.getByLabelText('Role name')).toHaveProperty('value','Latest role'));expect(reload).toHaveBeenCalledTimes(1)
 fireEvent.click(screen.getByRole('button',{name:'Save role'}));await waitFor(()=>expect(update).toHaveBeenCalledTimes(2));expect(update.mock.calls[1][0].version).toBe(3)
})
test('request payload roles require an explicit capture allowlist on assignments',async()=>{
 const create=vi.spyOn(api,'createAccessAssignment').mockResolvedValue(fixture.assignments[0])
 render(view('admin','assignments'));await screen.findByRole('button',{name:'Create assignment'})
 fireEvent.click(screen.getByRole('button',{name:'Create assignment'}));fill('Assigned role',ids.custom);fill('OIDC subject','new-subject')
 fireEvent.click(screen.getByRole('button',{name:'Save assignment'}));await screen.findByText(/Payload permissions require/);expect(create).not.toHaveBeenCalled()
 fireEvent.click(screen.getByLabelText('control'));fireEvent.click(screen.getByRole('button',{name:'Save assignment'}))
 await waitFor(()=>expect(create).toHaveBeenCalledTimes(1));expect(create.mock.calls[0][0]).toEqual({roleId:ids.custom,principal:{kind:'oidc',issuer:fixture.status.trustedIssuer,id:'new-subject'},scope:{kind:'tenant'},captureScopes:['control'],enabled:false})
})
test('editing an assignment retains its historical resource and owner pins',async()=>{
 const update=vi.spyOn(api,'updateAccessAssignment').mockResolvedValue(fixture.assignments[0])
 render(view('admin','assignments'));await screen.findByText('Request reader');fireEvent.click(screen.getByRole('button',{name:'Edit'}))
 await screen.findByRole('option',{name:new RegExp(`Historical.*${ids.oldEpoch}`)})
 fireEvent.click(screen.getByLabelText('Assignment enabled'));fireEvent.click(screen.getByRole('button',{name:'Save assignment'}))
 await waitFor(()=>expect(update).toHaveBeenCalledTimes(1));expect(update.mock.calls[0][1].scope).toEqual(fixture.assignments[0].scope);expect(update.mock.calls[0][0].version).toBe(1)
})
test('untrusted issuers cannot be enabled from the mapping editor',async()=>{
 const create=vi.spyOn(api,'createAccessMapping').mockResolvedValue(fixture.mappings[0])
 render(view('admin','mappings'));await screen.findByRole('button',{name:'Create mapping'});fireEvent.click(screen.getByRole('button',{name:'Create mapping'}))
 fill('OIDC issuer','https://foreign.example');fill('Claim value 1','auditors');fireEvent.click(screen.getByLabelText('Mapping enabled'));fireEvent.click(screen.getByRole('button',{name:'Save mapping'}))
 await screen.findByText(/Enabled mappings must exactly match/);expect(create).not.toHaveBeenCalled()
 fireEvent.click(screen.getByLabelText('Mapping enabled'));fireEvent.click(screen.getByRole('button',{name:'Save mapping'}))
 await waitFor(()=>expect(create).toHaveBeenCalledTimes(1));expect(create.mock.calls[0][0]).toMatchObject({issuer:'https://foreign.example',enabled:false,principalKind:'oidc'})
})
const result:Evaluation={simulationOnly:true,authenticationValidated:false,authorizationRevision:7,claimMappings:'live-audit',matchedMappingIds:[ids.mapping],candidateAssignmentIds:[ids.assignment],decision:{allowed:true,reason:'audit_policy_allowed'}}
function prepareSimulation(){fill('OIDC subject','sample-subject');fill('Sample OIDC claims','{"scope":"audit:read"}');fill('Permission to evaluate','audit.payloads.request.read');fill('Evaluation target','event');fill('Audit event UUID',ids.event)}
test('simulation uses only sample claims and persisted event metadata, never a payload download',async()=>{
 const evaluate=vi.spyOn(api,'evaluateAccess').mockResolvedValue(result),payload=vi.spyOn(api,'auditPayload')
 const storage=vi.spyOn(Storage.prototype,'setItem')
 render(view('admin','simulate'));await screen.findByRole('button',{name:'Evaluate permission'});prepareSimulation();fireEvent.click(screen.getByRole('button',{name:'Evaluate permission'}))
 await screen.findByText('Allowed by stored policy');expect(evaluate.mock.calls[0][0]).toEqual({principal:{kind:'oidc',issuer:fixture.status.trustedIssuer,id:'sample-subject'},claims:{scope:'audit:read'},permission:'audit.payloads.request.read',auditEventId:ids.event})
 expect(payload).not.toHaveBeenCalled();expect(storage).not.toHaveBeenCalled()
 fill('Sample OIDC claims','{"scope":"different"}');await waitFor(()=>expect(screen.queryByText('Allowed by stored policy')).toBeNull())
})
test('permission loss clears sensitive forms and discards a late simulation result',async()=>{
 let finish!:(v:Evaluation)=>void
 const evaluate=vi.spyOn(api,'evaluateAccess').mockImplementation(()=>new Promise(resolve=>{finish=resolve}))
 render(view('admin','simulate'));await screen.findByRole('button',{name:'Evaluate permission'});prepareSimulation();fill('Sample OIDC claims','{"private":"never-retain"}');fireEvent.click(screen.getByRole('button',{name:'Evaluate permission'}))
 act(()=>window.dispatchEvent(new Event('vmcp:session-expired')))
 expect(screen.queryByLabelText('Sample OIDC claims')).toBeNull();expect(evaluate.mock.calls[0][1]?.aborted).toBe(true)
 await act(async()=>finish(result));expect(screen.queryByText('Allowed by stored policy')).toBeNull();expect(document.body.textContent).not.toContain('never-retain')
})
test('server 403 clears the workspace instead of retaining management metadata',async()=>{
 vi.mocked(api.accessRoles).mockRejectedValue(new ApiError(403,'No longer admin'))
 render(view());await screen.findByText(/Access is no longer available/);expect(screen.queryByText('Platform Operator')).toBeNull()
})
test('migration status reports blockers without offering mode activation',async()=>{
 render(view('admin','migration'));await screen.findByRole('heading',{name:'Access warnings'})
 expect(screen.getByText('Enabled mappings refer to an unconfigured issuer.')).toBeTruthy();expect(screen.getByText('legacy Audit Events')).toBeTruthy()
 expect(screen.queryByRole('button',{name:/activate|enforce|apply/i})).toBeNull()
})

test('a delayed delete confirmation cannot submit after administrator access is lost',async()=>{
 let accept!:(accepted:boolean)=>void
 const {ConfirmationContext}=await import('../confirmation')
 const remove=vi.spyOn(api,'deleteAccessAssignment').mockResolvedValue(undefined)
 render(<ConfirmationContext.Provider value={()=>new Promise(resolve=>{accept=resolve})}>{view('admin','assignments')}</ConfirmationContext.Provider>)
 await screen.findByText('Request reader');fireEvent.click(screen.getByRole('button',{name:'Delete'}))
 act(()=>window.dispatchEvent(new Event('vmcp:session-expired')))
 await act(async()=>accept(true));expect(remove).not.toHaveBeenCalled()
})
test('changing admin to viewer unmounts an editor and its sensitive data',async()=>{
 const {rerender}=render(view('admin','mappings'));await screen.findByRole('button',{name:'Create mapping'});fireEvent.click(screen.getByRole('button',{name:'Create mapping'}));fill('Claim value 1','sensitive-draft')
 rerender(view('viewer','mappings'));expect(screen.queryByRole('dialog')).toBeNull();expect(screen.queryByLabelText('Claim value 1')).toBeNull();expect(document.body.textContent).not.toContain('sensitive-draft')
})
test('inconsistent policy revisions block editing until a successful refresh',async()=>{
 vi.mocked(api.accessAssignments).mockResolvedValueOnce({items:fixture.assignments,authorizationRevision:8})
 render(view());await screen.findByText(/policy changed while loading/);expect(screen.queryByRole('button',{name:'Create role'})).toBeNull()
 fireEvent.click(screen.getByRole('button',{name:'Refresh access'}));await loaded();expect(screen.getByRole('button',{name:'Create role'})).not.toHaveProperty('disabled',true)
})
test('a new current ownership period is sent only after explicit selection',async()=>{
 const update=vi.spyOn(api,'updateAccessAssignment').mockResolvedValue(fixture.assignments[0])
 render(view('admin','assignments'));await screen.findByText('Request reader');fireEvent.click(screen.getByRole('button',{name:'Edit'}));await screen.findByRole('option',{name:new RegExp(`Current.*${ids.newEpoch}`)})
 fill('Ownership period',ids.newEpoch);fireEvent.click(screen.getByRole('button',{name:'Save assignment'}));await waitFor(()=>expect(update).toHaveBeenCalledTimes(1))
 expect(update.mock.calls[0][1].scope).toEqual({kind:'upstream',id:ids.resource,epoch:ids.newEpoch,ownerTeam:ids.newOwner})
})
test('control-token assignments use immutable identities and omit the OIDC issuer',async()=>{
 const create=vi.spyOn(api,'createAccessAssignment').mockResolvedValue(fixture.assignments[0])
 render(view('admin','assignments'));await screen.findByRole('button',{name:'Create assignment'});fireEvent.click(screen.getByRole('button',{name:'Create assignment'}));fill('Assigned role',ids.observer);fill('Principal type','control_token')
 await screen.findByRole('option',{name:new RegExp('reader-token')});fill('Control token',ids.token);fireEvent.click(screen.getByRole('button',{name:'Save assignment'}));await waitFor(()=>expect(create).toHaveBeenCalledTimes(1))
 expect(create.mock.calls[0][0].principal).toEqual({kind:'control_token',id:ids.token});expect(create.mock.calls[0][0]).not.toHaveProperty('mappingId')
})
test('mapping assignments omit the direct principal and preserve explicit enabled state',async()=>{
 const create=vi.spyOn(api,'createAccessAssignment').mockResolvedValue(fixture.assignments[0])
 render(view('admin','assignments'));await screen.findByRole('button',{name:'Create assignment'});fireEvent.click(screen.getByRole('button',{name:'Create assignment'}));fill('Assigned role',ids.observer);fill('Identity source','mapping');fill('Claim mapping',ids.mapping);fireEvent.click(screen.getByLabelText('Assignment enabled'));fireEvent.click(screen.getByRole('button',{name:'Save assignment'}));await waitFor(()=>expect(create).toHaveBeenCalledTimes(1))
 expect(create.mock.calls[0][0]).toMatchObject({mappingId:ids.mapping,enabled:true});expect(create.mock.calls[0][0]).not.toHaveProperty('principal')
})
test('typed AND conditions are submitted without new trust or token fields',async()=>{
 const create=vi.spyOn(api,'createAccessMapping').mockResolvedValue(fixture.mappings[0])
 render(view('admin','mappings'));await screen.findByRole('button',{name:'Create mapping'});fireEvent.click(screen.getByRole('button',{name:'Create mapping'}));fill('Claim value 1','auditors');fireEvent.click(screen.getByRole('button',{name:'Add condition'}));fill('Claim path 2','scope');fill('Operator 2','scope_contains');fill('Claim value 2','audit:read write');fireEvent.click(screen.getByRole('button',{name:'Save mapping'}));await screen.findByText(/Scope conditions require one exact scope token/);expect(create).not.toHaveBeenCalled()
 fill('Claim value 2','audit:read');fireEvent.click(screen.getByRole('button',{name:'Save mapping'}));await waitFor(()=>expect(create).toHaveBeenCalledTimes(1));expect(create.mock.calls[0][0]).toEqual({issuer:fixture.status.trustedIssuer,principalKind:'oidc',enabled:false,conditions:[{path:'groups',operator:'array_contains',value:'auditors'},{path:'scope',operator:'scope_contains',value:'audit:read'}]})
})
test('invalid sample JSON never reaches the simulation API',async()=>{
 const evaluate=vi.spyOn(api,'evaluateAccess')
 render(view('admin','simulate'));await screen.findByRole('button',{name:'Evaluate permission'});prepareSimulation();fill('Sample OIDC claims','["not a claims object"]');fireEvent.click(screen.getByRole('button',{name:'Evaluate permission'}));await screen.findByText('Sample claims must be a JSON object.');expect(evaluate).not.toHaveBeenCalled()
})

test('a mapping reference conflict remains an editable draft without automatic retry',async()=>{
 const update=vi.spyOn(api,'updateAccessMapping').mockRejectedValue(new ApiError(409,'Disable mapping assignments before changing their issuer'))
 render(view('admin','mappings'));await screen.findByRole('button',{name:'Edit'});fireEvent.click(screen.getByRole('button',{name:'Edit'}));fireEvent.click(screen.getByLabelText('Mapping enabled'));fireEvent.click(screen.getByRole('button',{name:'Save mapping'}))
 await screen.findByText('Disable mapping assignments before changing their issuer');expect(screen.getByRole('dialog')).toBeTruthy();expect(update).toHaveBeenCalledTimes(1);expect(screen.getByLabelText('Mapping enabled')).toHaveProperty('checked',false)
})
test('selector access loss clears all management data as well as the selector itself',async()=>{
 vi.mocked(api.accessScopes).mockRejectedValue(new ApiError(403,'No longer admin'))
 render(view('admin','assignments'));await screen.findByRole('button',{name:'Create assignment'});fireEvent.click(screen.getByRole('button',{name:'Create assignment'}));fill('Principal type','control_token')
 await screen.findByText(/Access is no longer available/);expect(screen.queryByRole('dialog')).toBeNull();expect(screen.queryByText('Request reader')).toBeNull()
})
test('an empty selected-scope permission set cannot be assigned',async()=>{
 vi.mocked(api.accessPermissions).mockResolvedValue({items:[...fixture.permissions.map(p=>p.id==='upstreams.read'?{...p,scopes:p.scopes.filter(kind=>kind==='tenant')}:p)]})
 const create=vi.spyOn(api,'createAccessAssignment')
 render(view('admin','assignments'));await screen.findByRole('button',{name:'Create assignment'});fireEvent.click(screen.getByRole('button',{name:'Create assignment'}));fill('Assigned role',ids.observer);fill('OIDC subject','subject');fill('Scope kind','upstream');await screen.findByRole('option',{name:/CRM/});fill('Resource',ids.resource);fireEvent.click(screen.getByRole('button',{name:'Save assignment'}));await screen.findByText('None of the selected role permissions support this scope.');expect(create).not.toHaveBeenCalled()
})


test('explains that permission roles do not establish administrator identity',async()=>{
 render(view());await loaded()
 expect(screen.getByText(/Platform administrator access comes from deployment-configured admin groups or admin credentials/)).toBeTruthy()
 expect(screen.queryByText('Platform admin')).toBeNull()
})
