import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react'
import {afterEach,expect,test,vi} from 'vitest'
import CatalogPicker from './CatalogPicker'
import {api,type Upstream} from '../api'
afterEach(()=>{cleanup();vi.restoreAllMocks()})
test('only discovered objects can be added and selections can be removed',async()=>{
 vi.spyOn(api,'listCatalog').mockResolvedValue({resources:[{uri:'doc://public',name:'Public'}],resourceTemplates:[],prompts:[{name:'review'}],capabilities:{}})
 const onChange=vi.fn();const upstreams=[{slug:'alpha',name:'Alpha'}] as Upstream[]
 const view=render(<CatalogPicker upstreams={upstreams} value={[]} onChange={onChange}/>)
 await screen.findByText('alpha · resource · doc://public')
 fireEvent.change(screen.getByLabelText('Add catalog object'),{target:{value:'alpha\0resource\0doc://public'}})
 expect(onChange).toHaveBeenCalledWith([{upstream:'alpha',kind:'resource',name:'doc://public'}])
 view.rerender(<CatalogPicker upstreams={upstreams} value={[{upstream:'alpha',kind:'prompt',name:'removed'}]} onChange={onChange}/>)
 await screen.findByText('Missing from upstream discovery; this selection is not exposed.')
 fireEvent.click(screen.getByText('Remove selection'));expect(onChange).toHaveBeenLastCalledWith([])
})
test('discovery errors are visible and preserve the draft',async()=>{
 vi.spyOn(api,'listCatalog').mockRejectedValue(new Error('unavailable'));const onChange=vi.fn()
 render(<CatalogPicker upstreams={[{slug:'alpha'}] as Upstream[]} value={[{upstream:'alpha',kind:'resource',name:'doc://saved'}]} onChange={onChange}/>)
 await waitFor(()=>expect(screen.getByRole('alert').textContent).toContain('unavailable'));expect(onChange).not.toHaveBeenCalled();expect(screen.getByText('alpha · resource · doc://saved')).toBeTruthy()
})
test('one unavailable upstream does not hide another catalog and retry recovers',async()=>{
 const discovery=vi.spyOn(api,'listCatalog').mockImplementation(async slug=>{
  if(slug==='broken')throw new Error('offline')
  return {resources:[],resourceTemplates:[],prompts:[{name:'review'}],capabilities:{}}
 })
 render(<CatalogPicker upstreams={[{slug:'good'},{slug:'broken'}] as Upstream[]} value={[]} onChange={()=>{}}/>)
 expect(await screen.findByText('good · prompt · review')).toBeTruthy()
 expect((await screen.findByRole('alert')).textContent).toContain('broken')
 discovery.mockResolvedValue({resources:[],resourceTemplates:[],prompts:[{name:'recovered'}],capabilities:{}})
 fireEvent.click(screen.getByText('Retry catalogs'))
 expect(await screen.findByText('broken · prompt · recovered')).toBeTruthy()
 expect(screen.queryByRole('alert')).toBeNull()
})
