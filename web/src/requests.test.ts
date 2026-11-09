import { describe, expect, it, vi } from 'vitest'
import { ApiError } from './api'
import { optionalSource, readSources, source } from './requests'

describe('optional scoped catalogs', () => {
  it('clears inaccessible optional data while retaining the primary panel', async () => {
    const accept = vi.fn(),
      primary = vi.fn()
    const error = await readSources(
      [
        source('Primary', async () => ['owned'], primary),
        optionalSource(
          'Optional',
          async () => {
            throw new ApiError(403, 'denied', 'configuration_access_denied')
          },
          accept,
          [],
        ),
      ],
      new AbortController().signal,
    )
    expect(error).toBe('')
    expect(primary).toHaveBeenCalledWith(['owned'])
    expect(accept).toHaveBeenCalledWith([])
  })
  it.each([new ApiError(401, 'expired'), new ApiError(403, 'server denial'), new Error('network')])(
    'surfaces authentication/server failures: %s',
    async (failure) => {
      const error = await readSources(
        [
          optionalSource(
            'Catalog',
            async () => {
              throw failure
            },
            vi.fn(),
            [],
          ),
        ],
        new AbortController().signal,
      )
      expect(error).toContain(failure.message)
    },
  )
  it('does not update unmounted data on a delayed denial', async () => {
    const controller = new AbortController(),
      accept = vi.fn()
    await optionalSource(
      'Catalog',
      async () => {
        controller.abort()
        throw new ApiError(403, 'denied', 'configuration_access_denied')
      },
      accept,
      [],
    ).load(controller.signal)
    expect(accept).not.toHaveBeenCalled()
  })
})
