import { expect, test } from '@playwright/test'
import { withConfirmation } from './confirmation'
import { configurationFixture } from './configuration-fixtures'
import { emptyREST } from '../src/connections/rest-config'
import { upstream } from '../src/connections/fixtures'
import type { UpstreamInvocationSecurity } from '../src/api'

for (const kind of ['mcp', 'rest'] as const)
  test(`${kind} upstream read-only summaries open central policies and share the page edit session on mobile`, async ({
    page,
  }) => {
    let saves = 0
    let doc: UpstreamInvocationSecurity = {
      upstreamId: 'payments-id',
      revision: 1,
      tools: ['transfer'],
      sharedServices: ['corporate'],
      callerContextFields: [],
      policy: {
        identity: { source: 'oauth_subject' },
        stepUp: { acceptedAcrValues: ['mfa', 'hardware'], maxAgeSeconds: 300 },
      },
    }
    await page.route('**/auth/config', (r) => r.fulfill({ json: { oidc: false } }))
    await page.route('**/api/v1/**', (route) => {
      const capability = configurationFixture(route, 'operator', true)
      if (capability) return capability
      const path = new URL(route.request().url()).pathname
      if (path.endsWith('/me'))
        return route.fulfill({
          status: route.request().headers().authorization ? 200 : 401,
          json: { name: 'Payments owner', role: 'team' },
        })
      if (path.endsWith('/upstreams/payments'))
        return route.fulfill({
          json: {
            ...upstream,
            slug: 'payments',
            name: 'Payments',
            ownerTeam: 'payments',
            kind,
            ...(kind === 'rest'
              ? {
                  rest: {
                    ...emptyREST(),
                    operations: [
                      {
                        name: 'transfer',
                        description: 'Transfer funds',
                        method: 'POST',
                        path: '/transfer',
                        enabled: true,
                        readOnly: false,
                        parameters: [],
                      },
                    ],
                  },
                }
              : {}),
          },
        })
      if (path.endsWith('/upstreams/payments/invocation-security')) {
        if (route.request().method() === 'PUT') {
          saves++
          const data = route.request().postDataJSON()
          doc = { ...doc, ...data, revision: doc.revision + 1 }
        }
        return route.fulfill({ json: doc })
      }
      if (path.endsWith('/tools'))
        return route.fulfill({
          json: { items: [{ name: 'transfer', description: 'Transfer funds' }] },
        })
      if (path.endsWith('/catalog'))
        return route.fulfill({
          json: { resources: [], resourceTemplates: [], prompts: [], capabilities: {} },
        })
      if (path.endsWith('/audit-settings'))
        return route.fulfill({ json: { revision: 1, rules: [] } })
      return route.fulfill({ json: { items: [] } })
    })
    await page.goto('/upstreams/payments')
    await page.getByLabel('Admin token').fill('test')
    await page.getByRole('button', { name: 'Continue', exact: true }).click()
    const discovery = page.getByRole('region', {
      name: kind === 'rest' ? 'REST operations' : 'Upstream discovery',
    })
    const security = page.getByRole('region', { name: 'Upstream invocation security' })
    await expect(discovery.getByText('Uses upstream invocation default')).toBeVisible()
    await expect(
      discovery.getByRole('button', { name: 'Add invocation rule for transfer' }),
    ).toHaveCount(0)
    await discovery.getByRole('button', { name: 'View invocation policy for transfer' }).click()
    await expect(security.getByText('mfa, hardware · max 300s').last()).toBeVisible()
    await expect(
      security.getByRole('button', { name: 'Add invocation rule for transfer' }),
    ).toHaveCount(0)
    await security.screenshot({
      path: test.info().outputPath('upstream-security-readonly-desktop.png'),
    })
    await page.setViewportSize({ width: 390, height: 844 })
    await security.scrollIntoViewIfNeeded()
    await page.screenshot({ path: test.info().outputPath('upstream-security-readonly-mobile.png') })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.getByRole('button', { name: 'Edit upstream', exact: true }).click()
    await security.getByRole('button', { name: 'Add invocation rule for transfer' }).click()
    const modal = page.getByRole('dialog', { name: 'Invocation policy for transfer' })
    await modal.getByLabel('Automatic retry', { exact: true }).selectOption('once')
    await modal.getByLabel('Read-only classification', { exact: true }).selectOption('false')
    await modal.getByLabel('Accepted ACR values', { exact: true }).fill('hardware')
    await modal.getByLabel('Maximum authentication age (seconds)', { exact: true }).fill('60')
    await modal.getByRole('checkbox', { name: 'corporate' }).check()
    await page.setViewportSize({ width: 390, height: 844 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await expect(modal.getByText(/Step-up: hardware · max 60s · Approval: corporate/)).toBeVisible()
    await page.screenshot({ path: test.info().outputPath('upstream-invocation-mobile.png') })
    await modal.getByLabel('Automatic retry', { exact: true }).scrollIntoViewIfNeeded()
    await page.screenshot({ path: test.info().outputPath('execution-policy-mobile.png') })
    await page.setViewportSize({ width: 1280, height: 900 })
    await page.screenshot({ path: test.info().outputPath('execution-policy-desktop.png') })
    await page.setViewportSize({ width: 390, height: 844 })
    await withConfirmation(page, false, () =>
      modal.getByRole('button', { name: 'Cancel', exact: true }).click(),
    )
    await expect(modal.getByLabel('Accepted ACR values', { exact: true })).toHaveValue('hardware')
    await modal.getByRole('button', { name: 'Stage policy' }).click()
    expect(saves).toBe(0)
    await expect(
      security.getByText(/^off \(approval or session verification prevents replay\)/),
    ).toBeVisible()
    await expect(security.getByText('Review changes · 1 catalog tool affected')).toBeVisible()
    await security.getByText('Review changes · 1 catalog tool affected').scrollIntoViewIfNeeded()
    await expect(page.getByRole('button', { name: 'Save', exact: true })).toBeInViewport()
    await page.screenshot({ path: test.info().outputPath('upstream-security-draft-mobile.png') })
    await page.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Edit upstream', exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Edit upstream', exact: true }).click()
    await withConfirmation(page, true, () =>
      security.getByRole('button', { name: 'Remove invocation rule for transfer' }).click(),
    )
    await page.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Edit upstream', exact: true })).toBeVisible()
    expect(saves).toBe(2)
    expect(doc.policy.tools?.transfer).toBeUndefined()
    expect(doc.policy.stepUp?.maxAgeSeconds).toBe(300)
  })

test('shared service authentication editor supports OAuth, headers and none on mobile', async ({
  page,
}) => {
  await page.route('**/auth/config', (r) => r.fulfill({ json: { oidc: false } }))
  await page.route('**/api/v1/**', (route) => {
    const capability = configurationFixture(route, 'operator', true)
    if (capability) return capability
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/me'))
      return route.fulfill({
        status: route.request().headers().authorization ? 200 : 401,
        json: { name: 'Operator', role: 'admin' },
      })
    if (path.endsWith('/invocation-security/shared-services'))
      return route.fulfill({ json: { revision: 1, services: {}, inUse: [] } })
    return route.fulfill({ json: { items: [] } })
  })
  await page.goto('/invocation-security')
  await page.getByLabel('Admin token').fill('test')
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await page.getByRole('button', { name: 'Add service' }).click()
  const modal = page.getByRole('dialog')
  await modal.getByLabel('Service name', { exact: true }).fill('corporate')
  await modal
    .getByLabel('HTTPS decision endpoint', { exact: true })
    .fill('https://approval.test/check')
  await modal
    .getByLabel('Trusted approval screen origin', { exact: true })
    .fill('https://approval.test')
  await modal
    .getByLabel('Service authentication', { exact: true })
    .selectOption('oauth_client_credentials')
  await modal.getByLabel('OAuth token endpoint', { exact: true }).fill('https://idp.test/token')
  await modal.getByLabel('OAuth client ID', { exact: true }).fill('client')
  await modal.getByLabel('OAuth client secret', { exact: true }).fill('test-secret')
  await page.screenshot({ path: test.info().outputPath('service-oauth-desktop.png') })
  await page.setViewportSize({ width: 390, height: 844 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: test.info().outputPath('service-oauth-mobile.png') })
  await modal.getByLabel('Service authentication', { exact: true }).selectOption('headers')
  await modal.getByRole('button', { name: 'Add header' }).click()
  await modal.getByLabel('Header 1 name', { exact: true }).fill('X-API-Key')
  await modal.getByLabel('Header 1 value', { exact: true }).fill('test-key')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await modal.getByLabel('Service authentication', { exact: true }).selectOption('none')
  await modal.getByRole('button', { name: 'Stage service' }).click()
  await expect(page.getByText('No authentication', { exact: true })).toBeVisible()
})
