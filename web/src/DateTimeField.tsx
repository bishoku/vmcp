import { useContext, useId, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react'
import { CalendarDays, ChevronLeft, ChevronRight, Clock3, X } from 'lucide-react'
import { FormField } from './FormField'
import { FormContext } from './form-validation'
import { localDateTime, parseLocalDateTime } from './date-time'
import { btn, input, primary } from './styles'

const dayValue = (date: Date) => `${String(date.getFullYear()).padStart(4, '0')}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
const today = () => localDateTime(new Date().toISOString())

function positionCalendar(node: HTMLElement, control: HTMLElement) {
  const anchor = control.getBoundingClientRect(), bounds = node.getBoundingClientRect()
  node.style.left = `${Math.max(12, Math.min(anchor.left, innerWidth - bounds.width - 12))}px`
  const top = anchor.bottom + 8 + bounds.height < innerHeight - 12 ? anchor.bottom + 8 : anchor.top - bounds.height - 8
  node.style.top = `${Math.max(12, Math.min(top, innerHeight - bounds.height - 12))}px`
}

/** Native typing and mobile input, with an optional shared calendar in the top layer. */
export default function DateTimeField({ name, label, value, onChange, emptyLabel = 'No expiry', hint, error, disabled = false }: { name: string; label: string; value: string; onChange: (value: string) => void; emptyLabel?: string; hint?: string; error?: string; disabled?: boolean }) {
  const validation = useContext(FormContext)
  const id = useId()
  const native = useRef<HTMLInputElement>(null), trigger = useRef<HTMLButtonElement>(null), popup = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState('')
  const [month, setMonth] = useState(() => new Date())
  const [focusedDay, setFocusedDay] = useState('')
  const [calendarToday, setCalendarToday] = useState(() => today().slice(0,10))
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const commit = (next: string) => { validation?.clear(name); onChange(next) }
  const close = (restore = true) => { setOpen(false); if (restore && !trigger.current?.matches(':disabled')) trigger.current?.focus() }
  const show = () => {
    if (native.current?.matches(':disabled')) return
    const next = parseLocalDateTime(value) ? value.slice(0, 16) : today()
    setCalendarToday(today().slice(0,10)); setDraft(next); setMonth(new Date(`${next.slice(0, 10)}T12:00`)); setFocusedDay(next.slice(0, 10)); setOpen(true)
  }
  useLayoutEffect(() => {
    if (!open || !popup.current) return
    const node = popup.current
    if (node.showPopover) node.showPopover(); else node.removeAttribute('popover')
    const position = () => positionCalendar(node, native.current!)
    position()
    node.style.visibility = 'visible'
    node.querySelector<HTMLButtonElement>('[data-calendar-focus="true"]')?.focus({ preventScroll: true })
    const outside = (event: PointerEvent) => { if (!node.contains(event.target as Node) && !trigger.current?.contains(event.target as Node)) setOpen(false) }
    const escape = (event: globalThis.KeyboardEvent) => { if (event.key === 'Escape') { event.preventDefault(); event.stopImmediatePropagation(); setOpen(false); trigger.current?.focus() } }
    const dismiss = () => setOpen(false)
    const scroll = (event: Event) => { if (!node.contains(event.target as Node)) position() }
    document.addEventListener('pointerdown', outside); document.addEventListener('keydown', escape, true); window.addEventListener('resize', dismiss); window.addEventListener('scroll', scroll, true)
    return () => { document.removeEventListener('pointerdown', outside); document.removeEventListener('keydown', escape, true); window.removeEventListener('resize', dismiss); window.removeEventListener('scroll', scroll, true) }
  }, [open])
  useLayoutEffect(() => { if (open && popup.current && native.current) positionCalendar(popup.current, native.current) }, [open, month])
  const choose = (day: string) => { setDraft(`${day}T${draft.slice(11) || '12:00'}`); setFocusedDay(day) }
  const navigate = (offset: number) => setMonth(current => new Date(current.getFullYear(), current.getMonth() + offset, 1, 12))
  const dayKeys = (event: KeyboardEvent<HTMLButtonElement>, day: string) => {
    const date = new Date(`${day}T12:00`)
    const offset = ({ ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7, Home: -date.getDay(), End: 6 - date.getDay() } as Record<string, number>)[event.key]
    if (offset === undefined && event.key !== 'PageUp' && event.key !== 'PageDown') return
    event.preventDefault()
    if (offset !== undefined) date.setDate(date.getDate() + offset); else date.setMonth(date.getMonth() + (event.key === 'PageUp' ? -1 : 1), 1)
    const next = dayValue(date); setFocusedDay(next); setMonth(new Date(date.getFullYear(), date.getMonth(), 1, 12))
    requestAnimationFrame(() => popup.current?.querySelector<HTMLButtonElement>(`[data-day="${next}"]`)?.focus())
  }
  const trap = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'Tab') return
    const items = Array.from(event.currentTarget.querySelectorAll<HTMLElement>('button, input')).filter(el => el.tabIndex >= 0 && !el.matches(':disabled'))
    if (event.shiftKey && document.activeElement === items[0]) { event.preventDefault(); items.at(-1)?.focus() }
    else if (!event.shiftKey && document.activeElement === items.at(-1)) { event.preventDefault(); items[0]?.focus() }
    event.stopPropagation()
  }
  const first = new Date(month.getFullYear(), month.getMonth(), 1, 12)
  const days = new Date(month.getFullYear(), month.getMonth() + 1, 0).getDate()
  const invalid = !parseLocalDateTime(draft)
  return <FormField name={name} label={label} error={error} hint={hint ?? `Optional. Local time: ${zone}. Empty means ${emptyLabel.toLowerCase()}.`} renderControl={control => <>
    <div className="date-time-control relative min-w-0">{control}<button ref={trigger} type="button" disabled={disabled} className="date-time-open" aria-label={`Choose ${label.toLowerCase()} date and time`} aria-haspopup="dialog" aria-expanded={open} aria-controls={open ? `${id}-picker` : undefined} onClick={() => open ? close() : show()}><CalendarDays aria-hidden="true" className="size-4"/></button></div>
    {open && <div ref={popup} id={`${id}-picker`} popover="manual" role="dialog" aria-modal="true" aria-label={`${label} date and time`} className="date-time-popup" onKeyDown={trap}>
      <div className="flex items-start justify-between gap-2 border-b border-line p-4"><div><p className="text-sm font-semibold">{label}</p><p className="mt-1 text-[11px] text-ink-muted">Local time · {zone}</p></div><button type="button" className="date-calendar-action action-button action-secondary action-icon" aria-label="Close date picker" onClick={() => close()}><X className="size-4"/></button></div>
      <div className="p-3"><div className="mb-2 flex items-center justify-between"><button type="button" className="date-calendar-action action-button action-secondary action-icon" aria-label="Previous month" onClick={() => navigate(-1)}><ChevronLeft className="size-4"/></button><p aria-live="polite" className="text-sm font-semibold">{month.toLocaleDateString(undefined, { month: 'long', year: 'numeric' })}</p><button type="button" className="date-calendar-action action-button action-secondary action-icon" aria-label="Next month" onClick={() => navigate(1)}><ChevronRight className="size-4"/></button></div>
      <div className="grid grid-cols-7">{['Su','Mo','Tu','We','Th','Fr','Sa'].map(day => <span key={day} className="py-2 text-center text-[11px] font-medium text-ink-muted">{day}</span>)}</div>
      <div role="group" aria-label="Choose a day" className="grid grid-cols-7">{Array.from({ length: first.getDay() }, (_, i) => <span key={`blank-${i}`}/>)}{Array.from({ length: days }, (_, i) => {
        const date = new Date(month.getFullYear(), month.getMonth(), i + 1, 12), day = dayValue(date)
        return <button key={day} type="button" data-day={day} data-calendar-focus={day === focusedDay} tabIndex={day === focusedDay || (!focusedDay.startsWith(dayValue(first).slice(0,7)) && i === 0) ? 0 : -1} className="date-calendar-day" aria-label={date.toLocaleDateString(undefined, { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })} aria-pressed={draft.startsWith(day)} aria-current={day === calendarToday ? 'date' : undefined} onFocus={() => setFocusedDay(day)} onKeyDown={event => dayKeys(event, day)} onClick={() => choose(day)}>{i + 1}</button>
      })}</div>
      <div className="mt-3 rounded-xl border border-line bg-canvas p-3"><label htmlFor={`${id}-time`} className="mb-2 flex items-center gap-2 text-xs font-medium"><Clock3 aria-hidden="true" className="size-3.5"/>Time</label><input id={`${id}-time`} type="time" step="60" className={`${input} min-h-11`} value={draft.slice(11)} onChange={event => setDraft(`${draft.slice(0, 10)}T${event.target.value}`)} aria-invalid={invalid || undefined} aria-describedby={invalid ? `${id}-error` : undefined}/>{invalid && <p id={`${id}-error`} className="mt-2 text-xs text-red-700">Enter a valid local time. Some times do not exist when clocks change.</p>}</div>
      <div className="mt-3 flex flex-wrap items-center justify-between gap-2"><button type="button" className={`${btn} min-h-11`} onClick={() => { const next = today(); setDraft(next); setMonth(new Date(next)); setFocusedDay(next.slice(0,10)) }}>Today</button><button type="button" className={`${btn} min-h-11`} onClick={() => { commit(''); close() }}>{emptyLabel}</button></div></div>
      <div className="flex justify-end gap-2 border-t border-line p-3"><button type="button" className={`${btn} min-h-11`} onClick={() => close()}>Cancel</button><button type="button" className={`${primary} min-h-11`} disabled={invalid || disabled} onClick={() => { commit(draft); close() }}>Apply</button></div>
    </div>}
  </>}><input ref={native} type="datetime-local" step="60" disabled={disabled} className={`${input} date-time-input`} value={value} onKeyDown={event => { if (event.altKey && event.key === 'ArrowDown') { event.preventDefault(); show() } }} onChange={event => onChange(event.target.value)}/></FormField>
}
