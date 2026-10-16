import Button from './Button'
import Select from './Select'
import { Children, cloneElement, isValidElement, useId, type ReactNode } from 'react'
import { ArrowDown, ArrowUp, ArrowUpDown, ChevronDown, ChevronLeft, ChevronRight, LoaderCircle } from 'lucide-react'
import type { TableSorting } from './table-sorting'
import { btn, input } from './styles'

export function SortControl({ sorting, label, className = '' }: { sorting: TableSorting; label: string; className?: string }) {
  const id = useId()
  return <label htmlFor={id} className={`flex min-w-0 flex-wrap items-center gap-2 text-xs font-medium text-ink-muted ${className}`}><ArrowUpDown aria-hidden="true" className="icon-inline"/>Sort<Select id={id} aria-label={label} className={`${input} w-auto min-w-0 flex-1 py-1.5 text-xs sm:flex-none`} value={sorting.sort ? `${sorting.sort.key}|${sorting.sort.direction}` : ''} onChange={event => sorting.select(event.target.value)}><option value="">Default order</option>{sorting.keys.flatMap(key => [<option key={`${key}-asc`} value={`${key}|asc`}>{key} · Ascending</option>, <option key={`${key}-desc`} value={`${key}|desc`}>{key} · Descending</option>])}</Select></label>
}

export function DataTable({ caption, columns, sorting, children, primary = 0, actions, minWidth = 720, sortScope }: { caption: string; columns: string[]; sorting?: TableSorting; children: ReactNode; primary?: number; actions?: number; minWidth?: number; sortScope?: string }) {
  const rows = Children.toArray(children).map(row => {
    if (!isValidElement<{ children: ReactNode; className?: string }>(row)) return row
    return cloneElement(row, { ...{ role: 'row' }, className: `data-row ${row.props.className ?? ''}`, children: Children.toArray(row.props.children).map((cell, index) => isValidElement<{ className?: string }>(cell) ? cloneElement(cell, { ...{ role: 'cell', 'data-label': columns[index], 'data-primary': index === primary || undefined, 'data-actions': index === actions || undefined }, className: `data-cell ${cell.props.className ?? ''}` }) : cell) })
  })
  return <div className="min-w-0">
    {sorting && <div className={`flex flex-wrap items-center justify-between gap-3 border-b border-line bg-surface px-4 py-3 ${sortScope || sorting.sort ? '' : 'sm:hidden'}`}><SortControl sorting={sorting} label={`Sort ${caption}`} className="sm:hidden"/><div className="flex flex-wrap items-center gap-x-3 gap-y-1">{sorting.sort&&<p className="hidden text-xs font-medium text-accent-700 sm:block">{sorting.sort.key} · {sorting.sort.direction==='asc'?'Ascending':'Descending'}</p>}{sortScope && <p className="text-xs leading-relaxed text-ink-muted">{sortScope}</p>}</div>{sorting.sort&&<button type="button" className="text-xs font-medium text-accent-700 hover:underline" onClick={()=>sorting.select('')}>Reset sorting</button>}</div>}
    <div role="region" aria-label={`${caption} table`} tabIndex={0} className="data-table-scroll relative overflow-x-auto focus-visible:outline-offset-[-2px]">
      <table role="table" className="data-table w-full text-left text-sm" style={{ minWidth }}><caption className="sr-only">{caption}</caption><thead role="rowgroup" className="bg-canvas/80"><tr role="row">{columns.map(column => {
        const sortable = sorting?.keys.includes(column)
        const direction = sorting?.sort?.key === column ? sorting.sort.direction : null
        const Icon = direction === 'asc' ? ArrowUp : direction === 'desc' ? ArrowDown : ArrowUpDown
        return <th role="columnheader" scope="col" key={column} aria-sort={sortable ? direction === 'asc' ? 'ascending' : direction === 'desc' ? 'descending' : 'none' : undefined} className="px-4 py-3 text-xs font-semibold text-ink-muted">{sortable ? <button type="button" className={`inline-flex items-center gap-2 rounded-md py-1 text-left transition-colors hover:text-accent-700 ${direction?'text-accent-700':''}`} data-tooltip={`Sort ${column} ${direction==='asc'?'descending':'ascending'}`} onClick={() => sorting!.toggle(column)}>{column}<Icon aria-hidden="true" className={`icon-inline shrink-0 ${direction?'':'opacity-50'}`}/></button> : column}</th>
      })}</tr></thead><tbody role="rowgroup">{rows}</tbody></table>
    </div>
    {sorting?.sort && <p role="status" className="sr-only">{caption} sorted by {sorting.sort.key}, {sorting.sort.direction === 'asc' ? 'ascending' : 'descending'}{sortScope ? '. Sorting applies to loaded records only.' : ''}.</p>}
  </div>
}

export function RowActions({ label, children }: { label: string; children: ReactNode }) {
  return <div role="group" aria-label={label} className="row-actions flex flex-wrap items-center gap-2">{Children.toArray(children).map(child => {
    if (!isValidElement<{ children: ReactNode; className?: string; 'aria-label'?: string }>(child)) return child
    const name = child.props['aria-label']
    const iconOnly = name && isValidElement(child.props.children)
    return cloneElement(child, { className: `${child.props.className ?? ''} min-h-9`, children: <>{child.props.children}{iconOnly && <span className="sm:hidden">{name}</span>}</> })
  })}</div>
}

export function LoadMore({ shown, total, hasMore = total != null && shown < total, onLoad, label, busy = false, noun = 'records' }: { shown: number; total?: number; hasMore?: boolean; onLoad: () => void; label: string; busy?: boolean; noun?: string }) {
  return <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line bg-canvas/30 panel-footer"><p className="text-xs leading-relaxed text-ink-muted"><span className="font-medium tabular-nums text-ink">{shown}</span>{total != null ? <> of <span className="font-medium tabular-nums text-ink">{total}</span> {noun} shown</> : <> {noun} loaded{hasMore ? ' · more available' : ''}</>}</p>{hasMore && <Button loading={busy} type="button" className={`${btn} min-h-9 text-xs`} disabled={busy} onClick={onLoad}>{busy ? <LoaderCircle aria-hidden="true" className="icon-inline motion-spin"/> : <ChevronDown aria-hidden="true" className="icon-inline"/>}{label}</Button>}</div>
}

export function Pagination({ page, pageSize, total, onPage, label = 'Results pagination' }: { page: number; pageSize: number; total: number; onPage: (page: number) => void; label?: string }) {
  const pages = Math.max(1, Math.ceil(total / pageSize))
  const current = Math.max(0, Math.min(page, pages - 1))
  const visible = Array.from({ length: pages }, (_, i) => i).filter(i => i === 0 || i === pages - 1 || Math.abs(i - current) <= 1)
  return <nav aria-label={label} className="flex flex-wrap items-center justify-between gap-3 surface-card px-4 py-3 sm:px-5"><p className="text-xs text-ink-muted"><span className="font-medium tabular-nums text-ink">{total ? current * pageSize + 1 : 0}–{Math.min((current + 1) * pageSize, total)}</span> of {total} · Page {current + 1} of {pages}</p><div className="flex flex-wrap items-center gap-1.5"><button className={`${btn} min-h-9 text-xs`} disabled={current === 0} onClick={() => onPage(current - 1)}><ChevronLeft aria-hidden="true" className="icon-inline"/>Previous</button>{visible.map((value, index) => <span key={value} className="hidden items-center gap-1.5 sm:inline-flex">{index > 0 && value - visible[index - 1] > 1 && <span aria-hidden="true" className="px-1 text-xs text-ink-muted">…</span>}<button type="button" aria-label={`Page ${value + 1}`} aria-current={value === current ? 'page' : undefined} className={`${btn} size-9 p-0 text-xs aria-[current=page]:border-accent-100 aria-[current=page]:bg-accent-50 aria-[current=page]:text-accent-700`} onClick={() => onPage(value)}>{value + 1}</button></span>)}<button className={`${btn} min-h-9 text-xs`} disabled={current + 1 >= pages} onClick={() => onPage(current + 1)}>Next<ChevronRight aria-hidden="true" className="icon-inline"/></button></div></nav>
}
