import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import Button from './Button'
afterEach(cleanup)
test('pending actions disable native activation, expose busy state and retain an accessible label',()=>{
 const click=vi.fn()
 const {rerender}=render(<Button variant="primary" onClick={click}>Save</Button>)
 const button=screen.getByRole('button',{name:'Save'})
 expect(button.getAttribute('type')).toBe('button');fireEvent.click(button);expect(click).toHaveBeenCalledOnce()
 rerender(<Button variant="primary" loading loadingLabel="Saving…" onClick={click}>Save</Button>)
 const pending=screen.getByRole('button',{name:'Saving…'}) as HTMLButtonElement
 expect(pending.disabled).toBe(true);expect(pending.getAttribute('aria-busy')).toBe('true')
 fireEvent.click(pending);expect(click).toHaveBeenCalledOnce()
 expect(pending.querySelector('svg')?.getAttribute('aria-hidden')).toBe('true')
})
test('icon-only danger buttons preserve explicit names and native disabled fieldsets',()=>{
 const {container}=render(<fieldset disabled><Button variant="danger-outline" iconOnly aria-label="Delete key"><svg/></Button></fieldset>)
 const button=screen.getByRole('button',{name:'Delete key'})
 expect(button.matches(':disabled')).toBe(true)
 expect(button.classList.contains('action-icon')).toBe(true)
 expect(container.querySelector('.action-danger-outline')).toBe(button)
})
test('forms can explicitly opt into submit while secondary actions default to non-submit',()=>{
 render(<><Button type="submit">Create</Button><Button>Cancel</Button></>)
 expect(screen.getByRole('button',{name:'Create'}).getAttribute('type')).toBe('submit')
 expect(screen.getByRole('button',{name:'Cancel'}).getAttribute('type')).toBe('button')
})
