import { cleanup, fireEvent, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { render } from '../test-utils'
import Services from './Services'
import type { InvocationServices } from '../api'

beforeEach(() => {
  Element.prototype.scrollIntoView = vi.fn()
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
afterEach(cleanup)
function editor(services: InvocationServices = {}) {
  const change = vi.fn()
  render(
    <Services
      services={services}
      used={new Set()}
      writable
      busy={false}
      scope="shared"
      onChange={change}
    />,
  )
  fireEvent.click(
    screen.getByRole('button', {
      name: Object.keys(services).length ? 'Edit service corporate' : 'Add service',
    }),
  )
  return change
}
function fillBase() {
  fireEvent.change(screen.getByLabelText('Service name'), { target: { value: 'corporate' } })
  fireEvent.change(screen.getByLabelText('HTTPS decision endpoint'), {
    target: { value: 'https://approval.test/check' },
  })
  fireEvent.change(screen.getByLabelText('Trusted approval screen origin'), {
    target: { value: 'https://approval.test' },
  })
}
test('stages explicit unauthenticated service and bounded timeout', () => {
  const change = editor()
  fillBase()
  fireEvent.change(screen.getByLabelText('Service authentication'), { target: { value: 'none' } })
  fireEvent.change(screen.getByLabelText('Request timeout (seconds)'), { target: { value: '61' } })
  fireEvent.click(screen.getByRole('button', { name: 'Stage service' }))
  expect(change).not.toHaveBeenCalled()
  fireEvent.change(screen.getByLabelText('Request timeout (seconds)'), { target: { value: '8' } })
  fireEvent.click(screen.getByRole('button', { name: 'Stage service' }))
  expect(change.mock.calls[0][0].corporate).toMatchObject({ authType: 'none', timeoutSeconds: 8 })
  expect(change.mock.calls[0][0].corporate.token).toBeUndefined()
})
test('stages OAuth client credentials without bearer secret', () => {
  const change = editor()
  fillBase()
  fireEvent.change(screen.getByLabelText('Service authentication'), {
    target: { value: 'oauth_client_credentials' },
  })
  fireEvent.change(screen.getByLabelText('OAuth token endpoint'), {
    target: { value: 'https://idp.test/token' },
  })
  fireEvent.change(screen.getByLabelText('OAuth client ID'), { target: { value: 'client' } })
  fireEvent.change(screen.getByLabelText('OAuth client secret'), { target: { value: 'secret' } })
  fireEvent.change(screen.getByLabelText('OAuth scopes'), { target: { value: 'approval read' } })
  fireEvent.click(screen.getByRole('button', { name: 'Stage service' }))
  expect(change.mock.calls[0][0].corporate.oauth).toMatchObject({
    clientId: 'client',
    clientSecret: 'secret',
    scopes: ['approval', 'read'],
    authStyle: 'client_secret_basic',
  })
  expect(change.mock.calls[0][0].corporate.token).toBeUndefined()
})
test('retains redacted headers only for unchanged destination', () => {
  const change = editor({
    corporate: {
      url: 'https://approval.test/check',
      approvalOrigin: 'https://approval.test',
      authType: 'headers',
      headers: { 'X-Key': '' },
      hasHeaders: { 'X-Key': true },
    },
  })
  fireEvent.change(screen.getByLabelText('HTTPS decision endpoint'), {
    target: { value: 'https://other.test/check' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Stage service' }))
  expect(change).not.toHaveBeenCalled()
  fireEvent.change(screen.getByLabelText('Header 1 value'), { target: { value: 'new-secret' } })
  fireEvent.click(screen.getByRole('button', { name: 'Stage service' }))
  expect(change.mock.calls[0][0].corporate.headers).toEqual({ 'X-Key': 'new-secret' })
})
