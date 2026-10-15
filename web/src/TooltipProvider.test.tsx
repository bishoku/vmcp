import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { useState } from 'react'
import TooltipProvider from './TooltipProvider'
import HelpTip from './HelpTip'
import Select from './Select'
afterEach(()=>{cleanup();vi.useRealTimers()})

test('focus hints preserve existing descriptions and Escape dismisses without moving focus',()=>{
 const parentKey=vi.fn()
 render(<TooltipProvider><div onKeyDown={parentKey}><p id="existing-hint">Existing help</p><button data-tooltip="Credential metadata" aria-describedby="existing-hint">Inspect</button></div></TooltipProvider>)
 const trigger=screen.getByRole('button',{name:'Inspect'})
 act(()=>trigger.focus())
 const tooltip=screen.getByRole('tooltip')
 expect(trigger.getAttribute('aria-describedby')?.split(' ')).toEqual(['existing-hint',tooltip.id])
 fireEvent.keyDown(trigger,{key:'Escape'})
 expect(screen.queryByRole('tooltip')).toBeNull();expect(document.activeElement).toBe(trigger)
 expect(trigger.getAttribute('aria-describedby')).toBe('existing-hint');expect(parentKey).not.toHaveBeenCalled()
})
test('pointer hints wait briefly and route-like removal cleans up the visible hint',async()=>{
 vi.useFakeTimers()
 const view=render(<TooltipProvider><button data-tooltip="Rotate this key">Rotate</button></TooltipProvider>)
 fireEvent.pointerOver(screen.getByRole('button',{name:'Rotate'}),{pointerType:'mouse'})
 expect(screen.queryByRole('tooltip')).toBeNull()
 act(()=>vi.advanceTimersByTime(350));expect(screen.getByRole('tooltip').textContent).toContain('Rotate this key')
 view.rerender(<TooltipProvider><p>Another page</p></TooltipProvider>)
 await act(async()=>{})
 expect(screen.queryByRole('tooltip')).toBeNull()
})
test('touch help opens explicitly and an outside pointer dismisses it',()=>{
 render(<TooltipProvider><HelpTip label="About access" content="Use an agent credential."/><button>Outside</button></TooltipProvider>)
 fireEvent.pointerDown(screen.getByRole('button',{name:'About access'}),{pointerType:'touch'})
 expect(screen.getByRole('tooltip').textContent).toContain('agent credential')
 fireEvent.pointerDown(screen.getByRole('button',{name:'Outside'}),{pointerType:'touch'})
 expect(screen.queryByRole('tooltip')).toBeNull()
})
function NativeForm(){const[value,setValue]=useState('read');return <form aria-label="Credential"><label htmlFor="test-scope">Scope</label><Select id="test-scope" name="scope" value={value} required aria-describedby="scope-help" onChange={e=>setValue(e.target.value)}><option value="read">Read</option><option value="admin">Admin</option><option disabled value="unavailable">Unavailable</option></Select><p id="scope-help">Choose access</p></form>}
test('the shared select retains native labels, values, required state and form submission',()=>{
 render(<NativeForm/>);const select=screen.getByRole('combobox',{name:'Scope'}) as HTMLSelectElement
 expect(select.tagName).toBe('SELECT');expect(select.required).toBe(true);expect(select.classList.contains('vmcp-select')).toBe(true)
 fireEvent.change(select,{target:{value:'admin'}})
 expect(select.value).toBe('admin')
 expect(new FormData(screen.getByRole('form',{name:'Credential'}) as HTMLFormElement).get('scope')).toBe('admin')
 expect(select.options[2].disabled).toBe(true);expect(select.getAttribute('aria-describedby')).toBe('scope-help')
})
