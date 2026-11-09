import { expect, test } from '@playwright/test'
import { upstream, server } from '../src/connections/fixtures'

for (const family of ['upstreams', 'virtual-servers'])
  for (const writable of [true, false])
    test(`local ${family} writable=${writable} never downloads global or foreign settings`, async ({
      page,
    }) => {
      const slug = family === 'upstreams' ? 'alpha' : 'demo',
        localPath = `/${family}/${slug}/audit-settings`
      const calls: { method: string; path: string; body: any }[] = []
      let rules = [{ name: 'local', pattern: 'LOCAL-[0-9]+' }],
        revision = 5
      await page.route('**/auth/config', (route) => route.fulfill({ json: { oidc: true } }))
      await page.route('**/api/v1/**', (route) => {
        const request = route.request(),
          path = new URL(request.url()).pathname.replace('/api/v1', ''),
          method = request.method(),
          body = request.postDataJSON()
        if (path === '/me')
          return route.fulfill({
            json: { name: 'Local audit manager', role: 'custom', via: 'oidc' },
          })
        if (path === '/configuration-access')
          return route.fulfill({
            json: {
              authorizationRevision: 1,
              allowed: body.checks.map((check: any) => {
                const target = check.path.split('?')[0]
                if (target === localPath || target === localPath + '/preview')
                  return !check.permission && (check.method === 'GET' || writable)
                if (target === `/${family}/${slug}` || target === `/${family}`)
                  return (
                    check.method === 'GET' &&
                    (!check.permission || check.permission.endsWith('.read'))
                  )
                return false
              }),
            },
          })
        calls.push({ method, path, body })
        if (path === localPath) {
          if (method === 'PUT') {
            rules = body.rules
            revision++
          }
          return route.fulfill({ json: { rules, revision } })
        }
        if (path === localPath + '/preview')
          return route.fulfill({
            json: {
              state: 'redacted',
              content: { text: '[REDACTED]' },
              matches: [],
              redactions: 1,
              originalBytes: 20,
              storedBytes: 20,
              revision,
            },
          })
        if (path === '/upstreams/alpha') return route.fulfill({ json: upstream })
        if (path === '/virtual-servers/demo') return route.fulfill({ json: server })
        if (path === `/${family}`)
          return route.fulfill({ json: { items: [family === 'upstreams' ? upstream : server] } })
        return route.fulfill({ status: 403, json: { detail: 'Unexpected data download' } })
      })
      await page.goto(
        family === 'upstreams' ? '/upstreams/alpha' : '/virtual-servers/demo/protection',
      )
      if (family === 'upstreams') {
        await page.setViewportSize({ width: 390, height: 844 })
        await page.locator('summary').filter({ hasText: 'Audit payload masking' }).click()
        await expect(page.getByText('LOCAL-[0-9]+', { exact: true })).toBeVisible()
        await expect(
          page.getByRole('button', { name: 'Add masking rule', exact: true }),
        ).toHaveCount(0)
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        )
        if (writable) {
          await page.getByRole('button', { name: 'Edit upstream', exact: true }).click()
          await expect(page.getByLabel('URL', { exact: true })).toBeDisabled()
          await expect(page.getByLabel('Name', { exact: true })).toBeDisabled()
        }
      }
      if (!writable) {
        if (family === 'upstreams') {
          await expect(
            page.getByRole('button', { name: 'Edit upstream', exact: true }),
          ).toHaveCount(0)
          expect(
            calls.some((call) => call.method !== 'GET' || call.path === '/audit-settings'),
          ).toBe(false)
          return
        }
        await expect(page.getByLabel('Audit rule 1 regex', { exact: true })).toBeDisabled()
        await expect(
          page.getByRole('button', { name: 'Save audit settings', exact: true }),
        ).toBeDisabled()
        expect(calls.some((call) => call.method !== 'GET' || call.path === '/audit-settings')).toBe(
          false,
        )
        return
      }
      await page.getByLabel('Audit rule 1 regex', { exact: true }).fill('LOCAL-[A-Z]+')
      if (family === 'upstreams') {
        await page
          .getByRole('region', { name: 'Audit payload masking', exact: true })
          .scrollIntoViewIfNeeded()
        await expect(page.getByRole('button', { name: 'Save', exact: true })).toBeInViewport()
        await page.screenshot({ path: test.info().outputPath('upstream-audit-draft-mobile.png') })
      }
      await page
        .getByRole('button', {
          name: family === 'upstreams' ? 'Save' : 'Save audit settings',
          exact: true,
        })
        .click()
      await expect.poll(() => calls.filter((call) => call.method === 'PUT').length).toBe(1)
      expect(calls.find((call) => call.method === 'PUT')?.body).toEqual({
        revision: 5,
        rules: [{ name: 'local', pattern: 'LOCAL-[A-Z]+' }],
      })
      if (family === 'upstreams')
        await page.getByRole('button', { name: 'Edit upstream', exact: true }).click()
      await page.getByText('Test request / response audit payload', { exact: true }).click()
      await page.getByRole('button', { name: 'Preview audit payloads', exact: true }).click()
      await expect.poll(() => calls.filter((call) => call.path.endsWith('/preview')).length).toBe(2)
      for (const call of calls.filter((call) => call.path.endsWith('/preview'))) {
        expect(call.body).not.toHaveProperty('settings')
        expect(call.body).not.toHaveProperty('upstream')
        expect(call.body).not.toHaveProperty('virtualServer')
      }
      expect(
        calls.some((call) => call.path === '/audit-settings' || call.path === '/audit-preview'),
      ).toBe(false)
      expect(
        calls.every(
          (call) =>
            call.path === localPath ||
            call.path === localPath + '/preview' ||
            call.path === `/${family}` ||
            call.path === `/${family}/${slug}`,
        ),
      ).toBe(true)
    })
