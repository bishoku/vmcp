import {useState} from 'react'
import {cleanup,fireEvent,screen,waitFor,within} from '@testing-library/react'
import {afterEach,expect,test} from 'vitest'
import {render} from '../test-utils'
import {type RESTConfig} from '../api'
import {emptyREST} from './rest-config'
import RESTUpstreamEditor from './RESTUpstreamEditor'
HTMLDialogElement.prototype.showModal=function(){this.setAttribute('open','')}
HTMLDialogElement.prototype.close=function(){this.removeAttribute('open')}
afterEach(cleanup)
function Draft(){const [value,setValue]=useState<RESTConfig>({...emptyREST(),mock:{}});return <><RESTUpstreamEditor value={value} onChange={setValue} upstream={null}/><output data-testid="draft">{JSON.stringify(value)}</output></>}
test('mock operation drafts support dynamic rules without leaking them into MCP schema',async()=>{
 render(<Draft/>);fireEvent.click(screen.getByRole('button',{name:'Add operation'}))
 const editor=within(screen.getByRole('dialog',{name:'Edit REST operation'}))
 fireEvent.change(editor.getByLabelText('Tool name'),{target:{value:'sample'}})
 fireEvent.click(editor.getByRole('button',{name:'Mock responses'}))
 fireEvent.click(editor.getByRole('button',{name:'Add dynamic response'}))
 fireEvent.click(editor.getByRole('button',{name:'WireMock JSON'}))
 const rules=editor.getByLabelText('Mock response rules JSON') as HTMLTextAreaElement
 expect(JSON.parse(rules.value)).toHaveLength(2)
 expect(editor.getByLabelText('MCP tool definition').textContent).not.toContain('response-template')
 const valid=rules.value
 fireEvent.change(rules,{target:{value:'['}})
 fireEvent.click(editor.getByRole('button',{name:'WireMock JSON'}))
 expect(rules.value).toBe('[')
 expect(editor.getByRole('button',{name:'Apply operation'})).toHaveProperty('disabled',true)
 expect(JSON.parse(screen.getByTestId('draft').textContent!).operations).toHaveLength(0)
 fireEvent.change(rules,{target:{value:valid}})
 fireEvent.click(editor.getByRole('button',{name:'Apply operation'}))
 await waitFor(()=>expect(screen.queryByRole('dialog',{name:'Edit REST operation'})).toBeNull())
 expect(JSON.parse(screen.getByTestId('draft').textContent!).operations[0].mockRules[1].response.transformers).toEqual(['response-template'])
})

import MockRulesEditor from './MockRulesEditor'
import type {MockRule} from '../api'
function Rules({initial}:{initial:MockRule[]}){const [rules,setRules]=useState(initial),[invalid,setInvalid]=useState(false);return <><MockRulesEditor rules={rules} onChange={setRules} onInvalidChange={setInvalid}/><output data-testid="rules">{JSON.stringify(rules)}</output><output data-testid="invalid">{String(invalid)}</output></>}
const initial:MockRule[]=[{name:'fallback',priority:10,response:{status:200,jsonBody:{ok:true}}},{name:'special',priority:1,request:{bodyPatterns:[{matchesJsonPath:{expression:'$.order.id',equalTo:'A'}},{equalToJson:{flag:true},ignoreExtraElements:true}]},response:{status:201,jsonBody:{id:1}}}]
test('visual edits preserve advanced matchers and synchronize both views',()=>{
 render(<Rules initial={initial}/>)
 fireEvent.click(screen.getByRole('button',{name:/special/}))
 expect(screen.getByText('Advanced condition · preserved in WireMock JSON')).toBeTruthy()
 fireEvent.change(screen.getByLabelText('Response status'),{target:{value:'202'}})
 fireEvent.change(screen.getByLabelText('Condition field'),{target:{value:'type'}})
 fireEvent.change(screen.getByLabelText('Condition value'),{target:{value:'1'}})
 expect(screen.getByTestId('invalid').textContent).toBe('true')
 fireEvent.click(screen.getByRole('button',{name:'Add condition'}))
 expect(screen.getByTestId('invalid').textContent).toBe('false')
 fireEvent.click(screen.getByRole('button',{name:'WireMock JSON'}))
 const rules=JSON.parse((screen.getByLabelText('Mock response rules JSON') as HTMLTextAreaElement).value)
 expect(rules[1].request.bodyPatterns).toEqual(initial[1].request!.bodyPatterns)
 expect(rules[1].request.queryParameters.type).toEqual({equalTo:'1'})
 expect(rules[1].response.status).toBe(202)
 fireEvent.click(screen.getByRole('button',{name:'Visual editor'}))
 expect(screen.getByRole('button',{name:/special/})).toBeTruthy()
})
test('invalid body and header drafts block navigation until repaired and retain input',()=>{
 render(<Rules initial={initial}/>)
 fireEvent.change(screen.getByLabelText('Response body'),{target:{value:'['}})
 expect(screen.getByTestId('invalid').textContent).toBe('true')
 expect(screen.getByRole('button',{name:'WireMock JSON'})).toHaveProperty('disabled',true)
 expect(screen.getByRole('button',{name:/special/})).toHaveProperty('disabled',true)
 fireEvent.change(screen.getByLabelText('Response body'),{target:{value:'{"ok":false}'}})
 fireEvent.click(screen.getByRole('button',{name:'Add response header'}))
 expect(screen.getByTestId('invalid').textContent).toBe('true')
 fireEvent.change(screen.getByLabelText('Response header 1 name'),{target:{value:'X-Mock'}})
 fireEvent.change(screen.getByLabelText('Response header 1 value'),{target:{value:'yes'}})
 expect(screen.getByTestId('invalid').textContent).toBe('false')
 const rules=JSON.parse(screen.getByTestId('rules').textContent!)
 expect(rules[0].response).toMatchObject({jsonBody:{ok:false},headers:{'X-Mock':'yes'}})
})
test('quick mock generates nested request schema and opens a review without applying',async()=>{
 render(<Draft/>);fireEvent.click(screen.getByRole('button',{name:'Quick mock operation'}))
 fireEvent.change(screen.getByLabelText('Quick mock tool name'),{target:{value:'createOrder'}})
 fireEvent.change(screen.getByLabelText('Quick mock method'),{target:{value:'POST'}})
 fireEvent.change(screen.getByLabelText('Quick mock path'),{target:{value:'/orders/{id}'}})
 fireEvent.change(screen.getByLabelText('Quick mock request example'),{target:{value:'{"order":{"items":[{"productId":"SKU","quantity":2}]}}'}})
 fireEvent.change(screen.getByLabelText('Quick mock response example'),{target:{value:'{"id":1}'}})
 fireEvent.click(screen.getByRole('button',{name:'Review operation'}))
 expect(JSON.parse(screen.getByTestId('draft').textContent!).operations).toHaveLength(0)
 const editor=within(screen.getByRole('dialog',{name:'Edit REST operation'}))
 expect(editor.getByLabelText('Tool name')).toHaveProperty('value','createOrder')
 fireEvent.click(editor.getByRole('button',{name:'Apply operation'}))
 await waitFor(()=>expect(JSON.parse(screen.getByTestId('draft').textContent!).operations).toHaveLength(1))
 const op=JSON.parse(screen.getByTestId('draft').textContent!).operations[0]
 expect(op.bodySchema.properties.order.properties.items.items.properties.quantity.type).toBe('integer')
 expect(op.parameters[0]).toMatchObject({name:'id',in:'path',required:true})
 expect(op.responseSchema).toBeUndefined()
 expect(op.mockRules[0].response.jsonBody).toEqual({id:1})
})

test('reordering and duplicating keep the selected response body and advanced conditions',()=>{
 render(<Rules initial={initial}/>)
 fireEvent.click(screen.getByRole('button',{name:/special/}))
 fireEvent.click(screen.getByRole('button',{name:'Move later'}))
 expect(screen.getByLabelText('Response name')).toHaveProperty('value','special')
 expect(JSON.parse((screen.getByLabelText('Response body') as HTMLTextAreaElement).value)).toEqual({id:1})
 fireEvent.click(screen.getByRole('button',{name:'Duplicate response'}))
 const rules=JSON.parse(screen.getByTestId('rules').textContent!)
 expect(rules).toHaveLength(3)
 expect(rules[2].request).toEqual(initial[1].request)
 expect(screen.getByLabelText('Response name')).toHaveProperty('value','Copy 1')
 fireEvent.change(screen.getByLabelText('Response name'),{target:{value:''}})
 expect(screen.getByTestId('invalid').textContent).toBe('true')
 fireEvent.change(screen.getByLabelText('Response name'),{target:{value:'copy-fixed'}})
 expect(screen.getByTestId('invalid').textContent).toBe('false')
})
