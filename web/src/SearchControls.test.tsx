import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { useState } from 'react'
import { ActiveFilters, ResultCount, SearchField } from './SearchControls'
afterEach(cleanup)

test('Escape clears a nonempty search and stays focused without dismissing its enclosing view',()=>{
 const dismiss=vi.fn(),change=vi.fn()
 render(<div onKeyDown={dismiss}><SearchField label="Search keys" value="team" onChange={change}/></div>)
 const search=screen.getByRole('searchbox',{name:'Search keys'})
 fireEvent.keyDown(search,{key:'Escape'})
 expect(change).toHaveBeenCalledWith('');expect(document.activeElement).toBe(search);expect(dismiss).not.toHaveBeenCalled()
})
test('removing the final filter and resetting filters restore focus to their scoped search',async()=>{
 function View(){
  const [value,setValue]=useState('team')
  return <section><SearchField label="Search keys" value={value} onChange={setValue}/><ActiveFilters items={[{id:'search',label:'Search',value,onRemove:()=>setValue('')}]} onReset={()=>setValue('')}/></section>
 }
 render(<View/>);fireEvent.click(screen.getByRole('button',{name:'Remove Search filter'}))
 await waitFor(()=>expect(document.activeElement).toBe(screen.getByRole('searchbox')))
 expect(screen.queryByRole('group',{name:'Active filters'})).toBeNull()
 fireEvent.change(screen.getByRole('searchbox'),{target:{value:'another'}})
 fireEvent.click(screen.getByRole('button',{name:'Clear filters'}))
 await waitFor(()=>expect((screen.getByRole('searchbox') as HTMLInputElement).value).toBe(''))
 await waitFor(()=>expect(document.activeElement).toBe(screen.getByRole('searchbox')))
})
test('result counts distinguish loaded records from known totals and name the live status',()=>{
 const {rerender}=render(<ResultCount matched={5} noun="records" loaded/>)
 expect(screen.getByRole('status',{name:'records result count'}).textContent).toBe('5 loaded records')
 rerender(<ResultCount matched={2} total={60} noun="servers" busy/>)
 expect(screen.getByRole('status',{name:'servers result count'}).textContent).toBe('2 of 60 servers · updating…')
})
