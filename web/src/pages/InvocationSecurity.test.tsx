import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, afterEach, expect, test, vi } from 'vitest'
import { render } from '../test-utils'
import { api, type UpstreamInvocationSecurity } from '../api'
import { RoleContext } from '../interaction'
import { MemoryRouter } from 'react-router-dom'
import UpstreamSecurity, { ToolInvocationPolicy } from '../invocation/UpstreamSecurity'
import InvocationSecurity from './InvocationSecurity'
import { UpstreamEditProvider } from './UpstreamEdit'
import { useUpstreamEdit } from './upstream-edit-session'

function SessionControls() {
  const session = useUpstreamEdit()!
  return (
    <>
      <button onClick={() => session.setEditing(true)}>Edit upstream</button>
      <button disabled={session.blocked} onClick={() => void session.saveSections()}>
        Save
      </button>
    </>
  )
}
function openPolicy() {
  render(
    <UpstreamEditProvider>
      <SessionControls />
      <UpstreamSecurity slug="payments" version={1}>
        <section aria-label="Discovered tool">
          <ToolInvocationPolicy name="transfer" />
        </section>
      </UpstreamSecurity>
    </UpstreamEditProvider>,
  )
}

beforeEach(() => {
  Element.prototype.scrollIntoView = vi.fn()
  vi.spyOn(api, 'localAuditSettings').mockResolvedValue({ revision: 1, rules: [] })
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
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  Reflect.deleteProperty(HTMLDialogElement.prototype, 'showModal')
  Reflect.deleteProperty(HTMLDialogElement.prototype, 'close')
})

const upstream: UpstreamInvocationSecurity = {
  revision: 0,
  upstreamId: 'payments',
  sharedServices: ['shared'],
  tools: ['transfer'],
  callerContextFields: ['sessionId'],
  policy: {
    identity: { source: 'oauth_subject' },
    stepUp: { acceptedAcrValues: ['mfa', 'hardware'], maxAgeSeconds: 300 },
  },
}

test('tool editor previews inherited policy and rejects disjoint step-up requirements before staging', async () => {
  vi.spyOn(api, 'upstreamInvocationSecurity').mockResolvedValue(upstream)
  const save = vi
    .spyOn(api, 'saveUpstreamInvocationSecurity')
    .mockImplementation(async (_path, doc) => ({ ...upstream, ...doc, revision: 2 }))
  openPolicy()
  const row = await screen.findByRole('region', { name: 'Discovered tool' })
  fireEvent.click(
    await screen.findByRole('button', { name: 'View invocation policy for transfer' }),
  )
  expect(screen.queryByRole('button', { name: 'Add invocation rule for transfer' })).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'Edit upstream' }))
  fireEvent.click(screen.getByRole('button', { name: 'Add invocation rule for transfer' }))
  fireEvent.change(screen.getByLabelText('Accepted ACR values'), { target: { value: 'password' } })
  fireEvent.click(screen.getByRole('button', { name: 'Stage policy' }))
  expect(screen.getByRole('dialog')).toBeTruthy()
  expect(
    screen.getByText(/Choose at least one ACR accepted by the upstream policy/, {
      selector: 'div[data-validation-summary] li button',
    }),
  ).toBeTruthy()
  fireEvent.change(screen.getByLabelText('Accepted ACR values'), { target: { value: 'hardware' } })
  fireEvent.change(screen.getByLabelText('Maximum authentication age (seconds)'), {
    target: { value: '60' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Stage policy' }))
  expect(row.textContent).toContain('Tool-specific invocation policy')
  expect(screen.getByRole('region', { name: 'Tool invocation policies' }).textContent).toContain(
    'hardware · max 60s',
  )
  expect(save).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(save).toHaveBeenCalledTimes(1))
  expect(save.mock.calls[0][1].policy.tools?.transfer).toEqual({
    stepUp: { acceptedAcrValues: ['hardware'], maxAgeSeconds: 60 },
  })
})

test('read-only upstream access shows the effective rule without mutation controls', async () => {
  vi.spyOn(api, 'upstreamInvocationSecurity').mockResolvedValue(upstream)
  render(
    <RoleContext.Provider value="viewer">
      <UpstreamSecurity slug="payments" version={1}>
        <p>Upstream details</p>
      </UpstreamSecurity>
    </RoleContext.Provider>,
  )
  await screen.findByRole('heading', { name: 'Tool invocation policies', hidden: true })
  fireEvent.click(
    screen.getByText('Invocation security', { selector: 'summary span.font-semibold' }),
  )
  expect(screen.getByText('Uses upstream default', { selector: 'summary span' })).toBeTruthy()
  expect(screen.queryByRole('button', { name: 'Edit default' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()
})

test('failed shared-service save retains staged credentials and submits only shared service settings', async () => {
  const settings = { revision: 1, services: {}, inUse: [] }
  vi.spyOn(api, 'sharedInvocationServices').mockResolvedValue(settings)
  const save = vi
    .spyOn(api, 'saveSharedInvocationServices')
    .mockRejectedValue(new Error('Revision changed. Reload and review.'))
  render(
    <MemoryRouter>
      <InvocationSecurity />
    </MemoryRouter>,
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Add service' }))
  fireEvent.change(screen.getByLabelText('Service name'), { target: { value: 'approval' } })
  fireEvent.change(screen.getByLabelText('Service token'), {
    target: { value: 'private-service-token' },
  })
  fireEvent.change(screen.getByLabelText('HTTPS decision endpoint'), {
    target: { value: 'http://invalid.test' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Stage service' }))
  expect(
    await screen.findByText('Enter an HTTPS URL without embedded credentials or a fragment.'),
  ).toBeTruthy()
  expect((screen.getByLabelText('Service token') as HTMLInputElement).value).toBe(
    'private-service-token',
  )
  fireEvent.change(screen.getByLabelText('HTTPS decision endpoint'), {
    target: { value: 'https://approval.test/check' },
  })
  fireEvent.change(screen.getByLabelText('Trusted approval screen origin'), {
    target: { value: 'https://approval.test' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Stage service' }))
  fireEvent.click(screen.getByRole('button', { name: 'Save shared services' }))
  expect(await screen.findByText(/Revision changed/)).toBeTruthy()
  expect(screen.getByText('Token pending save')).toBeTruthy()
  expect(Object.keys(save.mock.calls[0][0]).sort()).toEqual(['revision', 'services'])
})

test('retry and read-only decisions stage independently and save the effective tool override', async () => {
  const doc = { ...upstream, policy: { retry: 'never' as const, readOnly: true } }
  vi.spyOn(api, 'upstreamInvocationSecurity').mockResolvedValue(doc)
  const save = vi
    .spyOn(api, 'saveUpstreamInvocationSecurity')
    .mockImplementation(async (_path, value) => ({ ...doc, ...value, revision: 2 }))
  openPolicy()
  const row = await screen.findByRole('region', { name: 'Discovered tool' })
  fireEvent.click(
    await screen.findByRole('button', { name: 'View invocation policy for transfer' }),
  )
  fireEvent.click(screen.getByRole('button', { name: 'Edit upstream' }))
  fireEvent.click(screen.getByRole('button', { name: 'Add invocation rule for transfer' }))
  fireEvent.change(screen.getByLabelText('Automatic retry'), { target: { value: 'once' } })
  fireEvent.change(screen.getByLabelText('Read-only classification'), {
    target: { value: 'false' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Stage policy' }))
  expect(save).not.toHaveBeenCalled()
  expect(row.textContent).toContain('Tool-specific invocation policy')
  expect(
    within(screen.getByRole('region', { name: 'Tool invocation policies' })).getByText(
      'once for eligible MCP calls',
    ),
  ).toBeTruthy()
  expect(
    within(screen.getByRole('region', { name: 'Tool invocation policies' })).getByText(
      'Not classified read-only',
    ),
  ).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(save).toHaveBeenCalledTimes(1))
  expect(save.mock.calls[0][1].policy.tools?.transfer).toMatchObject({
    retry: 'once',
    readOnly: false,
  })
  expect(save.mock.calls[0][1].policy.retry).toBe('never')
})
