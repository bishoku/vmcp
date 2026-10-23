import {useState} from 'react'
import {cleanup,fireEvent,screen,waitFor,within} from '@testing-library/react'
import {afterEach,beforeEach,expect,test,vi} from 'vitest'
import {render} from '../test-utils'
import SchemaEditor from './SchemaEditor'
import type {Schema} from './schema-editor'
beforeEach(()=>{HTMLDialogElement.prototype.showModal=function(){this.open=true};HTMLDialogElement.prototype.close=function(){this.open=false}})
afterEach(()=>{cleanup();vi.restoreAllMocks()})
function Draft({initial}:{initial?:Schema}){const[value,setValue]=useState<Schema|undefined>(initial);return <><SchemaEditor label="Response" value={value} onChange={setValue}/><output data-testid="schema">{JSON.stringify(value)}</output></>}
const open=()=>fireEvent.click(screen.getByRole('button',{name:'Edit response schema'}))
async function generate(example:string){fireEvent.change(screen.getByLabelText('Response example JSON'),{target:{value:example}});fireEvent.click(screen.getByRole('button',{name:'Generate response schema'}));await screen.findByRole('button',{name:'Expand all fields'})}
test('nested schema edits stay in modal until Apply and cancel restores original',async()=>{
 render(<Draft/>);open();await generate('{"rows":[{"id":1}]}')
 fireEvent.click(screen.getByRole('button',{name:'Expand all fields'}))
 const desc=await screen.findByLabelText('Response.rows[].id description');fireEvent.change(desc,{target:{value:'Record identifier'}});fireEvent.click(screen.getByLabelText('Response.rows[].id required'))
 expect(screen.getByTestId('schema').textContent).toBe('')
 fireEvent.click(screen.getByRole('button',{name:'Apply schema'}));await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull())
 expect(JSON.parse(screen.getByTestId('schema').textContent!).properties.rows.items.required).toEqual(['id'])
 open();fireEvent.click(screen.getByRole('button',{name:'Fields'}));fireEvent.click(screen.getByRole('button',{name:'Expand all fields'}));fireEvent.change(await screen.findByLabelText('Response.rows[].id description'),{target:{value:'Discard'}})
 vi.spyOn(window,'confirm').mockReturnValue(true);fireEvent.click(screen.getByRole('button',{name:'Cancel schema changes'}));await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());expect(JSON.parse(screen.getByTestId('schema').textContent!).properties.rows.items.properties.id.description).toBe('Record identifier')
})
test('merged example mismatches are visible; nullable field fixes response validation',async()=>{
 render(<Draft initial={{type:'object',properties:{id:{type:'integer'}}}}/>);open();await generate('{"id":null}')
 await screen.findByText('/id: must be integer')
 fireEvent.change(screen.getByLabelText('Find schema fields'),{target:{value:'id'}})
 fireEvent.click(await screen.findByLabelText('Response.id nullable'))
 fireEvent.click(screen.getByRole('button',{name:'Apply schema'}));await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull())
 expect(JSON.parse(screen.getByTestId('schema').textContent!).properties.id.type).toEqual(['integer','null'])
})
test('invalid examples and oversized schemas cannot replace the draft',async()=>{
 render(<Draft initial={{type:'object'}}/>);open();fireEvent.change(screen.getByLabelText('Response example JSON'),{target:{value:'{'}});fireEvent.click(screen.getByRole('button',{name:'Generate response schema'}));await screen.findByText('Enter valid JSON.')
 fireEvent.click(screen.getByRole('button',{name:'Schema JSON'}));fireEvent.change(screen.getByRole('textbox',{name:'Response schema'}),{target:{value:JSON.stringify({description:'x'.repeat(66000)})}})
 expect(screen.getByRole('button',{name:'Apply schema'})).toHaveProperty('disabled',true);expect(JSON.parse(screen.getByTestId('schema').textContent!)).toEqual({type:'object'})
})
test('large schemas start collapsed and search reveals nested fields',async()=>{
 render(<Draft initial={{type:'object',properties:Object.fromEntries(Array.from({length:150},(_,i)=>[`item${i}`,{type:'object',properties:{amount:{type:'number'}}}]))}}/>);open();fireEvent.click(screen.getByRole('button',{name:'Fields'}))
 expect(screen.queryByLabelText('Response.item149.amount type')).toBeNull()
 fireEvent.change(screen.getByLabelText('Find schema fields'),{target:{value:'item149'}})
 expect(await screen.findByLabelText('Response.item149.amount type')).toHaveProperty('value','number')
 expect(screen.queryByLabelText('Response.item0.amount type')).toBeNull()
})
test('complex schemas and constraints remain intact in field mode',async()=>{
 render(<Draft initial={{type:'object',properties:{status:{type:'string',enum:['ok']},data:{anyOf:[{type:'null'},{type:'object'}]}}}}/>);open();fireEvent.click(screen.getByRole('button',{name:'Fields'}));fireEvent.click(screen.getByRole('button',{name:'Expand all fields'}))
 expect(await screen.findByLabelText('Response.status type')).toHaveProperty('disabled',true)
 expect(within(screen.getByRole('dialog')).getByText(/Complex schema preserved/)).toBeTruthy()
})
test('replacement requires confirmation and removes stale imported constraints',async()=>{
 vi.spyOn(window,'confirm').mockReturnValue(true)
 render(<Draft initial={{type:'object',additionalProperties:false,required:['old'],properties:{old:{type:'string'}}}}/>);open()
 fireEvent.change(screen.getByLabelText('Schema generation mode'),{target:{value:'replace'}})
 await generate('{"current":1}')
 expect(window.confirm).toHaveBeenCalled()
 fireEvent.click(screen.getByRole('button',{name:'Apply schema'}));await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull())
 expect(JSON.parse(screen.getByTestId('schema').textContent!)).toEqual({type:'object',properties:{current:{type:'integer'}}})
})
