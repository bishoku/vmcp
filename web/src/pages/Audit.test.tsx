import { act, cleanup, fireEvent, render as renderUI, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import type { ReactNode } from 'react'
import Audit from './Audit'
const render=(ui:ReactNode)=>renderUI(<MemoryRouter>{ui}</MemoryRouter>)
import { api, type AuditEvent } from '../api'
afterEach(()=>{cleanup();vi.restoreAllMocks()})
const event=(id:number,tool:string)=>({id,tool,time:'2026-10-11T12:00:00Z',status:'ok',durationMs:1,virtualServer:'demo'}) as AuditEvent
test('obsolete audit responses cannot replace the current filter and are aborted',async()=>{
 let old!:(value:{items:AuditEvent[]})=>void
 const list=vi.spyOn(api,'listAudit').mockImplementationOnce(()=>new Promise(resolve=>{old=resolve})).mockResolvedValue({items:[event(2,'current')]})
 render(<Audit/>);await waitFor(()=>expect(list).toHaveBeenCalledTimes(1))
 const signal=list.mock.calls[0][1]!
 fireEvent.change(screen.getByPlaceholderText('Tool'),{target:{value:'current'}})
 expect(signal.aborted).toBe(true)
 await screen.findByText('current')
 await act(async()=>old({items:[event(1,'obsolete')]}))
 expect(screen.queryByText('obsolete')).toBeNull();expect(screen.getByText('current')).toBeTruthy()
})
test('load more prevents duplicate requests and duplicate event rows',async()=>{
 let finish!:(value:{items:AuditEvent[]})=>void
 const list=vi.spyOn(api,'listAudit').mockResolvedValueOnce({items:[event(1,'first')],nextCursor:'next'}).mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve}))
 render(<Audit/>);const more=await screen.findByText('Load more')
 fireEvent.click(more);fireEvent.click(more);expect(list).toHaveBeenCalledTimes(2)
 await act(async()=>finish({items:[event(1,'first'),event(2,'second')]}))
 expect(screen.getAllByText('first')).toHaveLength(1);expect(screen.getByText('second')).toBeTruthy()
})
test('groups only operation IDs, keeps raw access and never infers failure from a missing outcome',async()=>{
 vi.spyOn(api,'listAudit').mockResolvedValue({items:[
 {...event(1,'paired'),operationId:'pair',stage:'admission',status:'started'},
 {...event(2,'paired'),operationId:'pair',stage:'outcome'},
 {...event(3,'pending'),operationId:'pending',stage:'admission',status:'started'},
 {...event(4,'uncorrelated'),requestId:'same'}, {...event(5,'uncorrelated'),requestId:'same'},
 ]})
 render(<Audit initialOperation="pair"/>);await screen.findByText('paired')
 expect(screen.getAllByText('paired')).toHaveLength(1)
 expect(screen.getByText('outcome not loaded')).toBeTruthy()
 expect(screen.getAllByText('uncorrelated')).toHaveLength(2)
 fireEvent.click(screen.getByText('Raw events'));expect(screen.getAllByText('paired')).toHaveLength(2)
 expect(screen.getByLabelText('Operation ID').getAttribute('value')).toBe('pair')
})

test('operations combine upstream attempts under the tool outcome while raw events remain separate',async()=>{
 vi.spyOn(api,'listAudit').mockResolvedValue({items:[
  {...event(6,'createOrder'),operationId:'retry',parentOperationId:'tool',stage:'outcome',status:'error',durationMs:5,reason:'first attempt failed'},
  {...event(5,'createOrder'),operationId:'retry',parentOperationId:'tool',stage:'admission',status:'started'},
  {...event(4,'createOrder'),operationId:'upstream',parentOperationId:'tool',stage:'outcome',durationMs:10},
  {...event(3,'createOrder'),operationId:'upstream',parentOperationId:'tool',stage:'admission',status:'started'},
  {...event(2,'createOrder'),operationId:'tool',stage:'outcome',durationMs:50},
  {...event(1,'createOrder'),operationId:'tool',stage:'admission',status:'started'},
 ]})
 render(<Audit/>)
 await screen.findByText('createOrder')
 expect(screen.getAllByRole('button',{name:'Details'})).toHaveLength(1)
 expect(screen.getByText('6 records')).toBeTruthy()
 expect(screen.getByText('50 ms')).toBeTruthy()
 expect(screen.queryByText('first attempt failed')).toBeNull()
 fireEvent.click(screen.getByText('Raw events'))
 expect(screen.getAllByRole('button',{name:'Details'})).toHaveLength(6)
 expect(screen.getByText('first attempt failed')).toBeTruthy()
})

test('loading parent records merges into the existing child group and resolves its missing outcome',async()=>{
 vi.spyOn(api,'listAudit').mockResolvedValueOnce({items:[
  {...event(3,'createOrder'),operationId:'child',parentOperationId:'tool',stage:'outcome',durationMs:10},
 ],nextCursor:'parent-page'}).mockResolvedValueOnce({items:[
  {...event(2,'createOrder'),operationId:'tool',stage:'outcome',durationMs:50},
  {...event(1,'createOrder'),operationId:'tool',stage:'admission',status:'started'},
 ]})
 render(<Audit/>)
 await screen.findByText('outcome not loaded')
 expect(screen.queryByText('10 ms')).toBeNull()
 fireEvent.click(screen.getByRole('button',{name:'Load more'}))
 await screen.findByText('50 ms')
 expect(screen.queryByText('outcome not loaded')).toBeNull()
 expect(screen.getAllByRole('button',{name:'Details'})).toHaveLength(1)
 expect(screen.getByText('3 records')).toBeTruthy()
})


test('correlation filters preserve supported request and stage fields and clear together',async()=>{
 const list=vi.spyOn(api,'listAudit').mockResolvedValue({items:[]})
 render(<Audit initialQuery={{requestId:'request-1',stage:'admission',from:'2026-10-11T00:00:00Z',incomplete:'true'}}/>)
 await waitFor(()=>expect(list).toHaveBeenCalled())
 expect(screen.getByLabelText('Request ID').getAttribute('value')).toBe('request-1')
 expect((screen.getByLabelText('Stage') as HTMLSelectElement).value).toBe('admission')
 fireEvent.change(screen.getByLabelText('Request ID'),{target:{value:'request-2'}})
 await waitFor(()=>expect(list).toHaveBeenLastCalledWith(expect.objectContaining({requestId:'request-2',stage:'admission',incomplete:'true',from:'2026-10-11T00:00:00Z'}),expect.any(AbortSignal)))
 fireEvent.click(screen.getByRole('button',{name:'Clear all filters'}))
 await waitFor(()=>expect(list).toHaveBeenLastCalledWith({},expect.any(AbortSignal)))
 expect((screen.getByLabelText('Incomplete operations only') as HTMLInputElement).checked).toBe(false)
})

test('a failed refresh retains records and clearly identifies the read failure',async()=>{
 vi.spyOn(api,'listAudit').mockResolvedValueOnce({items:[event(1,'retained')]}).mockRejectedValueOnce(new Error('Audit storage unavailable'))
 render(<Audit/>);await screen.findByText('retained')
 fireEvent.click(screen.getByRole('button',{name:'Refresh'}))
 await waitFor(()=>expect(screen.getByRole('alert').textContent).toContain('previous successful query'))
 expect(screen.getByText('retained')).toBeTruthy()
 expect(screen.queryByText('No events match.')).toBeNull()
})

test('details retain raw records and operation links without mixing correlation IDs',async()=>{
 Object.defineProperty(HTMLDialogElement.prototype,'showModal',{configurable:true,value:function(this:HTMLDialogElement){this.open=true}})
 Object.defineProperty(HTMLDialogElement.prototype,'close',{configurable:true,value:function(this:HTMLDialogElement){this.open=false}})
 try{
  vi.spyOn(api,'listAudit').mockResolvedValue({items:[{...event(2,'lookup'),operationId:'pair',requestId:'request-out',traceId:'trace-out',stage:'outcome',status:'error',reason:'Upstream timeout'},{...event(1,'lookup'),operationId:'pair',requestId:'request-in',traceId:'trace-in',stage:'admission',status:'started'}]})
  render(<Audit/>)
  fireEvent.click(await screen.findByRole('button',{name:'Details'}))
  const dialog=screen.getByRole('dialog',{name:'Audit details'})
  expect(within(dialog).getByRole('link',{name:'Open operation'}).getAttribute('href')).toBe('/audit?operationId=pair')
  expect(within(dialog).getAllByRole('button',{name:'Copy request ID'})).toHaveLength(2)
  expect(within(dialog).getAllByRole('button',{name:'Copy trace ID'})).toHaveLength(2)
  expect([...dialog.querySelectorAll('pre')].map(pre=>JSON.parse(pre.textContent!).stage)).toEqual(['outcome','admission'])
  expect(within(dialog).getByRole('region',{name:'Operation summary'}).textContent).toContain('Upstream timeout')
 }finally{cleanup();Reflect.deleteProperty(HTMLDialogElement.prototype,'showModal');Reflect.deleteProperty(HTMLDialogElement.prototype,'close')}
})

test('activity filter requests virtual calls with their linked upstream records in both views',async()=>{
 const list=vi.spyOn(api,'listAudit').mockImplementation(async q=>({items:q.activity==='virtual'?[
  {...event(2,'createOrder'),scope:'upstream',operationId:'child',parentOperationId:'root',stage:'outcome'},
  {...event(1,'createOrder'),scope:'virtual',operationId:'root',stage:'outcome'},
 ]:[],nextCursor:'9'}))
 render(<Audit initialQuery={{activity:'control'}}/>);
 await waitFor(()=>expect(list).toHaveBeenCalled())
 expect((screen.getByLabelText('Activity') as HTMLSelectElement).value).toBe('control')
 fireEvent.change(screen.getByLabelText('Activity'),{target:{value:'virtual'}})
 await waitFor(()=>expect(list.mock.lastCall?.[0].activity).toBe('virtual'))
 expect(list.mock.lastCall?.[0].cursor).toBeUndefined()
 expect(screen.getByRole('button',{name:'Remove Activity filter'})).toBeTruthy()
 await waitFor(()=>expect(screen.getAllByRole('button',{name:'Details'})).toHaveLength(1))
 const calls=list.mock.calls.length
 fireEvent.click(screen.getByText('Raw events'))
 expect(screen.getAllByRole('button',{name:'Details'})).toHaveLength(2)
 expect(list.mock.calls.length).toBe(calls)
 const link=screen.getByRole('button',{name:'Copy audit filter link'})
 expect(link).toBeTruthy()
 fireEvent.click(screen.getByRole('button',{name:'Remove Activity filter'}))
 await waitFor(()=>expect(list.mock.lastCall?.[0].activity).toBeUndefined())
})
