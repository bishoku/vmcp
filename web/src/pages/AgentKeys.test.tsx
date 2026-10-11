import { render } from '../test-utils'
import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { api, type AgentKey } from '../api'
import AgentKeys from './AgentKeys'

const key: AgentKey = {
  slug: 'bot', version: 9, description: 'Original', keyPrefix: 'vmcp_test',
  virtualServers: [], ipAllowlist: [], rateLimitPerMinute: null,
  expiresAt: '2027-02-03T04:05:06.789Z', revokedAt: null, status: 'active', createdAt: '2026-01-01T00:00:00Z',
}
beforeEach(() => {
  // jsdom has no modal implementation; actual focus/keyboard behavior is tested in Chromium.
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', { configurable: true, value: function (this: HTMLDialogElement) { this.open = true } })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', { configurable: true, value: function (this: HTMLDialogElement) { this.open = false } })
  vi.spyOn(api, 'listAgentKeys').mockResolvedValue([key])
  vi.spyOn(api, 'listVirtualServers').mockResolvedValue([])
  vi.spyOn(api, 'patchAgentKey').mockResolvedValue(key)
})
afterEach(() => { cleanup(); Reflect.deleteProperty(HTMLDialogElement.prototype, 'showModal'); Reflect.deleteProperty(HTMLDialogElement.prototype, 'close'); vi.restoreAllMocks() })
async function open() {
  render(<AgentKeys />)
  fireEvent.click(await screen.findByRole('button', { name: 'bot' }))
}
async function save() {
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(api.patchAgentKey).toHaveBeenCalledTimes(1))
  return vi.mocked(api.patchAgentKey).mock.calls[0]
}
test('unrelated edits preserve the exact expiry instant including seconds/milliseconds', async () => {
  await open()
  fireEvent.change(screen.getByLabelText('Description'), { target: { value: 'Updated' } })
  const [slug, body, version] = await save()
  expect(slug).toBe('bot')
  expect(version).toBe(9)
  expect(body).toMatchObject({ description: 'Updated', expiresAt: key.expiresAt })
})
test('explicit expiry edits interpret the entered datetime in the displayed local timezone', async () => {
  await open()
  fireEvent.change(screen.getByLabelText(/Expires/), { target: { value: '2027-04-05T06:07' } })
  const [, body] = await save()
  expect(body.expiresAt).toBe(new Date('2027-04-05T06:07').toISOString())
  expect(body.expiresAt).not.toBe(key.expiresAt)
})
test('clearing expiry explicitly removes it', async () => {
  await open()
  fireEvent.change(screen.getByLabelText(/Expires/), { target: { value: '' } })
  const [, body] = await save()
  expect(body.expiresAt).toBeNull()
})
