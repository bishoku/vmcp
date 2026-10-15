import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import DateTimeField from './DateTimeField'
afterEach(cleanup)
test('calendar choices stay local until Apply and Cancel keeps the original value',()=>{
 const change=vi.fn()
 render(<DateTimeField name="expiresAt" label="Expires" value="2027-02-03T10:15" onChange={change}/>)
 fireEvent.click(screen.getByRole('button',{name:'Choose expires date and time'}))
 fireEvent.click(screen.getByRole('button',{name:'Thursday, February 4, 2027'}))
 fireEvent.change(screen.getByLabelText('Time'),{target:{value:'14:45'}})
 expect(change).not.toHaveBeenCalled()
 fireEvent.click(screen.getByRole('button',{name:'Cancel'}));expect(change).not.toHaveBeenCalled()
 fireEvent.click(screen.getByRole('button',{name:'Choose expires date and time'}))
 fireEvent.click(screen.getByRole('button',{name:'Thursday, February 4, 2027'}))
 fireEvent.click(screen.getByRole('button',{name:'Apply'}));expect(change).toHaveBeenCalledWith('2027-02-04T10:15')
})
test('clearing expiry is explicit and field errors describe the native input',()=>{
 const change=vi.fn()
 render(<DateTimeField name="expiresAt" label="Expires" value="2027-02-03T10:15" error="Expiry cannot be extended." onChange={change}/>)
 const input=screen.getByLabelText('Expires')
 expect(input.getAttribute('aria-invalid')).toBe('true')
 expect(input.getAttribute('aria-describedby')?.split(' ').map(id=>document.getElementById(id)?.textContent).join(' ')).toContain('Expiry cannot be extended.')
 fireEvent.click(screen.getByRole('button',{name:'Choose expires date and time'}))
 fireEvent.click(screen.getByRole('button',{name:'No expiry'}));expect(change).toHaveBeenCalledWith('')
})
