import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { api, type Upstream, type VirtualServer } from '../api'
import VirtualServers from './VirtualServers'

const server: VirtualServer = {
  slug: 'demo', name: 'Demo server', description: '', version: 7,
  enabled: true, authRequired: true, endpoint: '/mcp/v1/virtual/demo',
  policy: { default: 'allow', tools: { first: 'deny', second: 'read_only' } },
  tools: [
    { upstream: 'alpha', tool: 'one', alias: 'first', exposedName: 'first', upstreamDescription: '', descriptionDrift: false, orphaned: false },
    { upstream: 'alpha', tool: 'two', alias: 'second', exposedName: 'second', upstreamDescription: '', descriptionDrift: false, orphaned: false },
  ],
}
beforeEach(() => {
  vi.spyOn(api, 'listVirtualServers').mockResolvedValue([server])
  vi.spyOn(api, 'listUpstreams').mockResolvedValue([{ slug: 'alpha', name: 'Alpha' }] as Upstream[])
  vi.spyOn(api, 'listTools').mockResolvedValue([{ name: 'one' }, { name: 'two' }])
  vi.spyOn(api, 'listCatalog').mockResolvedValue({ resources: [], resourceTemplates: [], prompts: [], capabilities: {} })
  vi.spyOn(api, 'putVirtualServer').mockResolvedValue(server)
})
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllEnvs() })
async function open() {
  render(<VirtualServers />)
  fireEvent.click(await screen.findByText('Demo server'))
}
async function save() {
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(api.putVirtualServer).toHaveBeenCalledTimes(1))
  return vi.mocked(api.putVirtualServer).mock.calls[0]
}
test('renaming and clearing aliases preserve deny/read-only policies and the original version', async () => {
  await open()
  fireEvent.change(screen.getByLabelText('Alias for alpha/one'), { target: { value: 'renamed' } })
  fireEvent.change(screen.getByLabelText('Alias for alpha/two'), { target: { value: '' } })
  const [slug, body, version] = await save()
  expect(slug).toBe('demo')
  expect(version).toBe(7)
  expect(body.policy).toEqual({ default: 'allow', tools: { renamed: 'deny', alpha__two: 'read_only' } })
})
test('temporary duplicates block saving and swapping aliases keeps policies with their tools', async () => {
  await open()
  fireEvent.change(screen.getByLabelText('Alias for alpha/one'), { target: { value: 'second' } })
  expect((screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement).disabled).toBe(true)
  fireEvent.change(screen.getByLabelText('Alias for alpha/two'), { target: { value: 'first' } })
  const [, body] = await save()
  expect(body.policy).toEqual({ default: 'allow', tools: { second: 'deny', first: 'read_only' } })
})
test('removing a selection removes its obsolete policy from the saved configuration', async () => {
  await open()
  fireEvent.click(screen.getAllByRole('button', { name: 'Remove' })[0])
  const [, body] = await save()
  expect(body.tools).toHaveLength(1)
  expect(body.policy?.tools).toEqual({ second: 'read_only' })
})
test('shows and copies the configured public gateway endpoint', async () => {
  vi.stubEnv('VITE_PUBLIC_GATEWAY_URL', 'http://localhost:8800')
  const copy = vi.fn().mockResolvedValue(undefined)
  const original = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: copy } })
  try {
    await open()
    expect(screen.getByText('http://localhost:8800/mcp/v1/virtual/demo')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Copy endpoint' }))
    expect(copy).toHaveBeenCalledWith('http://localhost:8800/mcp/v1/virtual/demo')
  } finally {
    if (original) Object.defineProperty(navigator, 'clipboard', original)
    else Reflect.deleteProperty(navigator, 'clipboard')
  }
})
test('without public gateway configuration only a path is shown and endpoint copying is disabled', async () => {
  vi.stubEnv('VITE_PUBLIC_GATEWAY_URL', '')
  await open()
  expect(screen.getByText('/mcp/v1/virtual/demo')).toBeTruthy()
  expect(screen.getByText(/Public gateway URL is not configured/).textContent).toContain('not configured')
  expect((screen.getByRole('button', { name: 'Copy endpoint' }) as HTMLButtonElement).disabled).toBe(true)
})
