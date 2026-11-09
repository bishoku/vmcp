import { afterEach, expect, test, vi } from 'vitest'
import { api, clearToken } from '../api'
import { accessFixture, ids } from './fixtures'
afterEach(() => {
  clearToken()
  vi.unstubAllGlobals()
})
test('management writes keep the original version, no-store, CSRF and a strict writable body', async () => {
  const fetch = vi
    .fn()
    .mockImplementation(() => Promise.resolve(new Response(JSON.stringify(accessFixture.roles[2]))))
  vi.stubGlobal('fetch', fetch)
  await api.updateAccessRole(accessFixture.roles[2], {
    name: 'Changed',
    description: '',
    permissions: ['audit.events.read'],
  })
  expect(fetch.mock.calls[0][1]).toMatchObject({
    method: 'PUT',
    cache: 'no-store',
    headers: { 'If-Match': '"1"', 'X-VMCP-CSRF': '1' },
  })
  expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({
    name: 'Changed',
    description: '',
    permissions: ['audit.events.read'],
  })
  await api.deleteAccessMapping(accessFixture.mappings[0])
  expect(fetch.mock.calls[1][1].headers['If-Match']).toBe('"1"')
})
test('scope and historical selectors preserve filters while following all cursor pages', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      new Response(JSON.stringify({ items: [{ id: ids.oldEpoch }], nextCursor: 'next' })),
    )
    .mockResolvedValueOnce(
      new Response(JSON.stringify({ items: [{ id: ids.newEpoch }], nextCursor: null })),
    )
  vi.stubGlobal('fetch', fetch)
  const controller = new AbortController()
  const out = await api.accessEpochs('upstream', ids.resource, controller.signal)
  expect(out.items).toHaveLength(2)
  for (const [url, init] of fetch.mock.calls) {
    const query = new URL(url, 'https://app').searchParams
    expect(query.get('kind')).toBe('upstream')
    expect(query.get('resourceId')).toBe(ids.resource)
    expect(query.get('limit')).toBe('200')
    expect(init.signal).toBe(controller.signal)
    expect(init.cache).toBe('no-store')
  }
  expect(new URL(fetch.mock.calls[1][0], 'https://app').searchParams.get('cursor')).toBe('next')
})
test('pagination fails on policy changes instead of combining different revisions', async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      new Response(JSON.stringify({ items: [], nextCursor: 'next', authorizationRevision: 1 })),
    )
    .mockResolvedValueOnce(new Response(JSON.stringify({ items: [], authorizationRevision: 2 })))
  vi.stubGlobal('fetch', fetch)
  await expect(api.accessAssignments()).rejects.toThrow('policy changed')
})
test('management requests do not retry a version conflict', async () => {
  const fetch = vi.fn().mockResolvedValue(new Response('{}', { status: 412 }))
  vi.stubGlobal('fetch', fetch)
  await expect(
    api.updateAccessMapping(accessFixture.mappings[0], {
      issuer: 'https://trusted.example',
      principalKind: 'oidc',
      conditions: [],
      enabled: false,
    }),
  ).rejects.toMatchObject({ status: 412 })
  expect(fetch).toHaveBeenCalledTimes(1)
})
