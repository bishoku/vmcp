import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { render } from '../test-utils'
import { api, ApiError, type RESTImportResult, type RESTOperation } from '../api'
import RESTImportDialog from './RESTImportDialog'
const operation:RESTOperation={name:'lookup',method:'GET',path:'/orders',enabled:true,description:'Read orders',parameters:[],readOnly:false}
const result:RESTImportResult={title:'Orders',version:'3.1.0',servers:['https://orders.example'],warnings:['Security settings are preserved.'],operations:[{id:'GET /orders',method:'GET',path:'/orders',summary:'Read orders',tags:['orders'],operation,warnings:[],errors:[]},{id:'POST /files',method:'POST',path:'/files',summary:'Upload',tags:[],warnings:[],errors:['Multipart unsupported']}]}
beforeEach(()=>{HTMLDialogElement.prototype.showModal=function(){this.open=true};HTMLDialogElement.prototype.close=function(){this.open=false};vi.spyOn(api,'importREST').mockResolvedValue(result)})
afterEach(()=>{cleanup();vi.restoreAllMocks();Reflect.deleteProperty(HTMLDialogElement.prototype,'showModal');Reflect.deleteProperty(HTMLDialogElement.prototype,'close')})
async function analyze(){fireEvent.change(screen.getByLabelText('OpenAPI / Swagger definition'),{target:{value:'openapi: 3.1.0'}});fireEvent.click(screen.getByRole('button',{name:'Analyze definition'}));await screen.findByText('Multipart unsupported',{exact:false})}
test('analysis is explicit, unsupported operations cannot be selected, and only selected operations reach the draft',async()=>{
 const apply=vi.fn();render(<RESTImportDialog existing={[]} onApply={apply} onClose={()=>{}}/>);expect(api.importREST).not.toHaveBeenCalled();await analyze()
 expect((screen.getByRole('checkbox',{name:'GET /orders'}) as HTMLInputElement).checked).toBe(false)
 expect((screen.getByRole('checkbox',{name:'POST /files'}) as HTMLInputElement).disabled).toBe(true)
 expect(apply).not.toHaveBeenCalled();fireEvent.click(screen.getByRole('checkbox',{name:'GET /orders'}));fireEvent.click(screen.getByRole('button',{name:'Add 1 to draft'}));expect(apply).toHaveBeenCalledWith([operation])
})
test('name collisions preserve existing tools and changing source invalidates the analysis',async()=>{
 const apply=vi.fn();render(<RESTImportDialog existing={[{...operation,path:'/existing'}]} onApply={apply} onClose={()=>{}}/>);await analyze();expect(screen.getByText('Tool: lookup_2',{exact:false})).toBeTruthy()
 fireEvent.click(screen.getByRole('checkbox',{name:'GET /orders'}));fireEvent.change(screen.getByLabelText('OpenAPI / Swagger definition'),{target:{value:'changed'}})
 expect(screen.queryByRole('checkbox',{name:'GET /orders'})).toBeNull();expect((screen.getByRole('button',{name:'Add 0 to draft'}) as HTMLButtonElement).disabled).toBe(true);expect(apply).not.toHaveBeenCalled()
})
test('already configured method/path stays unchanged and cancel respects dirty confirmation',async()=>{
 const close=vi.fn(),apply=vi.fn(),confirm=vi.spyOn(window,'confirm').mockReturnValue(false)
 render(<RESTImportDialog existing={[operation]} onApply={apply} onClose={close}/>);await analyze();expect((screen.getByRole('checkbox',{name:'GET /orders'}) as HTMLInputElement).disabled).toBe(true)
 fireEvent.click(screen.getByRole('button',{name:'Cancel import'}));await waitFor(()=>expect(confirm).toHaveBeenCalled());expect(close).not.toHaveBeenCalled();confirm.mockReturnValue(true);fireEvent.click(screen.getByRole('button',{name:'Cancel import'}));await waitFor(()=>expect(close).toHaveBeenCalled());expect(apply).not.toHaveBeenCalled()
})
test('large inventories render incrementally and refuse selection beyond the upstream limit',async()=>{
 vi.mocked(api.importREST).mockResolvedValue({...result,operations:Array.from({length:250},(_,n)=>({...result.operations[0],id:`GET /orders/${n}`,path:`/orders/${n}`,operation:{...operation,name:`lookup_${n}`,path:`/orders/${n}`}}))})
 render(<RESTImportDialog existing={[]} onApply={()=>{}} onClose={()=>{}}/>);fireEvent.change(screen.getByLabelText('OpenAPI / Swagger definition'),{target:{value:'doc'}});fireEvent.click(screen.getByRole('button',{name:'Analyze definition'}));await screen.findByRole('button',{name:'Show more operations'})
 expect(screen.getAllByRole('checkbox')).toHaveLength(50);fireEvent.click(screen.getByRole('button',{name:'Select available results'}));await screen.findByText('Only 200 slots remain. Narrow the search or select fewer operations.');expect((screen.getByRole('button',{name:'Add 0 to draft'}) as HTMLButtonElement).disabled).toBe(true)
 fireEvent.click(screen.getByRole('button',{name:'Show more operations'}));expect(screen.getAllByRole('checkbox')).toHaveLength(100)
})

test('parser validation details are visible and no stale selection survives a failed analysis',async()=>{
 vi.mocked(api.importREST).mockRejectedValue(new ApiError(422,'Validation failed','validation_failed',[{field:'/document',message:'Duplicate object keys are not allowed.',code:'validation_failed'}]))
 render(<RESTImportDialog existing={[]} onApply={()=>{}} onClose={()=>{}}/>);fireEvent.change(screen.getByLabelText('OpenAPI / Swagger definition'),{target:{value:'invalid'}});fireEvent.click(screen.getByRole('button',{name:'Analyze definition'}));await screen.findByText('Duplicate object keys are not allowed.');expect((screen.getByRole('button',{name:'Add 0 to draft'}) as HTMLButtonElement).disabled).toBe(true)
})
test('blocked reasons are visible and availability filters preserve selection',async()=>{
 render(<RESTImportDialog existing={[]} onApply={()=>{}} onClose={()=>{}}/>);await analyze()
 expect(screen.getByText('Cannot select: Multipart unsupported')).toBeTruthy()
 expect(screen.getByText(/This adapter supports JSON requests/)).toBeTruthy()
 fireEvent.click(screen.getByRole('checkbox',{name:'GET /orders'}))
 fireEvent.change(screen.getByLabelText('Filter import availability'),{target:{value:'blocked'}})
 expect(screen.queryByRole('checkbox',{name:'GET /orders'})).toBeNull()
 expect(screen.getByRole('checkbox',{name:'POST /files'})).toHaveProperty('disabled',true)
 expect(screen.getByRole('button',{name:'Add 1 to draft'})).toHaveProperty('disabled',false)
 fireEvent.change(screen.getByLabelText('Filter import availability'),{target:{value:'available'}})
 expect(screen.getByRole('checkbox',{name:'GET /orders'})).toHaveProperty('checked',true)
})
