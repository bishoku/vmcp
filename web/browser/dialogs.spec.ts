import { expect, test } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.route('**/auth/config', r => r.fulfill({ json: { oidc: false } }))
  await page.route('**/api/v1/**', route => {
    const request = route.request(), path = new URL(request.url()).pathname
    if (path.endsWith('/me')) return route.fulfill({ status: request.headers().authorization ? 200 : 401, json: { name: 'Admin', role: 'admin', via: 'bootstrap-token' } })
    if (path.endsWith('/agent-keys') && request.method() === 'POST') {
      return route.fulfill({ json: { slug: 'test', key: 'one-time-secret' } })
    }
    return route.fulfill({ json: { items: [] } })
  })
  await page.goto('/')
  await page.getByRole('textbox').fill('admin-test')
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
})

test('upstream drawer has a name, contains keyboard focus, closes with Escape and restores focus', async ({ page }) => {
  await page.getByRole('link', { name: 'Upstreams', exact: true }).click()
  const trigger = page.getByRole('button', { name: 'Add upstream', exact: true })
  await trigger.click()
  const dialog = page.getByRole('dialog', { name: 'New upstream', exact: true })
  await expect(dialog).toBeVisible()
  const close = dialog.getByRole('button', { name: 'Close', exact: true })
  await expect(close).toBeFocused()
  await page.keyboard.press('Shift+Tab')
  await expect(dialog.getByRole('button', { name: 'Cancel', exact: true })).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(close).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(trigger).toBeFocused()
})

test('agent form supports Escape, focus restoration and backdrop dismissal', async ({ page }) => {
  await page.getByRole('link', { name: 'Agent Keys', exact: true }).click()
  const trigger = page.getByRole('button', { name: 'New key', exact: true })
  await trigger.click()
  const dialog = page.getByRole('dialog', { name: 'New agent key', exact: true })
  await expect(dialog.getByRole('button', { name: 'Close', exact: true })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(trigger).toBeFocused()
  await trigger.click()
  // Outside the centered card, on the full-screen overlay.
  await dialog.click({ position: { x: 5, y: 5 } })
  await expect(dialog).toHaveCount(0)
  await expect(trigger).toBeFocused()
})

test('one-time secrets require explicit acknowledgement and cannot be lost to Escape/backdrop', async ({ page }) => {
  await page.getByRole('link', { name: 'Agent Keys', exact: true }).click()
  await page.getByRole('button', { name: 'New key', exact: true }).click()
  await page.getByLabel('Name', { exact: true }).fill('test')
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Agent key created', exact: true })
  await expect(dialog.getByText('one-time-secret', { exact: true })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeVisible()
  await dialog.click({ position: { x: 5, y: 5 } })
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: 'I have saved it', exact: true }).click()
  await expect(dialog).toHaveCount(0)
})
