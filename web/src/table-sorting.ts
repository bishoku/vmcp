import { useState } from 'react'

export type SortValue = string | number | null | undefined
export interface SortState { key: string; direction: 'asc' | 'desc' }
export interface TableSorting {
  sort: SortState | null
  keys: string[]
  toggle: (key: string) => void
  select: (value: string) => void
}
const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })
export function sortRows<T>(rows: T[], sort: SortState | null, getters: Record<string, (row: T) => SortValue>) {
  const getter = sort && getters[sort.key]
  if (!sort || !getter) return rows
  return rows.map((row, index) => ({ row, index, value: getter(row) })).sort((a, b) => {
    // Missing dates/counts stay last in either direction; ties keep the received order.
    if (a.value == null || b.value == null) return a.value == null && b.value == null ? a.index - b.index : a.value == null ? 1 : -1
    const comparison = typeof a.value === 'number' && typeof b.value === 'number' ? a.value - b.value : collator.compare(String(a.value), String(b.value))
    return comparison * (sort.direction === 'asc' ? 1 : -1) || a.index - b.index
  }).map(item => item.row)
}
export function useTableSort<T>(rows: T[], getters: Record<string, (row: T) => SortValue>) {
  const [sort, setSort] = useState<SortState | null>(null)
  return {
    rows: sortRows(rows, sort, getters), sort, keys: Object.keys(getters),
    toggle: (key: string) => setSort(current => ({ key, direction: current?.key === key && current.direction === 'asc' ? 'desc' : 'asc' })),
    select: (value: string) => { const [key, direction] = value.split('|'); setSort(value ? { key, direction: direction === 'desc' ? 'desc' : 'asc' } : null) },
  }
}
