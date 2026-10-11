import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { source, useSources } from './requests'
afterEach(cleanup)
test('independent source failure retains successful data and reports the failing panel',async()=>{
 const accept=vi.fn()
 const {result}=renderHook(()=>useSources(()=>[
  source('Available',async()=>['data'],accept),source('Offline',async()=>{throw new Error('unavailable')},()=>{}),
 ]))
 await act(()=>result.current.load())
 expect(accept).toHaveBeenCalledWith(['data']);expect(result.current.readError).toBe('Offline: unavailable');expect(result.current.loading).toBe(false)
})
test('new reads and unmount cancel obsolete requests even if a source ignores cancellation',async()=>{
 let finish!:(value:string)=>void
 const read=vi.fn<(signal:AbortSignal)=>Promise<string>>().mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve})).mockResolvedValue('current')
 const accept=vi.fn()
 const {result,unmount}=renderHook(()=>useSources(()=>[source('Items',read,accept)]))
 let old!:Promise<void>
 act(()=>{old=result.current.load()})
 const firstSignal=read.mock.calls[0][0]
 await act(()=>result.current.load())
 expect(firstSignal.aborted).toBe(true)
 await act(async()=>{finish('obsolete');await old})
 expect(accept.mock.calls).toEqual([['current']])
 unmount();await waitFor(()=>expect(read.mock.calls[1][0].aborted).toBe(true))
})
