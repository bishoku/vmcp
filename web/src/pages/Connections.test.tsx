import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { api } from '../api'
import { server, upstream, key, binding } from '../connections/fixtures'
import Connections from './Connections'

beforeEach(()=>{
 vi.spyOn(api,'listVirtualServers').mockResolvedValue([server])
 vi.spyOn(api,'listUpstreams').mockResolvedValue([upstream])
 vi.spyOn(api,'listAgentKeys').mockResolvedValue([key])
 vi.spyOn(api,'listAudit')
 vi.spyOn(api,'listBindings').mockResolvedValue([binding])
})
afterEach(()=>{cleanup();vi.restoreAllMocks();Reflect.deleteProperty(HTMLDialogElement.prototype,'showModal');Reflect.deleteProperty(HTMLDialogElement.prototype,'close')})
const open=()=>render(<MemoryRouter><Connections/></MemoryRouter>)
test('configuration graph and accessible table show the same grants and selected upstream',async()=>{
 open();await screen.findByText('Configured relationships (3)')
 const flow=screen.getByRole('region',{name:'Connection flow'})
 expect(within(flow).getByRole('button',{name:/team-key/})).toBeTruthy()
 expect(within(flow).getByRole('link',{name:'Open Demo configuration'}).getAttribute('href')).toBe('/virtual-servers/demo/overview')
 expect(screen.getByText(/not active clients or live traffic/)).toBeTruthy()
 expect(api.listAudit).not.toHaveBeenCalled() // no eager audit or gateway calls
})
test('search includes aliases and policies and dialog reveals health timestamps and missing objects',async()=>{
 HTMLDialogElement.prototype.showModal=function(){this.open=true}
 HTMLDialogElement.prototype.close=function(){this.open=false}
 open();await screen.findByText('Configured relationships (3)')
 fireEvent.change(screen.getByLabelText('Filter relationships'),{target:{value:'doc://team'}})
 await screen.findByText('Configured relationships (1)')
 fireEvent.click(screen.getByRole('button',{name:'Inspect upstream demo to alpha'}))
 const dialog=screen.getByRole('dialog',{name:'demo → alpha'})
 expect(within(dialog).getByText('Missing object')).toBeTruthy()
 expect(within(dialog).getByText(/Checked:/)).toBeTruthy()
 expect(within(dialog).getByRole('link',{name:'Open configuration'}).getAttribute('href')).toBe('/upstreams/alpha')
})
test('large tables render 50 rows at a time and filtering resets the visible page',async()=>{
 vi.mocked(api.listAgentKeys).mockResolvedValue(Array.from({length:120},(_,i)=>({...key,slug:`key-${i}`})))
 open();await screen.findByText('Configured relationships (122)')
 const table=screen.getByRole('table')
 expect(within(table).getAllByRole('row')).toHaveLength(51)
 fireEvent.click(screen.getByText('Show 50 more relationships'))
 expect(within(table).getAllByRole('row')).toHaveLength(101)
 fireEvent.change(screen.getByLabelText('Filter relationships'),{target:{value:'key-119'}})
 expect(within(table).getAllByRole('row')).toHaveLength(2)
})
test('partial catalog failure remains visible while successful relationships are retained',async()=>{
 vi.mocked(api.listBindings).mockRejectedValue(new Error('bindings offline'))
 open();await waitFor(()=>expect(screen.getByRole('alert').textContent).toContain('bindings offline'))
 expect(screen.getByText('Configured relationships (2)')).toBeTruthy()
})

test('overview includes every server and ignores relationship-table filters',async()=>{
 vi.mocked(api.listVirtualServers).mockResolvedValue([server,{...server,slug:'second',name:'Second',tools:[],catalog:[]}])
 vi.mocked(api.listUpstreams).mockResolvedValue([upstream,{...upstream,slug:'idle',name:'Idle upstream'}])
 open();await screen.findByText('Configured relationships (3)')
 const flow=screen.getByRole('region',{name:'Connection flow'})
 expect(within(flow).getByRole('button',{name:'Second'})).toBeTruthy()
 expect(within(flow).getByRole('button',{name:'Idle upstream'})).toBeTruthy()
 fireEvent.change(screen.getByLabelText('Filter relationships'),{target:{value:'no match'}})
 expect(within(flow).getByRole('button',{name:'Demo'})).toBeTruthy()
 expect(within(flow).getAllByTestId('flow-ribbon')).toHaveLength(3)
})
test('tool selection highlights only virtual servers that map that upstream tool',async()=>{
 vi.mocked(api.listVirtualServers).mockResolvedValue([server,{...server,slug:'second',name:'Second',tools:[{...server.tools[0],tool:'other',exposedName:'other'}],catalog:[]}])
 vi.mocked(api.listAgentKeys).mockResolvedValue([key,{...key,slug:'other-key',virtualServers:['second']}])
 open();await screen.findByText('Configured relationships (5)')
 const flow=screen.getByRole('region',{name:'Connection flow'})
 fireEvent.click(within(flow).getByRole('button',{name:'Alpha'}))
 fireEvent.click(within(flow).getByRole('button',{name:'Trace upstream tool alpha / lookup'}))
 expect(within(flow).getAllByTestId('flow-ribbon').filter(e=>e.getAttribute('data-highlighted')==='true')).toHaveLength(5)
 expect(within(flow).getByRole('status').textContent).toContain('Tracing alpha / lookup through all virtual servers')
 expect(api.listAudit).not.toHaveBeenCalled()
})
test('large inventories bundle all identities and expand groups without truncation',async()=>{
 vi.mocked(api.listAgentKeys).mockResolvedValue(Array.from({length:120},(_,i)=>({...key,slug:`key-${String(i).padStart(3,'0')}`})))
 open();await screen.findByText('Configured relationships (122)')
 const flow=screen.getByRole('region',{name:'Connection flow'})
 expect(within(flow).getByText('123 nodes · 122 connections. All configured connections.')).toBeTruthy()
 fireEvent.click(within(flow).getAllByRole('button',{name:'Expand 11 identities'})[0])
 expect(within(flow).getByRole('navigation',{name:'Flow drill-down'})).toBeTruthy()
 expect(within(flow).getByRole('button',{name:'key-000'})).toBeTruthy()
 expect(within(flow).queryByRole('button',{name:'key-119'})).toBeNull()
 fireEvent.click(within(flow).getByRole('button',{name:'Reset to overview'}))
 expect(within(flow).getAllByRole('button',{name:'Expand 11 identities'}).length).toBeGreaterThan(0)
})


test('server selection unfolds aliases and upstream definitions in the Sankey, without a tool table',async()=>{
 const tools=Array.from({length:40},(_,i)=>({...server.tools[0],tool:`tool-${String(i).padStart(2,'0')}`,exposedName:`alias-${String(i).padStart(2,'0')}`}))
 vi.mocked(api.listVirtualServers).mockResolvedValue([{...server,tools,catalog:[]}])
 open();await screen.findByText('Configured relationships (3)')
 const flow=screen.getByRole('region',{name:'Connection flow'})
 fireEvent.click(within(flow).getByRole('button',{name:'Demo'}))
 expect(within(flow).getByRole('group',{name:'Identities through exposed tools to upstream definitions'})).toBeTruthy()
 expect(within(flow).getByText('Exposed tools')).toBeTruthy()
 expect(within(flow).getByText('Upstream definitions')).toBeTruthy()
 expect(within(flow).queryByRole('table')).toBeNull()
 fireEvent.click(within(flow).getAllByRole('button',{name:'Expand 4 tools'})[0])
 expect(within(flow).getByRole('button',{name:'Trace alias demo / alias-00'})).toBeTruthy()
 fireEvent.click(within(flow).getByRole('button',{name:'Trace alias demo / alias-00'}))
 expect(within(flow).getAllByTestId('flow-ribbon').filter(e=>e.getAttribute('data-highlighted')==='true')).toHaveLength(5)
 fireEvent.click(within(flow).getByRole('button',{name:'Back one level'}))
 expect(within(flow).getAllByRole('button',{name:'Expand 4 tools'}).length).toBeGreaterThan(0)
 fireEvent.change(within(flow).getByLabelText('Find tool mapping'),{target:{value:'tool-39'}})
 expect(within(flow).getByRole('button',{name:'Trace upstream tool alpha / tool-39'})).toBeTruthy()
 expect(within(flow).queryByRole('button',{name:'Trace upstream tool alpha / tool-00'})).toBeNull()
 fireEvent.click(within(flow).getByRole('button',{name:'Reset to overview'}))
 expect(within(flow).queryByText('Upstream definitions')).toBeNull()
 expect(api.listAudit).not.toHaveBeenCalled()
})
