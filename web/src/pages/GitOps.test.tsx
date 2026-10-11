import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import GitOps from './GitOps'
import { api } from '../api'
afterEach(cleanup)
test('preview is required, edits invalidate it, and apply retains preview revision',async()=>{
 vi.spyOn(api,'info').mockResolvedValue({version:'test',configRevision:42})
 const apply=vi.spyOn(api,'applyConfig').mockResolvedValue({changes:[{kind:'upstream',slug:'x',action:'delete'}]})
 vi.spyOn(api,'exportConfig').mockResolvedValue({apiVersion:'vmcp/v1',upstreams:[],virtualServers:[]})
 const confirm=vi.spyOn(window,'confirm').mockReturnValue(true)
 render(<GitOps canManage/>)
 expect(screen.queryByText('Apply preview')).toBeNull()
 fireEvent.change(screen.getByLabelText('Configuration document'),{target:{value:'{"apiVersion":"vmcp/v1","upstreams":[],"virtualServers":[]}'}})
 fireEvent.click(screen.getByRole('checkbox'))
 fireEvent.click(screen.getByText('Preview changes'))
 await screen.findByText('Apply preview')
 fireEvent.click(screen.getByText('Apply preview'))
 await waitFor(()=>expect(apply).toHaveBeenCalledTimes(2))
 expect(apply.mock.calls[1].slice(1)).toEqual([false,true,42])
 expect(confirm).toHaveBeenCalled()
})
test('read role can export but cannot import or apply',()=>{render(<GitOps canManage={false}/>);expect(screen.getByText('Export configuration')).toBeTruthy();expect(screen.queryByText('Preview changes')).toBeNull()})
test('pending preview locks the document and prevents duplicate submissions',async()=>{
 let finish!:(value:{changes:[]})=>void
 vi.spyOn(api,'info').mockResolvedValue({version:'test',configRevision:8})
 const apply=vi.spyOn(api,'applyConfig').mockImplementation(()=>new Promise(resolve=>{finish=resolve}))
 render(<GitOps canManage/>)
 const document=screen.getByLabelText('Configuration document')
 fireEvent.change(document,{target:{value:'{"apiVersion":"vmcp/v1","upstreams":[],"virtualServers":[]}'}})
 const preview=screen.getByText('Preview changes')
 fireEvent.click(preview);fireEvent.click(preview)
 await waitFor(()=>expect(apply).toHaveBeenCalledTimes(1))
 expect((document as HTMLTextAreaElement).closest('fieldset')?.disabled).toBe(true)
 finish({changes:[]});await screen.findByText('Apply preview')
 expect((document as HTMLTextAreaElement).closest('fieldset')?.disabled).toBe(false)
})
