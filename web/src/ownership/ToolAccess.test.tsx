import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { api, type ToolGrant, type VirtualServer } from '../api'
import { RoleContext } from '../interaction'
import ServerToolAccess from './ServerToolAccess'
import BulkGrantDialog from './BulkGrantDialog'
import Ownership from '../pages/Ownership'

beforeEach(() => {
 HTMLDialogElement.prototype.showModal = function() {this.open=true}
 HTMLDialogElement.prototype.close = function() {this.open=false}
})
afterEach(() => {cleanup();vi.restoreAllMocks()})
const tool=(name:string,status:string)=>({upstream:'crm',tool:name,approvalStatus:status})
const server={slug:'chat',name:'Chat',tools:[tool('get','required'),tool('list','pending'),tool('write','revoked'),tool('echo','approved')]} as VirtualServer
const grant=(name:string,status:ToolGrant['status']='pending',canDecide=true):ToolGrant=>({virtualServer:'chat',upstream:'crm',tool:name,status,reason:'Needed for chat',version:4,canDecide})

test('request all excludes existing approvals, pending and revoked decisions; uses current versions', async()=>{
 vi.spyOn(api,'toolGrants').mockResolvedValue({items:[grant('list'),grant('write','revoked'),grant('echo','approved')]})
 const submit=vi.spyOn(api,'bulkToolGrants').mockResolvedValue({items:[],changed:1,unchanged:0,failed:0})
 const reload=vi.fn().mockResolvedValue(undefined)
 render(<MemoryRouter><ServerToolAccess server={server} canEdit onReload={reload}/></MemoryRouter>)
 expect(screen.getByText(/1 approval-cleared · 1 awaiting request · 1 pending approval/)).toBeTruthy()
 expect(screen.getByRole('link',{name:'View requests and decisions'}).getAttribute('href')).toBe('/ownership?section=access&server=chat')
 fireEvent.click(screen.getByRole('button',{name:'Request all missing approvals (1)'}))
 const dialog=await screen.findByRole('dialog')
 expect(within(dialog).getByText('get · pending')).toBeTruthy()
 fireEvent.change(within(dialog).getByRole('textbox'),{target:{value:'New capability'}})
 fireEvent.click(within(dialog).getByRole('button',{name:'Request access (1)'}))
 await waitFor(()=>expect(submit).toHaveBeenCalledExactlyOnceWith([{virtualServer:'chat',upstream:'crm',tool:'get',status:'pending',reason:'New capability',version:0}]))
 expect(await within(dialog).findByText('1 updated · 0 unchanged · 0 failed')).toBeTruthy()
 expect(reload).toHaveBeenCalledOnce()
})

test('bulk decisions expose partial failures and cannot replay the successful selection',async()=>{
 const submit=vi.spyOn(api,'bulkToolGrants').mockResolvedValue({items:[{virtualServer:'chat',upstream:'crm',tool:'list',result:'failed',code:'precondition_failed',message:'Refresh'}],changed:1,unchanged:0,failed:1})
 render(<BulkGrantDialog items={[grant('get'),grant('list')]} status="rejected" onClose={vi.fn()} onComplete={vi.fn().mockResolvedValue(undefined)}/>)
 const button=screen.getByRole('button',{name:'Reject requests (2)'}) as HTMLButtonElement
 expect(button.disabled).toBe(true)
 fireEvent.change(screen.getByRole('textbox'),{target:{value:'Unsupported use case'}})
 fireEvent.click(button)
 expect(await screen.findByText('1 updated · 0 unchanged · 1 failed')).toBeTruthy()
 expect(screen.getByText(/list: precondition_failed/)).toBeTruthy()
 expect(screen.queryByRole('button',{name:'Reject requests (2)'})).toBeNull()
 expect(submit).toHaveBeenCalledOnce()
})

test('owner review queue selects only authorized pending tools as a group',async()=>{
 vi.spyOn(api,'teams').mockResolvedValue({items:[]})
 vi.spyOn(api,'projects').mockResolvedValue({items:[]})
 vi.spyOn(api,'listVirtualServers').mockResolvedValue([])
 vi.spyOn(api,'toolGrants').mockResolvedValue({items:[grant('get'),grant('list'),grant('other','pending',false),grant('old','approved')]})
 render(<MemoryRouter initialEntries={['/ownership?section=access']}><RoleContext.Provider value="team"><Ownership/></RoleContext.Provider></MemoryRouter>)
 const group=await screen.findByRole('checkbox',{name:'chat → crm · 2 requests'})
 fireEvent.click(group)
 fireEvent.click(screen.getByRole('button',{name:'Approve selected pending (2)'}))
 const dialog=await screen.findByRole('dialog')
 expect(within(dialog).getByText('get · pending')).toBeTruthy()
 expect(within(dialog).getByText('list · pending')).toBeTruthy()
 expect(within(dialog).queryByText('other · pending')).toBeNull()
 expect(within(dialog).queryByText('old · approved')).toBeNull()
})

test('explicit re-request uses the revoked grant version, and viewers cannot submit requests',async()=>{
 vi.spyOn(api,'toolGrants').mockResolvedValue({items:[grant('write','revoked')]})
 const submit=vi.spyOn(api,'bulkToolGrants').mockResolvedValue({items:[],changed:1,unchanged:0,failed:0})
 const {unmount}=render(<MemoryRouter><ServerToolAccess server={server} canEdit onReload={vi.fn().mockResolvedValue(undefined)}/></MemoryRouter>)
 fireEvent.click(screen.getByText(/Select tools to request or request again/))
 fireEvent.click(screen.getByRole('checkbox',{name:'crm / write — Revoked'}))
 fireEvent.click(screen.getByRole('button',{name:'Request access for selected (1)'}))
 const dialog=await screen.findByRole('dialog')
 fireEvent.click(within(dialog).getByRole('button',{name:'Request access (1)'}))
 await waitFor(()=>expect(submit).toHaveBeenCalledWith([expect.objectContaining({tool:'write',status:'pending',version:4})]))
 unmount()
 render(<MemoryRouter><ServerToolAccess server={server} canEdit={false} onReload={vi.fn()}/></MemoryRouter>)
 expect(screen.queryByRole('button',{name:/Request all/})).toBeNull()
})
