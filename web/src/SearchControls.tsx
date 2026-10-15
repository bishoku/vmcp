import { useId, useRef, type ReactNode } from 'react'
import { Search, SlidersHorizontal, X } from 'lucide-react'
import { input } from './styles'

export function SearchField({ label, value, onChange, placeholder, visibleLabel = false, className = '', disabled = false }: { label: string; value: string; onChange: (value: string) => void; placeholder?: string; visibleLabel?: boolean; className?: string; disabled?: boolean }) {
  const id = useId(), ref = useRef<HTMLInputElement>(null)
  const clear = () => { onChange(''); ref.current?.focus() }
  return <div className={`min-w-0 ${className}`}><label htmlFor={id} className={visibleLabel ? 'mb-2 block text-xs font-medium text-ink' : 'sr-only'}>{label}</label><div className="search-control relative">
    <Search aria-hidden="true" className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-ink-muted"/>
    <input ref={ref} id={id} type="search" disabled={disabled} autoComplete="off" spellCheck={false} enterKeyHint="search" className={`${input} search-input`} value={value} placeholder={placeholder} onChange={event => onChange(event.target.value)} onKeyDown={event => { if (event.key === 'Escape' && value) { event.preventDefault(); event.stopPropagation(); clear() } }}/>
    {value && <button type="button" disabled={disabled} className="search-clear action-button action-ghost action-icon" aria-label={`Clear ${label.replace(/^(Search|Filter|Find)\s+/i, '').toLowerCase()} search`} onClick={clear}><X aria-hidden="true" className="size-4"/></button>}
  </div></div>
}

export interface ActiveFilter { id: string; label: string; value: string; onRemove: () => void }
export function ResultCount({ matched, total, noun = 'results', busy = false, note, loaded = false }: { matched: number; total?: number; noun?: string; busy?: boolean; note?: string; loaded?: boolean }) {
  return <p role="status" aria-label={`${noun} result count`} aria-live="polite" aria-atomic="true" className="result-count"><span className="result-count-number">{matched}</span>{total !== undefined ? <> of <span className="font-medium tabular-nums text-ink">{total}</span></> : null} {loaded ? 'loaded ' : ''}{noun}{busy ? ' · updating…' : ''}{note ? ` · ${note}` : ''}</p>
}
export function ActiveFilters({ items, onReset, resetLabel = 'Clear filters', label = 'Active filters', children, className = '' }: { items: ActiveFilter[]; onReset?: () => void; resetLabel?: string; label?: string; children?: ReactNode; className?: string }) {
  const active = items.filter(item => item.value !== '')
  if (!active.length && !children) return null
  return <div className={`filter-summary ${className}`}>
    {active.length > 0 && <div role="group" aria-label={label} className="flex min-w-0 flex-1 flex-wrap items-center gap-2"><span className="inline-flex items-center gap-1.5 text-[11px] font-medium text-ink-muted"><SlidersHorizontal aria-hidden="true" className="size-3.5"/>{active.length} active</span>{active.map(item => <button key={item.id} type="button" className="filter-chip" data-tooltip={`${item.label}: ${item.value}`} aria-description={`${item.label}: ${item.value}`} aria-label={`Remove ${item.label} filter`} onClick={event => {
      // A removed chip must not strand keyboard focus on the document body.
      const group = event.currentTarget.closest('[role="group"]')
      const siblings = Array.from(group?.querySelectorAll<HTMLButtonElement>('button') ?? [])
      const next = siblings[siblings.indexOf(event.currentTarget) + 1] ?? siblings[siblings.indexOf(event.currentTarget) - 1]
      const reset = event.currentTarget.closest('.filter-summary')?.querySelector<HTMLButtonElement>('[data-filter-reset]')
      const control = group?.closest('section')?.querySelector<HTMLInputElement>('input[type="search"], input, select')
      item.onRemove()
      requestAnimationFrame(() => { if (next?.isConnected) next.focus(); else if (reset?.isConnected) reset.focus(); else control?.focus() })
    }}><span className="min-w-0 break-words">{item.label}: {item.value}</span><X aria-hidden="true" className="size-3.5 shrink-0"/></button>)}</div>}
    <div className="flex max-w-full flex-wrap items-center gap-3">{children}{active.length > 0 && onReset && <button type="button" data-filter-reset className="filter-reset" onClick={event => { const control = event.currentTarget.closest('section')?.querySelector<HTMLInputElement>('input[type="search"], input, select'); onReset(); requestAnimationFrame(() => control?.focus()) }}><X aria-hidden="true" className="size-3.5"/>{resetLabel}</button>}</div>
  </div>
}
