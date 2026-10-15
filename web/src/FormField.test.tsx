import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'
import { useState } from 'react'
import { ApiError } from './api'
import { FormField, FormErrors, FormScope } from './FormField'
import { useFormValidation, requiredText, rateError, slugError, urlError } from './form-validation'
afterEach(cleanup)
function Form(){
 const validation=useFormValidation(),[value,setValue]=useState('')
 return <FormScope validation={validation}><FormErrors validation={validation}/><FormField name="name" label="Name" required hint="A display name"><input value={value} onChange={e=>setValue(e.target.value)}/></FormField><FormField name="ipAllowlist" label="IP allowlist"><textarea/></FormField><button onClick={()=>validation.check({name:requiredText(value,'Name')})}>Save</button><button onClick={()=>validation.fail(new ApiError(422,'Invalid',undefined,[{field:'/ipAllowlist/0',message:'Invalid CIDR'}]))}>Server error</button><button onClick={()=>validation.fail(new Error('Service unavailable'))}>Network error</button></FormScope>
}
test('required errors link to inputs and focus them; editing removes only the corrected error',async()=>{
 render(<Form/>);const input=screen.getByLabelText('Name')
 expect(input.getAttribute('aria-required')).toBe('true')
 fireEvent.click(screen.getByRole('button',{name:'Save'}))
 await waitFor(()=>expect(document.activeElement).toBe(input))
 expect(input.getAttribute('aria-invalid')).toBe('true')
 const description=input.getAttribute('aria-describedby')!.split(' ').map(id=>document.getElementById(id)!.textContent).join(' ')
 expect(description).toContain('A display name');expect(description).toContain('Name is required.')
 fireEvent.change(input,{target:{value:'Workspace'}})
 expect(input.getAttribute('aria-invalid')).toBeNull();expect(screen.queryByRole('alert')).toBeNull()
})
test('indexed API errors attach to multiline inputs and summary links focus the correct field',async()=>{
 render(<Form/>);fireEvent.click(screen.getByRole('button',{name:'Server error'}))
 const input=screen.getByLabelText('IP allowlist')
 await waitFor(()=>expect(document.activeElement).toBe(input))
 expect(input.getAttribute('aria-invalid')).toBe('true')
 fireEvent.click(screen.getByRole('button',{name:'IP allowlist / 1: Invalid CIDR'}))
 expect(document.activeElement).toBe(input)
})
test('unknown request failures keep entered values and focus the error summary',async()=>{
 render(<Form/>);fireEvent.change(screen.getByLabelText('Name'),{target:{value:'Retain me'}})
 fireEvent.click(screen.getByRole('button',{name:'Network error'}))
 await act(async()=>{})
 await waitFor(()=>expect(document.activeElement?.hasAttribute('data-validation-summary')).toBe(true))
 expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('Retain me')
 expect(screen.getByRole('alert').textContent).toContain('Service unavailable')
})
test('local rules accept supported values and reject malformed identifiers, rates and endpoints',()=>{
 expect(slugError('team-mcp')).toBeUndefined();expect(slugError('-team')).toBeTruthy()
 expect(rateError('')).toBeUndefined();expect(rateError('1000000')).toBeUndefined();expect(rateError('0')).toBeTruthy();expect(rateError('1.5')).toBeTruthy()
 expect(urlError('https://mcp.example/path?tenant=a')).toBeUndefined();expect(urlError('https://user:pass@mcp.example')).toBeTruthy();expect(urlError('file:///tmp/mcp')).toBeTruthy()
})
