import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { render } from './test-utils'
import { api } from './api'
import AdmissionSettingsPanel from './AdmissionSettingsPanel'
vi.unmock('./configuration-access')
const settings = { revision: 3, total: 1024, perServer: 128, perPrincipal: 16 }
beforeEach(() => {
  vi.spyOn(api, 'configurationAccess').mockResolvedValue({
    authorizationRevision: 1,
    allowed: [true, true],
  })
  vi.spyOn(api, 'admissionSettings').mockResolvedValue({ ...settings })
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})
test('operator updates local limits with saved revision', async () => {
  const save = vi
    .spyOn(api, 'saveAdmissionSettings')
    .mockImplementation(async (s) => ({ ...s, revision: 4 }))
  render(<AdmissionSettingsPanel />)
  const field = await screen.findByLabelText('Concurrent requests per principal')
  fireEvent.change(field, { target: { value: '20' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save runtime protection' }))
  await waitFor(() => expect(save).toHaveBeenCalledWith({ ...settings, perPrincipal: 20 }))
  await screen.findByRole('status')
})
test('invalid limits cannot be saved and conflict preserves draft', async () => {
  vi.spyOn(api, 'saveAdmissionSettings').mockRejectedValue(
    new Error('Reload admission settings before saving'),
  )
  render(<AdmissionSettingsPanel />)
  const field = await screen.findByLabelText('Concurrent requests per principal')
  fireEvent.change(field, { target: { value: '0' } })
  expect(
    (screen.getByRole('button', { name: 'Save runtime protection' }) as HTMLButtonElement).disabled,
  ).toBe(true)
  fireEvent.change(field, { target: { value: '20' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save runtime protection' }))
  await screen.findByText('Reload admission settings before saving')
  expect((field as HTMLInputElement).value).toBe('20')
  fireEvent.click(screen.getByRole('button', { name: 'Reload runtime protection' }))
  await waitFor(() => expect((field as HTMLInputElement).value).toBe('16'))
})
test('unprivileged caller does not fetch settings', async () => {
  vi.spyOn(api, 'configurationAccess').mockResolvedValue({
    authorizationRevision: 1,
    allowed: [false, false],
  })
  render(<AdmissionSettingsPanel />)
  await waitFor(() => expect(api.configurationAccess).toHaveBeenCalled())
  expect(api.admissionSettings).not.toHaveBeenCalled()
  expect(screen.queryByText('Runtime protection')).toBeNull()
})
test('read permission does not allow editing', async () => {
  vi.spyOn(api, 'configurationAccess').mockResolvedValue({
    authorizationRevision: 1,
    allowed: [true, false],
  })
  render(<AdmissionSettingsPanel />)
  await screen.findByLabelText('Concurrent requests per principal')
  expect(screen.queryByRole('button', { name: 'Save runtime protection' })).toBeNull()
  expect(
    (
      screen
        .getByLabelText('Concurrent requests per principal')
        .closest('fieldset') as HTMLFieldSetElement
    ).disabled,
  ).toBe(true)
})
