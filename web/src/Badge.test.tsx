import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'
import { Badge, StatusBadge } from './Badge'
import { statusMeta } from './status-meta'
afterEach(cleanup)
test('compound relationship qualifiers keep warning and failure states visible',()=>{
 expect(statusMeta('active · server disabled').tone).toBe('neutral')
 expect(statusMeta('active · anonymous access bypasses credentials').tone).toBe('warning')
 expect(statusMeta('expired · server disabled').tone).toBe('warning')
 expect(statusMeta('revoked · server disabled').tone).toBe('danger')
 expect(statusMeta('agent key · active').tone).toBe('success')
})
test('saved discovery and missing outcomes do not imply runtime success or failure',()=>{
 for(const status of ['discovered','snapshot available','admission','started','allow'])expect(statusMeta(status).tone).toBe('info')
 expect(statusMeta('outcome not loaded').tone).toBe('warning')
 expect(statusMeta('not refreshed').tone).toBe('neutral')
 expect(statusMeta('new_backend_state').tone).toBe('neutral')
})
test('static badges retain readable labels and decorative icons without adding live announcements',()=>{
 const {container}=render(<><StatusBadge status="tool_error"/><StatusBadge status={undefined}/><StatusBadge status="refresh failed" label="Refresh failed"/><Badge>read</Badge></>)
 expect(screen.getByText('tool error')).toBeTruthy();expect(screen.getByText('unknown')).toBeTruthy();expect(screen.getByText('Refresh failed')).toBeTruthy()
 expect(container.querySelectorAll('svg[aria-hidden="true"]')).toHaveLength(4)
 expect(screen.queryByRole('status')).toBeNull()
 expect(container.querySelector('[data-state="tool_error"]')?.getAttribute('data-tone')).toBe('warning')
})
