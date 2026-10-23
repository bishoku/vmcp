import { useState } from 'react'
import { cleanup, fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'
import { render } from '../test-utils'
import { responseSelectionError } from './rest-tool-preview'
import ResponseFieldSelector from './ResponseFieldSelector'
afterEach(cleanup)
const schema = { type: 'object', properties: { id: { type: 'integer' }, profile: { type: 'object' }, secret: { type: 'string' } } }
function Draft(){const[fields,setFields]=useState<string[][]>([]);return <><ResponseFieldSelector schema={schema} fields={fields} onChange={setFields}/><output data-testid="fields">{JSON.stringify(fields)}</output></>}
test('explicit filtering includes declared fields and never turns an empty selection into full output',()=>{
 render(<Draft/>);
 fireEvent.click(screen.getByRole('button',{name:'Selected fields only'}))
 fireEvent.click(screen.getByLabelText('Return field secret'))
 fireEvent.click(screen.getByLabelText('Return field profile'))
 expect(screen.getByTestId('fields').textContent).toBe('[["id"]]')
 expect(screen.getByLabelText('Return field id')).toHaveProperty('disabled',true)
 fireEvent.click(screen.getByRole('button',{name:'Full response'}))
 expect(screen.getByTestId('fields').textContent).toBe('[]')
})
test('schema drift is reported without silently widening the output',()=>{
 expect(responseSelectionError(schema,['deleted'])).toContain('no longer declared')
 expect(responseSelectionError({type:'array'},['id'])).toContain('object response')
 expect(responseSelectionError(schema,['id'])).toBe('')
 render(<ResponseFieldSelector schema={schema} fields={[["deleted"]]} onChange={()=>{}}/>)
 expect(screen.getByRole('alert')).toBeTruthy()
 expect(screen.getByText('Missing from schema: deleted')).toBeTruthy()
})

test('nested array selections narrow a parent and keep the final path explicit',()=>{
 const nested={type:'object',properties:{order:{type:'object',properties:{id:{type:'integer'},items:{type:'array',items:{type:'object',properties:{productId:{type:'string'},quantity:{type:'integer'},cost:{type:'number'}}}}}}}}
 function Nested(){const [fields,setFields]=useState<string[][]>([['order']]);return <><ResponseFieldSelector schema={nested} fields={fields} onChange={setFields}/><output data-testid="paths">{JSON.stringify(fields)}</output></>}
 render(<Nested/>);
 fireEvent.change(screen.getByLabelText('Find response fields'),{target:{value:'items'}})
 fireEvent.click(screen.getByLabelText('Return field order.items[].cost'))
 expect(JSON.parse(screen.getByTestId('paths').textContent!)).toEqual([['order','id'],['order','items','productId'],['order','items','quantity']])
 expect(screen.getByLabelText('Return field order')).toHaveProperty('indeterminate',true)
 fireEvent.click(screen.getByLabelText('Return field order.items[].quantity'))
 fireEvent.change(screen.getByLabelText('Find response fields'),{target:{value:'order'}})
 fireEvent.click(screen.getByLabelText('Return field order.id'))
 expect(screen.getByLabelText('Return field order.items[].productId')).toHaveProperty('disabled',true)
})
test('root arrays expose item properties and preserve nested paths',()=>{
 const schema={type:'array',items:{type:'array',items:{type:'object',properties:{id:{type:'integer'},secret:{type:'string'}}}}}
 const changed:string[][][]=[]
 render(<ResponseFieldSelector schema={schema} fields={[['id'],['secret']]} onChange={paths=>changed.push(paths)}/>);
 fireEvent.click(screen.getByLabelText('Return field [][].secret'))
 expect(changed).toEqual([[['id']]])
})
