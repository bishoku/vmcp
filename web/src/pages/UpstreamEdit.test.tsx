import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { UpstreamEditProvider } from './UpstreamEdit'
import { useUpstreamEdit } from './upstream-edit-session'

afterEach(cleanup)

test('retry skips a completed write before rendered dirty snapshots settle, but includes a new edit', async () => {
  const { result } = renderHook(useUpstreamEdit, { wrapper: UpstreamEditProvider })
  let invocationDirty = true
  let auditDirty = true
  const invocation = {
    label: 'Invocation security',
    writable: true,
    dirty: true,
    blocked: false,
    isDirty: () => invocationDirty,
    save: vi.fn(async () => {
      invocationDirty = false
    }),
    discard: vi.fn(),
  }
  const audit = {
    label: 'Audit payload masking',
    writable: true,
    dirty: true,
    blocked: false,
    isDirty: () => auditDirty,
    save: vi
      .fn()
      .mockRejectedValueOnce(new Error('Audit revision changed'))
      .mockImplementation(async () => {
        auditDirty = false
      }),
    discard: vi.fn(),
  }
  act(() => {
    result.current!.register('invocation', invocation)
    result.current!.register('audit', audit)
  })
  await act(async () => {
    await expect(result.current!.saveSections()).rejects.toThrow(
      'Already saved: Invocation security',
    )
    // The registry still contains dirty:true for both sections. No re-registration
    // or render is needed between the failed attempt and this immediate retry.
    await expect(result.current!.saveSections()).resolves.toEqual(['Audit payload masking'])
  })
  expect(invocation.save).toHaveBeenCalledTimes(1)
  expect(audit.save).toHaveBeenCalledTimes(2)
  invocationDirty = true
  await act(async () => {
    await expect(result.current!.saveSections()).resolves.toEqual(['Invocation security'])
  })
  expect(invocation.save).toHaveBeenCalledTimes(2)
  expect(audit.save).toHaveBeenCalledTimes(2)
})
