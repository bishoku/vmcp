import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { render } from '../test-utils'
import { api, ApiError } from '../api'
import { upstream } from '../connections/fixtures'
import UpstreamDetail from './UpstreamDetail'
import { RoleContext } from '../interaction'

beforeEach(() => {
  vi.spyOn(api, 'getUpstream').mockResolvedValue(upstream)
  vi.spyOn(api, 'upstreamInvocationSecurity').mockResolvedValue({
    upstreamId: 'alpha',
    revision: 0,
    policy: {},
    sharedServices: [],
    tools: [],
    callerContextFields: [],
  })
  vi.spyOn(api, 'localAuditSettings').mockResolvedValue({ revision: 1, rules: [] })
  vi.spyOn(api, 'teams').mockResolvedValue({ items: [] })
  vi.spyOn(api, 'auditSettings').mockResolvedValue({
    mode: '',
    retentionDays: 7,
    maxPayloadBytes: 65536,
    revision: 1,
    scopes: {},
    rules: [],
    virtualServers: {},
    upstreams: {},
    exportPayloads: {},
  })

  vi.spyOn(api, 'listUpstreams').mockResolvedValue([upstream])
  vi.spyOn(api, 'listTools').mockResolvedValue([{ name: 'lookup' }])
  vi.spyOn(api, 'listCatalog').mockResolvedValue({
    resources: [],
    resourceTemplates: [],
    prompts: [],
    capabilities: {},
  })
  vi.spyOn(api, 'patchUpstream').mockResolvedValue({ ...upstream, version: 3, name: 'Updated' })
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})
function open(viewer = false) {
  render(
    <MemoryRouter initialEntries={['/upstreams/alpha']}>
      <Routes>
        <Route
          path="/upstreams/:slug"
          element={
            viewer ? (
              <RoleContext.Provider value="viewer">
                <UpstreamDetail />
              </RoleContext.Provider>
            ) : (
              <UpstreamDetail />
            )
          }
        />
      </Routes>
    </MemoryRouter>,
  )
}
test('opens a full read-only page and saving returns to read-only with the latest version', async () => {
  open()
  const name = await screen.findByLabelText('Name')
  expect((name as HTMLInputElement).disabled || name.closest('fieldset')?.disabled).toBe(true)
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'Edit upstream' }))
  fireEvent.change(name, { target: { value: 'Updated' } })
  vi.mocked(api.getUpstream).mockResolvedValue({ ...upstream, version: 3, name: 'Updated' })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(api.patchUpstream).toHaveBeenCalledWith(
      'alpha',
      expect.objectContaining({ name: 'Updated' }),
      2,
    ),
  )
  expect(await screen.findByRole('button', { name: 'Edit upstream' })).toBeTruthy()
  expect(screen.getByRole('heading', { name: 'Updated' })).toBeTruthy()
  expect((screen.getByLabelText('Name').closest('fieldset') as HTMLFieldSetElement).disabled).toBe(
    true,
  )
})
test('Cancel confirms discarding dirty changes and restores saved values', async () => {
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
  open()
  await screen.findByLabelText('Name')
  fireEvent.click(screen.getByRole('button', { name: 'Edit upstream' }))
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Draft' } })
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('Draft')
  confirm.mockReturnValue(true)
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('Alpha'),
  )
  expect(api.patchUpstream).not.toHaveBeenCalled()
})
test('viewers cannot enter edit mode or modify stored configuration', async () => {
  open(true)
  await screen.findByLabelText('Name')
  expect(screen.queryByRole('button', { name: 'Edit upstream' })).toBeNull()
  expect((screen.getByLabelText('URL').closest('fieldset') as HTMLFieldSetElement).disabled).toBe(
    true,
  )
})
test('a stale write retains the draft and requires explicit recovery', async () => {
  vi.mocked(api.patchUpstream).mockRejectedValue(
    new ApiError(412, 'This item changed in another session.'),
  )
  open()
  await screen.findByLabelText('Name')
  fireEvent.click(screen.getByRole('button', { name: 'Edit upstream' }))
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Draft' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await screen.findByText('Reload latest and discard draft')
  expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('Draft')
})
test('missing upstream has a useful error state and list navigation', async () => {
  vi.mocked(api.getUpstream).mockRejectedValue(new ApiError(404, 'Not found'))
  open()
  expect(await screen.findByText('Upstream “alpha” was not found.')).toBeTruthy()
  expect(screen.getByRole('link', { name: 'Back to upstreams' }).getAttribute('href')).toBe(
    '/upstreams',
  )
})
test('creating an upstream opens its saved read-only detail page', async () => {
  vi.spyOn(api, 'createUpstream').mockResolvedValue(upstream)
  render(
    <MemoryRouter initialEntries={['/upstreams/new']}>
      <Routes>
        <Route path="/upstreams/new" element={<UpstreamDetail create />} />
        <Route path="/upstreams/:slug" element={<UpstreamDetail />} />
      </Routes>
    </MemoryRouter>,
  )
  fireEvent.change(screen.getByLabelText('Slug'), { target: { value: 'alpha' } })
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Alpha' } })
  fireEvent.change(screen.getByLabelText('URL'), { target: { value: 'https://example.com/mcp' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(await screen.findByRole('button', { name: 'Edit upstream' })).toBeTruthy()
  expect((screen.getByLabelText('Name').closest('fieldset') as HTMLFieldSetElement).disabled).toBe(
    true,
  )
})

test('upstream transport selection offers only HTTP endpoint transports', async () => {
  open()
  await screen.findByLabelText('Name')
  fireEvent.click(screen.getByRole('button', { name: 'Edit upstream' }))
  const options = within(screen.getByLabelText('Transport'))
    .getAllByRole('option')
    .map((option) => (option as HTMLOptionElement).value)
  expect(options).toEqual(['streamable_http', 'sse'])
})

test('stored credential testing remains explicit and separate from edited testing', async () => {
  const stored = vi.spyOn(api, 'testStoredConnection').mockResolvedValue({ ok: true, toolCount: 1 })
  const edited = vi.spyOn(api, 'testConnection').mockResolvedValue({ ok: true })
  const refresh = vi.spyOn(api, 'refreshTools')
  open()
  await screen.findByRole('region', { name: 'Connection verification' })
  expect(stored).not.toHaveBeenCalled()
  expect(edited).not.toHaveBeenCalled()
  expect(refresh).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Test stored credentials' }))
  await screen.findByText('Connected · 1 tools')
  expect(stored).toHaveBeenCalledWith('alpha')
  expect(edited).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Edit upstream' }))
  fireEvent.click(screen.getByRole('button', { name: 'Test edited connection' }))
  await waitFor(() =>
    expect(edited).toHaveBeenCalledWith(
      expect.objectContaining({ url: upstream.url, transport: upstream.transport, headers: {} }),
    ),
  )
})

test('discovery filtering and partial reloads retain successful snapshots without changing drafts', async () => {
  vi.mocked(api.listCatalog).mockResolvedValue({
    resources: [{ uri: 'doc://team', description: 'Team docs' }],
    resourceTemplates: [],
    prompts: [{ name: 'review' }],
    capabilities: {},
  })
  open()
  await screen.findByText('doc://team')
  fireEvent.click(screen.getByRole('radio', { name: 'Resources (1)' }))
  const table = screen.getByRole('table', { name: 'Stored upstream capabilities' })
  expect(within(table).getByText('doc://team')).toBeTruthy()
  expect(within(table).queryByText('lookup')).toBeNull()
  fireEvent.change(screen.getByLabelText('Search discovered capabilities'), {
    target: { value: 'missing' },
  })
  expect(screen.getByText('No matching capabilities')).toBeTruthy()
  fireEvent.change(screen.getByLabelText('Search discovered capabilities'), {
    target: { value: '' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Edit upstream' }))
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Draft' } })
  vi.mocked(api.listCatalog).mockRejectedValueOnce(new Error('Catalog unavailable'))
  fireEvent.click(screen.getByRole('button', { name: 'Reload discovery snapshots' }))
  await waitFor(() =>
    expect(screen.getByRole('alert').textContent).toContain('Catalog unavailable'),
  )
  expect(screen.getByText('doc://team')).toBeTruthy()
  expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('Draft')
  expect(api.patchUpstream).not.toHaveBeenCalled()
})

test('saving a name change preserves stored secrets by omitting write-only replacements', async () => {
  vi.mocked(api.listUpstreams).mockResolvedValue([
    {
      ...upstream,
      headerNames: ['Authorization'],
      tls: { ...upstream.tls, hasClientKey: true },
      auth: {
        type: 'oauth_client_credentials',
        hasClientSecret: true,
        clientId: 'test-client',
        tokenUrl: 'https://idp.example/token',
      },
    },
  ])
  open()
  await screen.findByLabelText('Name')
  fireEvent.click(screen.getByRole('button', { name: 'Edit upstream' }))
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Updated' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(api.patchUpstream).toHaveBeenCalled())
  const [, payload, version] = vi.mocked(api.patchUpstream).mock.calls[0]
  expect(version).toBe(2)
  expect(payload.headers).toBeUndefined()
  expect(payload.tls?.clientKeyPem).toBeUndefined()
  expect(payload.auth).toBeUndefined()
})

async function editSecurityAndAudit() {
  fireEvent.click(screen.getByRole('button', { name: 'Edit upstream' }))
  fireEvent.click(
    screen.getByText('Invocation security', { selector: 'summary span.font-semibold' }),
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Configure default' }))
  fireEvent.change(screen.getByLabelText('Automatic retry'), { target: { value: 'once' } })
  fireEvent.click(screen.getByRole('button', { name: 'Stage policy' }))
  fireEvent.click(
    screen.getByText('Audit payload masking', { selector: 'summary span.font-semibold' }),
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Add masking rule' }))
  fireEvent.change(screen.getByLabelText('Audit rule 1 name'), { target: { value: 'customer' } })
  fireEvent.change(screen.getByLabelText('Audit rule 1 regex'), {
    target: { value: 'CUST-[0-9]+' },
  })
}

beforeEach(() => {
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', {
    configurable: true,
    value: function (this: HTMLDialogElement) {
      this.open = true
    },
  })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', {
    configurable: true,
    value: function (this: HTMLDialogElement) {
      this.open = false
    },
  })
  vi.spyOn(api, 'saveUpstreamInvocationSecurity').mockImplementation(async (_path, value) => ({
    ...value,
    revision: value.revision + 1,
    tools: [],
    sharedServices: [],
    callerContextFields: [],
  }))
  vi.spyOn(api, 'saveLocalAuditSettings').mockImplementation(async (_path, value) => ({
    ...value,
    revision: value.revision + 1,
  }))
})

test('security and audit are read-only until Edit, and share one Save with upstream configuration', async () => {
  open()
  await screen.findByLabelText('Name')
  fireEvent.click(
    screen.getByText('Invocation security', { selector: 'summary span.font-semibold' }),
  )
  fireEvent.click(
    screen.getByText('Audit payload masking', { selector: 'summary span.font-semibold' }),
  )
  await screen.findByText(/No local masking rules/)
  expect(screen.queryByRole('button', { name: 'Configure default' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Add service' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Add masking rule' })).toBeNull()
  // Close the disclosures before exercising their normal editing entry points.
  fireEvent.click(
    screen.getByText('Invocation security', { selector: 'summary span.font-semibold' }),
  )
  fireEvent.click(
    screen.getByText('Audit payload masking', { selector: 'summary span.font-semibold' }),
  )
  await editSecurityAndAudit()
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Updated' } })
  expect(screen.queryByRole('button', { name: 'Save invocation security' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Save audit settings' })).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await screen.findByRole('button', { name: 'Edit upstream' })
  expect(api.saveUpstreamInvocationSecurity).toHaveBeenCalledOnce()
  expect(api.saveLocalAuditSettings).toHaveBeenCalledWith('/upstreams/alpha/audit-settings', {
    revision: 1,
    rules: [{ name: 'customer', pattern: 'CUST-[0-9]+' }],
  })
  expect(api.patchUpstream).toHaveBeenCalledOnce()
  expect(screen.queryByRole('button', { name: 'Add masking rule' })).toBeNull()
})

test('Cancel confirms and discards security and audit changes even when the main form is clean', async () => {
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
  open()
  await screen.findByLabelText('Name')
  await editSecurityAndAudit()
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(confirm).toHaveBeenCalled()
  expect(screen.getByDisplayValue('CUST-[0-9]+')).toBeTruthy()
  confirm.mockReturnValue(true)
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await screen.findByRole('button', { name: 'Edit upstream' })
  expect(screen.queryByText('customer', { exact: true })).toBeNull()
  expect(api.saveUpstreamInvocationSecurity).not.toHaveBeenCalled()
  expect(api.saveLocalAuditSettings).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Edit upstream' }))
  expect(screen.getByRole('button', { name: 'Configure default' })).toBeTruthy()
  expect(screen.queryByLabelText('Audit rule 1 name')).toBeNull()
})

test('partial save keeps pending drafts and retries only sections that have not saved', async () => {
  vi.mocked(api.saveLocalAuditSettings).mockRejectedValueOnce(
    new ApiError(412, 'Audit revision changed'),
  )
  open()
  await screen.findByLabelText('Name')
  await editSecurityAndAudit()
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Updated' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await screen.findByText(/Already saved: Invocation security/)
  expect(api.patchUpstream).not.toHaveBeenCalled()
  expect(screen.getByDisplayValue('CUST-[0-9]+')).toBeTruthy()
  expect(screen.getByDisplayValue('Updated')).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await screen.findByRole('button', { name: 'Edit upstream' })
  expect(api.saveUpstreamInvocationSecurity).toHaveBeenCalledTimes(1)
  expect(api.saveLocalAuditSettings).toHaveBeenCalledTimes(2)
  expect(api.patchUpstream).toHaveBeenCalledTimes(1)
})

test('security-only edits do not patch the upstream or save unchanged audit rules', async () => {
  open()
  await screen.findByLabelText('Name')
  fireEvent.click(screen.getByRole('button', { name: 'Edit upstream' }))
  fireEvent.click(
    screen.getByText('Invocation security', { selector: 'summary span.font-semibold' }),
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Configure default' }))
  fireEvent.change(screen.getByLabelText('Automatic retry'), { target: { value: 'once' } })
  fireEvent.click(screen.getByRole('button', { name: 'Stage policy' }))
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await screen.findByRole('button', { name: 'Edit upstream' })
  expect(api.saveUpstreamInvocationSecurity).toHaveBeenCalledOnce()
  expect(api.patchUpstream).not.toHaveBeenCalled()
  expect(api.saveLocalAuditSettings).not.toHaveBeenCalled()
})

test('Cancel after a partial save discards pending configuration but keeps applied security and audit changes', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(true)
  vi.mocked(api.patchUpstream).mockRejectedValueOnce(
    new ApiError(412, 'This item changed in another session.'),
  )
  open()
  await screen.findByLabelText('Name')
  await editSecurityAndAudit()
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Pending name' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await screen.findByText(/Already saved: .*Upstream configuration was not saved/)
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await screen.findByRole('button', { name: 'Edit upstream' })
  expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('Alpha')
  expect(screen.getByText('customer', { exact: true })).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: 'Edit upstream' }))
  expect(screen.getByRole('button', { name: 'Edit default' })).toBeTruthy()
  expect(screen.getByDisplayValue('CUST-[0-9]+')).toBeTruthy()
  expect(api.saveUpstreamInvocationSecurity).toHaveBeenCalledOnce()
  expect(api.saveLocalAuditSettings).toHaveBeenCalledOnce()
})
