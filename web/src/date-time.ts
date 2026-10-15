/** Local wall-clock values must survive a round trip, including DST gaps. */
export function parseLocalDateTime(value: string): Date | null {
  const parts = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{1,3}))?)?$/.exec(value)
  if (!parts) return null
  const [, y, m, d, h, min, sec = '0', ms = '0'] = parts
  const date = new Date(value)
  return Number.isFinite(date.getTime()) && date.getFullYear() === +y && date.getMonth() === +m - 1 && date.getDate() === +d && date.getHours() === +h && date.getMinutes() === +min && date.getSeconds() === +sec && date.getMilliseconds() === +ms.padEnd(3, '0') ? date : null
}
export function localDateTime(iso?: string | null): string {
  if (!iso) return ''
  const date = new Date(iso)
  return Number.isFinite(date.getTime()) ? new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16) : ''
}
export function auditDateErrors(filters: { from?: string; to?: string }): { from?: string; to?: string } {
  if (filters.from && !Number.isFinite(Date.parse(filters.from))) return { from: 'Enter a valid start date and time.' }
  if (filters.to && !Number.isFinite(Date.parse(filters.to))) return { to: 'Enter a valid end date and time.' }
  if (filters.from && filters.to && Date.parse(filters.from) > Date.parse(filters.to)) return { to: 'End time must be on or after the start time.' }
  return {}
}
