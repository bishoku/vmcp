import { useState } from 'react'
import { cleanup, fireEvent, screen, within } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'
import { render } from '../test-utils'
import SchemaEditor from './SchemaEditor'
import type { Schema } from './schema-editor'
afterEach(cleanup)
function Draft({ initial }: { initial?: Schema }) {
  const [value, setValue] = useState<Schema | undefined>(initial)
  return <><SchemaEditor label="Response" value={value} onChange={setValue}/><output data-testid="schema">{JSON.stringify(value)}</output></>
}
test('generates nested fields, edits list items, and toggles required', () => {
  render(<Draft/>)
  fireEvent.change(screen.getByLabelText('Response example JSON'), { target: { value: '{"rows":[{"id":1}]}' } })
  fireEvent.click(screen.getByRole('button', { name: 'Generate response schema' }))
  fireEvent.change(screen.getByLabelText('Response.rows[].id description'), { target: { value: 'Record identifier' } })
  fireEvent.click(screen.getByLabelText('Response.rows[].id required'))
  const value = JSON.parse(screen.getByTestId('schema').textContent!)
  expect(value.properties.rows.items.required).toEqual(['id'])
  expect(value.properties.rows.items.properties.id.description).toBe('Record identifier')
})
test('invalid example does not replace existing schema; complex nodes remain intact', () => {
  const initial = { type: 'object', properties: { status: { type: 'string', enum: ['ok'] }, data: { anyOf: [{ type: 'null' }, { type: 'object' }] } } }
  render(<Draft initial={initial}/>)
  expect(screen.getByLabelText('Response.status type')).toHaveProperty('disabled', true)
  expect(screen.getByText(/Complex schema preserved/)).toBeTruthy()
  fireEvent.change(screen.getByLabelText('Response example JSON'), { target: { value: '{' } })
  fireEvent.click(screen.getByRole('button', { name: 'Generate response schema' }))
  expect(screen.getByRole('alert').textContent).toBe('Enter valid JSON.')
  expect(JSON.parse(screen.getByTestId('schema').textContent!)).toEqual(initial)
  expect(within(screen.getByRole('region', { name: 'Response visual editor' })).getByText('data')).toBeTruthy()
})
