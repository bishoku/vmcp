import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { Checkbox, Switch, RadioGroup } from './SelectionControls'
import { FormContext } from './form-validation'
afterEach(cleanup)

test('checkbox labels keep the name separate from help and preserve native form values and mixed state',()=>{
 const {container,rerender}=render(<form><Checkbox label="Server access" description="Team gateway" name="server" value="demo" defaultChecked indeterminate/></form>)
 const box=screen.getByRole('checkbox',{name:'Server access'}) as HTMLInputElement
 expect(box.indeterminate).toBe(true)
 expect(document.getElementById(box.getAttribute('aria-describedby')!)?.textContent).toBe('Team gateway')
 expect(new FormData(container.querySelector('form')!).get('server')).toBe('demo')
 rerender(<form><Checkbox label="Server access" name="server" value="demo" defaultChecked/></form>)
 expect(box.indeterminate).toBe(false)
 fireEvent.click(screen.getByText('Server access'))
 expect(new FormData(container.querySelector('form')!).has('server')).toBe(false)
})

test('switch API errors are linked and clearing the field keeps the draft update',()=>{
 const clear=vi.fn(),change=vi.fn()
 render(<FormContext.Provider value={{issues:[{field:'enabled',message:'Cannot enable this server yet'}],clear} as never}><Switch label="Enabled" field="enabled" description="Apply on Save" checked={false} onChange={change}/></FormContext.Provider>)
 const toggle=screen.getByRole('switch',{name:'Enabled'})
 expect(toggle.getAttribute('aria-invalid')).toBe('true')
 expect(toggle.getAttribute('aria-describedby')?.split(' ').map(id=>document.getElementById(id)?.textContent)).toEqual(['Apply on Save','Cannot enable this server yet'])
 fireEvent.click(screen.getByText('Enabled'));expect(clear).toHaveBeenCalledWith('enabled');expect(change).toHaveBeenCalledOnce()
})

test('radio group exposes options, disabled choices and shared error descriptions',()=>{
 const change=vi.fn()
 render(<RadioGroup label="View" value="all" onChange={change} options={[{value:'all',label:'All'},{value:'tools',label:'Tools',disabled:true},{value:'resources',label:'Resources'}]} error="Choose a supported view"/>)
 expect((screen.getByRole('radio',{name:'All'}) as HTMLInputElement).checked).toBe(true)
 expect((screen.getByRole('radio',{name:'Tools'}) as HTMLInputElement).disabled).toBe(true)
 const radio=screen.getByRole('radio',{name:'Resources'})
 expect(document.getElementById(radio.getAttribute('aria-describedby')!)?.textContent).toBe('Choose a supported view')
 fireEvent.click(radio);expect(change).toHaveBeenCalledWith('resources')
})
