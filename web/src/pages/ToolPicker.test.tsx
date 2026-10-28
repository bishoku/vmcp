import { fireEvent, render, screen, cleanup, within } from '@testing-library/react'
import { useState } from 'react'
import { afterEach,beforeEach,expect,test,vi } from 'vitest'
import ToolPicker from './ToolPicker'
import type { ToolSelection } from '../api'
import {upstream} from '../connections/fixtures'

beforeEach(()=>vi.stubGlobal('matchMedia',vi.fn(()=>({matches:true,addEventListener:vi.fn(),removeEventListener:vi.fn()}))))
afterEach(()=>{cleanup();vi.unstubAllGlobals()})
const tools=Array.from({length:60},(_,i)=>({name:`tool-${i}`,description:i<3?'Customer lookup':'Other operation'}))
function Picker(){
 const [selected,setSelected]=useState<ToolSelection[]>([{upstream:'alpha',tool:'tool-0'}])
 return <><p data-testid="selected">{selected.length}</p><ToolPicker upstreams={[upstream]} catalog={{alpha:tools}} shared={[{upstream:'shared',ownerTeam:'other',name:'shared-lookup',description:'Customer lookup'},{upstream:'alpha',ownerTeam:'mine',name:'duplicate',description:'Private'}]} selected={selected} disabled={false} loading={false} onAdd={items=>setSelected(previous=>[...previous,...items])}/></>
}
test('starts empty, browses 25 at a time, adds all remaining without readding selections',()=>{
 render(<Picker/>)
 expect(screen.queryByRole('button',{name:/tool-1 /})).toBeNull()
 fireEvent.change(screen.getByLabelText('Tool upstream'),{target:{value:'alpha'}})
 expect(screen.getByRole('button',{name:'tool-24 Other operation'})).toBeTruthy()
 expect(screen.queryByRole('button',{name:'tool-25 Other operation'})).toBeNull()
 fireEvent.click(screen.getByRole('button',{name:'Show 25 more tools'}))
 expect(screen.getByRole('button',{name:'tool-25 Other operation'})).toBeTruthy()
 fireEvent.click(screen.getByRole('button',{name:'Add all 59 remaining tools'}))
 expect(screen.getByTestId('selected').textContent).toBe('60')
 expect((screen.getByRole('button',{name:'Add all 0 remaining tools'}) as HTMLButtonElement).disabled).toBe(true)
})
test('All upstreams starts collapsed; search works without scope and bulk respects matches',()=>{
 render(<Picker/>)
 fireEvent.change(screen.getByLabelText('Tool upstream'),{target:{value:'all'}})
 expect((screen.getByText('Alpha · 60 tools · 1 selected').parentElement as HTMLDetailsElement).open).toBe(false)
 fireEvent.click(screen.getByText('Alpha · 60 tools · 1 selected'))
 expect(screen.getByRole('button',{name:'tool-1 Customer lookup'})).toBeTruthy()
 fireEvent.click(screen.getByRole('button',{name:'Clear picker filters'}))
 fireEvent.change(screen.getByRole('searchbox',{name:'Filter tools'}),{target:{value:'Customer'}})
 fireEvent.click(screen.getByRole('button',{name:'Add 2 matching tools'}))
 expect(screen.getByTestId('selected').textContent).toBe('3')
})
test('Shared tab excludes managed upstream duplicates and supports keyboard switching',()=>{
 render(<Picker/>)
 const shared=screen.getByRole('tab',{name:'Shared'})
 fireEvent.keyDown(screen.getByRole('tab',{name:'Available'}),{key:'ArrowRight'})
 expect(shared.getAttribute('aria-selected')).toBe('true')
 fireEvent.change(screen.getByLabelText('Tool upstream'),{target:{value:'shared'}})
 const panel=screen.getByRole('tabpanel',{name:'Shared'})
 fireEvent.click(within(panel).getByRole('button',{name:'Add all 1 remaining tools'}))
 expect(screen.getByTestId('selected').textContent).toBe('2')
 expect(screen.queryByText('duplicate')).toBeNull()
})
