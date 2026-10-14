import { render } from '../test-utils'
import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { api } from '../api'
import Settings from './Settings'

afterEach(() => { cleanup(); vi.restoreAllMocks() })

test('viewers can download both specifications using the authenticated API', async () => {
  vi.spyOn(api, 'listAdminTokens').mockResolvedValue([])
  vi.spyOn(api, 'info').mockResolvedValue({ version: 'test', configRevision: 1 })
  const download = vi.spyOn(api, 'downloadOpenAPI').mockResolvedValue(new Blob(['spec']))
  const create = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:spec')
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  render(<Settings role="viewer" />)
  fireEvent.click(screen.getByRole('button', { name: 'Download OpenAPI JSON' }))
  await waitFor(() => expect(create).toHaveBeenCalledTimes(1))
  await waitFor(() => expect((screen.getByRole('button', { name: 'Download OpenAPI YAML' }) as HTMLButtonElement).disabled).toBe(false))
  fireEvent.click(screen.getByRole('button', { name: 'Download OpenAPI YAML' }))
  await waitFor(() => expect(download.mock.calls).toEqual([['json'], ['yaml']]))
})
