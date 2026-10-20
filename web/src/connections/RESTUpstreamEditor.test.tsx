import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { useState } from 'react'
import { render } from '../test-utils'
import { api, type RESTConfig } from '../api'
import { upstream } from './fixtures'
import RESTUpstreamEditor, { RESTConnectionSettings } from './RESTUpstreamEditor'
import { emptyREST } from './rest-config'
import { RoleContext } from '../interaction'
HTMLDialogElement.prototype.showModal=function(){this.setAttribute('open','')}
HTMLDialogElement.prototype.close=function(){this.removeAttribute('open')}
afterEach(()=>{cleanup();vi.restoreAllMocks()})
const config:RESTConfig={...emptyREST(),auth:{type:'basic',username:'service',hasPassword:true},operations:[{name:'lookup',description:'Look up an order',enabled:true,method:'GET',path:'/orders',parameters:[],readOnly:true,headerNames:['X-Key']}]}
function Draft(){const [value,setValue]=useState(config);return <><RESTConnectionSettings value={value} onChange={setValue}/><RESTUpstreamEditor value={value} onChange={setValue} upstream={null}/></>}
test('operation edits remain local and omitted credentials stay masked',async()=>{
 const save=vi.spyOn(api,'patchUpstream');render(<Draft/>);expect((screen.getByLabelText('Password') as HTMLInputElement).value).toBe('')
 fireEvent.change(screen.getByLabelText('Tool name'),{target:{value:'find_order'}})
 expect(screen.queryByRole('dialog')).toBeNull();expect(screen.getByLabelText('MCP tool definition').textContent).toContain('find_order');expect(save).not.toHaveBeenCalled()
})
test('preview does not execute and admin execution requires confirmation',async()=>{
 const request=vi.spyOn(api,'restOperation').mockResolvedValue({method:'GET',url:'https://example.com/orders',headers:{}})
 const confirm=vi.spyOn(window,'confirm').mockReturnValue(false)
 render(<RoleContext.Provider value="admin"><RESTUpstreamEditor value={config} upstream={{...upstream,kind:'rest',rest:config}}/></RoleContext.Provider>)
 expect(request).not.toHaveBeenCalled();fireEvent.click(screen.getByRole('button',{name:'Preview request'}))
 await waitFor(()=>expect(request).toHaveBeenCalledWith(upstream.slug,'preview','lookup',{},upstream.version))
 fireEvent.click(screen.getByRole('button',{name:'Execute test request'}));await waitFor(()=>expect(confirm).toHaveBeenCalled());expect(request).toHaveBeenCalledTimes(1)
 confirm.mockReturnValue(true);fireEvent.click(screen.getByRole('button',{name:'Execute test request'}))
 await waitFor(()=>expect(request).toHaveBeenCalledWith(upstream.slug,'test','lookup',{},upstream.version))
})
test('viewers cannot execute and required identity prevents impersonation tests',()=>{
 render(<RoleContext.Provider value="viewer"><RESTUpstreamEditor value={config} upstream={{...upstream,kind:'rest',rest:{...config,requireIdentity:true}}}/></RoleContext.Provider>)
 expect((screen.getByRole('button',{name:'Execute test request'}) as HTMLButtonElement).disabled).toBe(true)
 expect((screen.getByRole('button',{name:'Preview request'}) as HTMLButtonElement).disabled).toBe(true)
})

test('path placeholders and visual query parameters update the MCP definition without requests',()=>{
 const request=vi.spyOn(api,'restOperation');render(<Draft/>)
 fireEvent.change(screen.getByLabelText('Operation path'),{target:{value:'/orders/{id}'}})
 expect(screen.getByLabelText('Parameter 1 name')).toHaveProperty('value','id')
 fireEvent.change(screen.getByLabelText('Parameter 1 type'),{target:{value:'integer'}})
 fireEvent.click(screen.getByRole('button',{name:'Add query parameter'}))
 fireEvent.change(screen.getByLabelText('Parameter 2 name'),{target:{value:'expanded'}})
 fireEvent.change(screen.getByLabelText('Parameter 2 type'),{target:{value:'boolean'}})
 const tool=JSON.parse(screen.getByLabelText('MCP tool definition').textContent!)
 expect(tool.inputSchema).toEqual({type:'object',properties:{id:{type:'integer'},expanded:{type:'boolean'}},required:['id'],additionalProperties:false})
 fireEvent.change(screen.getByLabelText('Operation path'),{target:{value:'/orders'}})
 expect(screen.getByLabelText('Parameter 1 name')).toHaveProperty('value','expanded')
 expect(request).not.toHaveBeenCalled()
})
test('invalid schema blocks save and switching tools requires an explicit discard',async()=>{
 const blocked=vi.fn();const confirm=vi.spyOn(window,'confirm').mockReturnValue(false)
 function Workspace(){const[value,setValue]=useState({...config,operations:[...config.operations,{...config.operations[0],name:'other'}]});return <RESTUpstreamEditor value={value} onChange={setValue} upstream={null} onInvalidChange={blocked}/>}
 render(<Workspace/>)
 fireEvent.change(screen.getByLabelText('Response schema'),{target:{value:'{'}})
 expect(blocked).toHaveBeenLastCalledWith(true)
 expect(screen.queryByLabelText('MCP tool definition')).toBeNull()
 fireEvent.click(screen.getByRole('button',{name:/other GET/}));await waitFor(()=>expect(confirm).toHaveBeenCalled())
 expect(screen.getByLabelText('Tool name')).toHaveProperty('value','lookup')
 confirm.mockReturnValue(true);fireEvent.click(screen.getByRole('button',{name:/other GET/}))
 await waitFor(()=>expect(screen.getByLabelText('Tool name')).toHaveProperty('value','other'))
 expect(blocked).toHaveBeenLastCalledWith(false)
})
test('inline import previews a candidate and appends it without saving or changing credentials',async()=>{
 vi.spyOn(api,'importREST').mockResolvedValue({title:'Orders',version:'3.1',servers:[],warnings:[],operations:[{id:'GET /new',method:'GET',path:'/new',summary:'New order lookup',tags:[],warnings:[],errors:[],operation:{...config.operations[0],name:'new_lookup',path:'/new'}}]})
 const save=vi.spyOn(api,'patchUpstream');render(<Draft/>)
 fireEvent.click(screen.getByRole('button',{name:'Import OpenAPI / Swagger'}))
 expect(screen.queryByRole('dialog')).toBeNull()
 fireEvent.change(screen.getByLabelText('OpenAPI / Swagger definition'),{target:{value:'definition'}})
 fireEvent.click(screen.getByRole('button',{name:'Analyze definition'}))
 fireEvent.click(await screen.findByRole('checkbox',{name:'GET /new'}))
 expect(screen.getByLabelText('MCP tool definition').textContent).toContain('new_lookup')
 fireEvent.click(screen.getByRole('button',{name:'Add 1 to draft'}))
 expect(screen.getByLabelText('Tool name')).toHaveProperty('value','new_lookup')
 expect(screen.getByLabelText('Username')).toHaveProperty('value','service')
 expect(screen.getByLabelText('Password')).toHaveProperty('value','')
 expect(save).not.toHaveBeenCalled()
})
test('advanced parameter schemas round-trip through visual edits without losing constraints',()=>{
 render(<Draft/>)
 fireEvent.change(screen.getByLabelText('Request parameters (JSON)'),{target:{value:JSON.stringify([{name:'state',in:'query',required:false,schema:{type:'string',enum:['open','closed'],default:'open'}}])}})
 expect(screen.getByLabelText('Parameter 1 name')).toHaveProperty('value','state')
 fireEvent.change(screen.getByLabelText('Parameter 1 description'),{target:{value:'Order status'}})
 const schema=JSON.parse(screen.getByLabelText('MCP tool definition').textContent!).inputSchema.properties.state
 expect(schema).toEqual({type:'string',enum:['open','closed'],default:'open',description:'Order status'})
 expect(JSON.parse((screen.getByLabelText('Request parameters (JSON)') as HTMLTextAreaElement).value)[0].schema).toEqual(schema)
})

test('successful saved test generates response fields in the draft without the MCP envelope', async()=>{
 vi.spyOn(window,'confirm').mockReturnValue(true)
 vi.spyOn(api,'restOperation').mockResolvedValue({structuredContent:{result:{origin:'127.0.0.1'}},content:[]})
 function Workspace(){const[value,setValue]=useState(config);return <RESTUpstreamEditor value={value} onChange={setValue} upstream={{...upstream,kind:'rest',rest:config}}/>}
 render(<RoleContext.Provider value="admin"><Workspace/></RoleContext.Provider>)
 fireEvent.click(screen.getByRole('button',{name:'Execute test request'}))
 const use=await screen.findByRole('button',{name:'Use test response as schema'})
 fireEvent.click(use)
 expect(screen.getByLabelText('Response.origin type')).toHaveProperty('value','string')
 const raw=JSON.parse((screen.getByLabelText('Response schema') as HTMLTextAreaElement).value)
 expect(raw).toEqual({type:'object',properties:{origin:{type:'string'}}})
})
test('failed test results cannot generate schemas', async()=>{
 vi.spyOn(window,'confirm').mockReturnValue(true)
 vi.spyOn(api,'restOperation').mockResolvedValue({isError:true,content:[{type:'text',text:'Request failed'}]})
 render(<RoleContext.Provider value="admin"><RESTUpstreamEditor value={config} upstream={{...upstream,kind:'rest',rest:config}}/></RoleContext.Provider>)
 fireEvent.click(screen.getByRole('button',{name:'Execute test request'}))
 await screen.findByLabelText('REST operation result')
 expect(screen.queryByRole('button',{name:'Use test response as schema'})).toBeNull()
})

test('visual response selection updates MCP preview and schema drift blocks saving',()=>{
 const blocked=vi.fn()
 function Workspace(){const[value,setValue]=useState<RESTConfig>({...config,operations:[{...config.operations[0],responseSchema:{type:'object',properties:{id:{type:'integer'},secret:{type:'string'}},required:['id','secret']}}]});return <RESTUpstreamEditor value={value} onChange={setValue} upstream={null} onInvalidChange={blocked}/>}
 render(<Workspace/>);
 fireEvent.click(screen.getByRole('button',{name:'Selected fields only'}))
 fireEvent.click(screen.getByLabelText('Return field secret'))
 let tool=JSON.parse(screen.getByLabelText('MCP tool definition').textContent!)
 expect(tool.outputSchema.properties.result).toEqual({type:'object',properties:{id:{type:'integer'}},required:['id'],additionalProperties:false})
 fireEvent.click(screen.getByRole('button',{name:'Remove id'}))
 expect(blocked).toHaveBeenLastCalledWith(true)
 expect(screen.queryByLabelText('MCP tool definition')).toBeNull()
 fireEvent.click(screen.getByRole('button',{name:'Full response'}))
 expect(blocked).toHaveBeenLastCalledWith(false)
 tool=JSON.parse(screen.getByLabelText('MCP tool definition').textContent!)
 expect(tool.outputSchema.properties.result.properties.secret).toEqual({type:'string'})
})
