import { render } from '../test-utils'
import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
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
  fireEvent.click(screen.getByRole('button',{name:'Edit key'}))
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


test('saved keys open in inspection mode and filters use effective credential state',async()=>{
 vi.mocked(api.listAgentKeys).mockResolvedValue([key,{...key,slug:'expired-bot',expiresAt:'2020-01-01T00:00:00Z'}])
 render(<AgentKeys/>)
 fireEvent.change(await screen.findByLabelText('Agent key status'),{target:{value:'expired'}})
 await screen.findByRole('button',{name:'expired-bot'})
 expect(screen.queryByRole('button',{name:'bot'})).toBeNull()
 fireEvent.change(screen.getByLabelText('Agent key status'),{target:{value:''}})
 fireEvent.click(screen.getByRole('button',{name:'bot'}))
 const dialog=screen.getByRole('dialog',{name:'bot'})
 expect(within(dialog).queryByRole('button',{name:'Save'})).toBeNull()
 expect(within(dialog).getByRole('button',{name:'Edit key'})).toBeTruthy()
 expect(within(dialog).getByText('vmcp_test…')).toBeTruthy()
 expect(api.patchAgentKey).not.toHaveBeenCalled()
})

test('key list paginates and search resets the visible page',async()=>{
 vi.mocked(api.listAgentKeys).mockResolvedValue(Array.from({length:110},(_,i)=>({...key,slug:`bot-${i}`})))
 render(<AgentKeys/>)
 await screen.findByRole('button',{name:'bot-0'})
 const table=screen.getByRole('table',{name:'Configured agent keys'})
 expect(within(table).getAllByRole('row')).toHaveLength(51)
 fireEvent.click(screen.getByRole('button',{name:'Show 50 more keys'}))
 expect(within(table).getAllByRole('row')).toHaveLength(101)
 fireEvent.change(screen.getByLabelText('Search agent keys'),{target:{value:'bot-109'}})
 expect(within(table).getAllByRole('row')).toHaveLength(2)
})
