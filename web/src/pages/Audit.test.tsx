import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import Audit from './Audit'
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
