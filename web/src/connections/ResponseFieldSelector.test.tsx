import { useState } from 'react'
import { cleanup, fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'
import { render } from '../test-utils'
import { responseSelectionError } from './rest-tool-preview'
import ResponseFieldSelector from './ResponseFieldSelector'
afterEach(cleanup)
const schema = { type: 'object', properties: { id: { type: 'integer' }, profile: { type: 'object' }, secret: { type: 'string' } } }
function Draft(){const[fields,setFields]=useState<string[]>([]);return <><ResponseFieldSelector schema={schema} fields={fields} onChange={setFields}/><output data-testid="fields">{JSON.stringify(fields)}</output></>}
test('explicit filtering includes declared fields and never turns an empty selection into full output',()=>{
 render(<Draft/>);
 fireEvent.click(screen.getByRole('button',{name:'Selected fields only'}))
 fireEvent.click(screen.getByLabelText('Return field secret'))
 fireEvent.click(screen.getByLabelText('Return field profile'))
 expect(screen.getByTestId('fields').textContent).toBe('["id"]')
 expect(screen.getByLabelText('Return field id')).toHaveProperty('disabled',true)
 fireEvent.click(screen.getByRole('button',{name:'Full response'}))
 expect(screen.getByTestId('fields').textContent).toBe('[]')
})
test('schema drift is reported without silently widening the output',()=>{
 expect(responseSelectionError(schema,['deleted'])).toContain('no longer declared')
 expect(responseSelectionError({type:'array'},['id'])).toContain('object response')
 expect(responseSelectionError(schema,['id'])).toBe('')
 render(<ResponseFieldSelector schema={schema} fields={['deleted']} onChange={()=>{}}/>)
 expect(screen.getByRole('alert')).toBeTruthy()
 expect(screen.getByLabelText('Return field deleted')).toHaveProperty('checked',true)
})
