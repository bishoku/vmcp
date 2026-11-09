import { expect, test } from '@playwright/test'
import { auditSettings } from './audit-fixtures'

for (const width of [390, 1440])
  for (const access of ['metadata', 'request', 'response'] as const)
    test(`audit ${access} visibility never downloads forbidden payloads at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 900 })
      const fetched: string[] = []
      page.on('request', (r) => {
        if (new URL(r.url()).pathname.endsWith('/payload')) fetched.push(r.url())
      })
      let revoked = false
      const events = () =>
        ['request', 'response'].map((direction, i) => ({
          id: i + 1,
          eventId: direction,
          operationId: 'operation',
          direction,
          stage: i ? 'outcome' : 'admission',
          status: i ? 'ok' : 'started',
          time: '2026-11-04T10:00:00Z',
          method: 'tools/call',
          tool: 'lookup',
          virtualServer: 'chat',
          scope: 'virtual',
          payloadState: 'captured',
          actorId: 'private-identity',
          clientIp: 'private-ip',
          reason: 'private-reason',
          target: 'private-target',
          capabilities: {
            canReadMetadata: true,
            canReadIdentities: false,
            canReadDetails: false,
            canReadPayload: !revoked && access === direction,
            canReadRequestPayload: !revoked && access === 'request' && direction === 'request',
            canReadResponsePayload: !revoked && access === 'response' && direction === 'response',
            authorizationRevision: revoked ? 2 : 1,
          },
        }))
      await page.route('**/auth/config', (r) => r.fulfill({ json: { oidc: false } }))
      await page.route('**/api/v1/**', (r) => {
        if (new URL(r.request().url()).pathname.endsWith('/configuration-access'))
          return r.fulfill({
            json: {
              authorizationRevision: 1,
              allowed: r
                .request()
                .postDataJSON()
                .checks.map(() => false),
            },
          })
        const path = new URL(r.request().url()).pathname
        // Admin without payload capabilities must be denied by the UI as well.
        if (path.endsWith('/me'))
          return r.fulfill({
            json: { name: 'Reviewer', role: access === 'metadata' ? 'admin' : 'viewer' },
          })
        if (path.endsWith('/audit-settings')) return r.fulfill({ json: auditSettings })
        if (path.endsWith('/audit-events'))
          return r.fulfill({
            json: { items: events(), capabilities: { canFilterIdentities: false } },
          })
        if (path.endsWith('/payload'))
          return r.fulfill({ json: { state: 'captured', content: { message: 'allowed-message' } } })
        return r.fulfill({ json: { items: [] } })
      })
      await page.goto('/audit')
      await page.getByRole('button', { name: 'Details', exact: true }).click()
      const dialog = page.getByRole('dialog', { name: 'Audit details' })
      await expect(
        dialog.getByRole('region', { name: 'Request payload', exact: true }),
      ).toBeVisible()
      expect(fetched).toHaveLength(0)
      for (const value of ['private-identity', 'private-ip', 'private-reason', 'private-target'])
        await expect(dialog).not.toContainText(value)
      if (access === 'metadata') {
        await expect(dialog.getByRole('button', { name: 'Load audit payload' })).toHaveCount(0)
      } else {
        const allowed = dialog.getByRole('region', {
          name: `${access === 'request' ? 'Request' : 'Response'} payload`,
          exact: true,
        })
        const denied = dialog.getByRole('region', {
          name: `${access === 'request' ? 'Response' : 'Request'} payload`,
          exact: true,
        })
        await expect(denied.getByRole('button', { name: 'Load audit payload' })).toHaveCount(0)
        await allowed.getByRole('button', { name: 'Load audit payload' }).click()
        await expect(allowed.locator('pre')).toContainText('allowed-message')
        expect(fetched).toHaveLength(1)
        expect(fetched[0]).toContain(`/${access}/payload`)
      }
      await page.keyboard.press('Escape')
      revoked = true
      const refreshed = page.waitForResponse(
        (r) => new URL(r.url()).pathname === '/api/v1/audit-events',
      )
      await page.getByRole('button', { name: 'Refresh', exact: true }).click()
      await refreshed
      await expect(page.getByRole('button', { name: 'Refresh', exact: true })).toBeEnabled()
      await page.getByRole('button', { name: 'Details', exact: true }).click()
      await expect(dialog.getByRole('button', { name: 'Load audit payload' })).toHaveCount(0)
      await expect(dialog).not.toContainText('allowed-message')
      expect(fetched).toHaveLength(access === 'metadata' ? 0 : 1)
    })

for (const width of [390, 1440])
  test(`custom claim session stays on audit and clears payload on claim expiry at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 })
    const forbidden: string[] = [],
      payloads: string[] = []
    await page.route('**/auth/config', (r) => r.fulfill({ json: { oidc: true } }))
    await page.route('**/api/v1/**', (r) => {
      if (new URL(r.request().url()).pathname.endsWith('/configuration-access'))
        return r.fulfill({
          json: {
            authorizationRevision: 1,
            allowed: r
              .request()
              .postDataJSON()
              .checks.map(() => false),
          },
        })
      const path = new URL(r.request().url()).pathname
      if (path.endsWith('/me'))
        return r.fulfill({
          json: {
            name: 'Claim Auditor',
            role: 'custom',
            via: 'oidc',
            claimsFresh: true,
            claimsExpire: new Date(Date.now() + 15 * 60 * 1000).toISOString(),
          },
        })
      if (path.endsWith('/audit-events'))
        return r.fulfill({
          json: {
            items: [
              {
                id: 1,
                eventId: 'request',
                operationId: 'operation',
                direction: 'request',
                stage: 'admission',
                status: 'ok',
                time: '2026-11-05T10:00:00Z',
                method: 'tools/call',
                scope: 'virtual',
                payloadState: 'captured',
                capabilities: {
                  canReadMetadata: true,
                  canReadPayload: true,
                  canReadRequestPayload: true,
                  canReadResponsePayload: false,
                  authorizationRevision: 1,
                },
              },
            ],
            capabilities: { canFilterIdentities: false },
          },
        })
      if (path.endsWith('/payload')) {
        payloads.push(path)
        return r.fulfill({
          json: { state: 'captured', content: { secret: 'claim-authorized-payload' } },
        })
      }
      forbidden.push(path)
      return r.fulfill({ status: 403, json: { detail: 'No configuration permission' } })
    })
    await page.goto('/audit')
    await expect(page).toHaveURL(/\/audit$/)
    await page.getByRole('button', { name: 'Details', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Audit details' })
    await dialog.getByRole('button', { name: 'Load audit payload' }).click()
    await expect(dialog.locator('pre').first()).toContainText('claim-authorized-payload')
    await page.evaluate(() => window.dispatchEvent(new Event('vmcp:claims-expired')))
    await expect(dialog).toHaveCount(0)
    await expect(page.locator('body')).not.toContainText('claim-authorized-payload')
    expect(payloads).toHaveLength(1)
    expect(forbidden).toHaveLength(0)
  })
