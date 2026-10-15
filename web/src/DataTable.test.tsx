import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { DataTable, LoadMore, Pagination } from './DataTable'
import { sortRows, useTableSort } from './table-sorting'
afterEach(cleanup)

test('sorting is stable, natural and keeps missing values last without mutating source rows',()=>{
 const rows=[{name:'Tool 10',count:2},{name:'tool 2',count:2},{name:'Tool 1',count:null}]
 const getters={Name:(r:typeof rows[number])=>r.name,Count:(r:typeof rows[number])=>r.count}
 expect(sortRows(rows,{key:'Name',direction:'asc'},getters).map(r=>r.name)).toEqual(['Tool 1','tool 2','Tool 10'])
 expect(sortRows(rows,{key:'Count',direction:'desc'},getters).map(r=>r.name)).toEqual(['Tool 10','tool 2','Tool 1'])
 expect(sortRows(rows,{key:'Count',direction:'asc'},getters).map(r=>r.name)).toEqual(['Tool 10','tool 2','Tool 1'])
 expect(rows.map(r=>r.name)).toEqual(['Tool 10','tool 2','Tool 1'])
})
function Table(){const sorting=useTableSort([{name:'Tool 10'},{name:'Tool 2'}],{Name:r=>r.name});return <DataTable caption="Tools" columns={['Name','Actions']} sorting={sorting} actions={1}>{sorting.rows.map(r=><tr key={r.name}><td>{r.name}</td><td><button>Inspect {r.name}</button></td></tr>)}</DataTable>}
test('sort headers expose direction, preserve row actions and can reset original order',()=>{
 render(<Table/>);const table=screen.getByRole('table',{name:'Tools'})
 const header=within(table).getByRole('columnheader',{name:'Name'})
 fireEvent.click(within(header).getByRole('button',{name:'Name'}))
 expect(header.getAttribute('aria-sort')).toBe('ascending')
 expect(within(table).getAllByRole('row')[1].textContent).toBe('Tool 2Inspect Tool 2')
 fireEvent.click(within(header).getByRole('button',{name:'Name'}))
 expect(header.getAttribute('aria-sort')).toBe('descending')
 fireEvent.click(screen.getByRole('button',{name:'Reset sorting'}))
 expect(header.getAttribute('aria-sort')).toBe('none')
 expect(within(table).getAllByRole('row')[1].textContent).toBe('Tool 10Inspect Tool 10')
})
test('numbered pagination clamps bounds and marks the selected page',()=>{
 const onPage=vi.fn();render(<Pagination page={1} pageSize={24} total={60} onPage={onPage}/>)
 expect(screen.getByRole('button',{name:'Page 2'}).getAttribute('aria-current')).toBe('page')
 fireEvent.click(screen.getByRole('button',{name:'Next'}));expect(onPage).toHaveBeenCalledWith(2)
 fireEvent.click(screen.getByRole('button',{name:'Previous'}));expect(onPage).toHaveBeenCalledWith(0)
})
test('cursor pagination shows loaded counts without inventing a total and disables concurrent loading',()=>{
 const onLoad=vi.fn();const view=render(<LoadMore shown={50} hasMore onLoad={onLoad} label="Load more" busy/>)
 expect(screen.getByText(/records loaded/).textContent).not.toContain('of')
 fireEvent.click(screen.getByRole('button',{name:'Load more'}));expect(onLoad).not.toHaveBeenCalled()
 view.rerender(<LoadMore shown={50} hasMore onLoad={onLoad} label="Load more"/>)
 fireEvent.click(screen.getByRole('button',{name:'Load more'}));expect(onLoad).toHaveBeenCalledTimes(1)
})
