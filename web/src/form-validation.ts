import { parseLocalDateTime } from './date-time'
import { createContext, useEffect, useRef, useState } from 'react'
import { ApiError } from './api'

export interface FormIssue { field?: string; message: string }
const fieldPath = (field = '') => field.replace(/^\//, '').replace(/\//g, '.').replace(/\[(\d+)\]/g, '.$1')
function findField(root: HTMLElement | null, field?: string) {
  if (!field) return null
  const controls = Array.from(root?.querySelectorAll<HTMLElement>('[data-validation-field]') ?? [])
  // Indexed server errors can belong to a single multiline input, such as the IP allowlist.
  return controls.find(node => node.dataset.validationField === field) ?? controls.find(node => field.startsWith(`${node.dataset.validationField}.`))
}
export function focusField(root: HTMLElement | null, field?: string) {
  const control = findField(root, field)
  const target = (control?.matches(':disabled')?null:control) ?? root?.querySelector<HTMLElement>('[data-validation-summary]')
  if (!target) return
  for (let parent = target.parentElement; parent && parent !== root; parent = parent.parentElement) if (parent instanceof HTMLDetailsElement) parent.open = true
  target.focus({ preventScroll: true })
  target.scrollIntoView?.({ block: 'center', behavior: 'instant' })
}

export function useFormValidation() {
  const [issues, setIssues] = useState<FormIssue[]>([])
  const [attempt, setAttempt] = useState(0)
  const root = useRef<HTMLDivElement>(null)
  const focus = useRef<string|undefined>(undefined)
  useEffect(() => {
    if (!attempt) return
    const frame = requestAnimationFrame(() => focusField(root.current, focus.current))
    return () => cancelAnimationFrame(frame)
  }, [attempt])
  const report = (next: FormIssue[]) => { setIssues(next); focus.current = next[0]?.field; if (next.length) setAttempt(n => n + 1) }
  return {
    root, issues,
    reset: () => setIssues([]),
    clear: (field: string) => setIssues(current => current.filter(issue => issue.field !== field && !issue.field?.startsWith(`${field}.`))),
    check: (rules: Record<string, string | undefined>) => { const next = Object.entries(rules).flatMap(([field, message]) => message ? [{ field, message }] : []); report(next); return next.length === 0 },
    fail: (error: unknown) => report(error instanceof ApiError && error.errors.length ? error.errors.map(issue => ({ field: fieldPath(issue.field), message: issue.message })) : [{ message: error instanceof Error ? error.message : String(error) }]),
  }
}
export type Validation = ReturnType<typeof useFormValidation>
export const FormContext = createContext<Omit<Validation,'root'> | null>(null)
export const requiredText = (value: string, label: string, max?: number) => !value.trim() ? `${label} is required.` : max && value.length > max ? `Use ${max} characters or fewer.` : undefined
export const slugError = (value: string) => !value ? 'Enter a stable identifier.' : value.length > 63 || !/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(value) ? 'Use 1–63 lowercase letters, numbers or hyphens; start and end with a letter or number.' : undefined
export const rateError = (value: string) => value && (!Number.isInteger(Number(value)) || Number(value) < 1 || Number(value) > 1000000) ? 'Enter a whole number from 1 to 1,000,000, or leave empty for unlimited.' : undefined
export const dateError = (value: string) => value && !parseLocalDateTime(value) ? 'Enter a valid date and time, or leave empty for no expiry.' : undefined
export const urlError = (value: string) => { try { const url = new URL(value); if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.hash) throw new Error(); return undefined } catch { return 'Enter an absolute HTTP or HTTPS URL without credentials or a fragment.' } }

export function fieldLabel(path: string) {
  const labels: Record<string,string> = {slug:'Identifier',name:'Name',url:'URL',description:'Description',rateLimitPerMinute:'Rate limit',ipAllowlist:'IP allowlist',expiresAt:'Expiry',fallbackUrls:'Fallback URLs',auth:'Authentication',tokenUrl:'Token URL',clientId:'Client ID',clientSecret:'Client secret',tls:'TLS',caPem:'CA certificates',serverName:'Server name',clientCertPem:'Client certificate',clientKeyPem:'Client key',conditions:'Conditions',claim:'Claim',value:'Value',op:'Operator',document:'Configuration document',token:'Admin token',virtualServers:'Virtual server grants',tools:'Tools',catalog:'Catalog objects',pii:'Data masking',customRules:'Custom rules',pattern:'Pattern',alias:'Alias',clientIp:'Client IP',claims:'Token claims',scope:'Scope',scopes:'Scopes',audience:'Audience',headers:'Headers'}
  return path.split('.').map(part=>/^\d+$/.test(part)?String(Number(part)+1):labels[part]??part.replace(/([a-z])([A-Z])/g,'$1 $2')).join(' / ')
}
