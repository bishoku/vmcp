// Run against the isolated --security-demo stack, with fresh Keycloak users.
// Optional VMCP_DEMO_OTP_FILE persists test-only OTP seeds across runs (mode 0600).
import { chromium } from 'playwright-core'
import { createHmac } from 'node:crypto'
import fs from 'node:fs'
import assert from 'node:assert/strict'
const base = process.env.VMCP_DEMO_URL || 'https://localhost:8844'
const seedFile = process.env.VMCP_DEMO_OTP_FILE
const seeds =
  seedFile && fs.existsSync(seedFile) ? JSON.parse(fs.readFileSync(seedFile, 'utf8')) : {}
// Keycloak's hidden totpSecret is the raw secret; the UI encodes it for QR/manual setup.
function otp(seed) {
  const b = Buffer.alloc(8)
  b.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 30000)))
  const h = createHmac('sha1', Buffer.from(seed, 'utf8')).update(b).digest(),
    offset = h[19] & 15
  return ((h.readUInt32BE(offset) & 0x7fffffff) % 1000000).toString().padStart(6, '0')
}

const browser = await chromium.launch({ headless: true })
const context = await browser.newContext({ ignoreHTTPSErrors: true })
context.setDefaultTimeout(15000)
// Keep transport diagnostics free of OAuth codes, tokens and customer sessions.
context.on('requestfailed', (request) =>
  console.log('TRANSPORT_DIAGNOSTIC', {
    method: request.method(),
    path: new URL(request.url()).pathname,
    error: request.failure()?.errorText,
  }),
)
async function openDemo(page) {
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      await page.goto(base)
      return
    } catch (error) {
      if (attempt === 2 || !String(error).includes('net::ERR_NETWORK_CHANGED')) throw error
      await page.waitForTimeout(1000)
    }
  }
}
async function signIn(page, user = 'alice') {
  for (let i = 0; i < 100; i++) {
    if (new URL(page.url()).origin === base) {
      await page.waitForTimeout(300)
      return
    }
    if (await page.locator('#username').isVisible()) await page.locator('#username').fill(user)
    if (await page.locator('#password').isVisible()) {
      await page.locator('#password').fill('demo-password')
      await page.locator('#kc-login').click()
    } else if (await page.locator('#totpSecret').count()) {
      seeds[user] = await page.locator('#totpSecret').inputValue()
      if (seedFile) fs.writeFileSync(seedFile, JSON.stringify(seeds), { mode: 0o600 })
      await page.locator('#totp').fill(otp(seeds[user]))
      await page.locator('#userLabel').fill('Protocol lab test')
      await page.locator('#saveTOTPBtn').click()
    } else if (await page.locator('#otp').isVisible()) {
      assert(seeds[user], 'OTP already enrolled: use a fresh demo realm or VMCP_DEMO_OTP_FILE')
      await page.locator('#otp').fill(otp(seeds[user]))
      await page.locator('#kc-login').click()
    }
    await page.waitForTimeout(200)
  }
  console.log('LOGIN_DIAGNOSTIC', {
    url: new URL(page.url()).pathname,
    inputs: await page
      .locator('input')
      .evaluateAll((xs) => xs.map((x) => ({ id: x.id, type: x.type }))),
    alerts: await page.locator('[role=alert],.kc-feedback-text,#input-error').allTextContents(),
  })
  throw Error('Login did not finish')
}
async function login(page, user = 'alice') {
  await page.locator('#login').click()
  await page.waitForURL((u) => u.origin !== base)
  await signIn(page, user)
  await page.waitForFunction(() =>
    document.querySelector('#identity')?.textContent.includes('subject'),
  )
}
async function init(page) {
  await page.locator('#clear').click()
  await page.locator('#connect').click()
  await page.waitForFunction(() =>
    document.querySelector('#log').textContent.includes('serverInfo'),
  )
}
async function send(page, tool, args = {}, fresh = true) {
  console.log('Testing', tool, fresh ? 'new' : 'retry')
  if (fresh) await page.locator('#new').click()
  await page.locator('#params').fill(JSON.stringify({ name: tool, arguments: args }))
  await page.locator('#clear').click()
  await page.locator('#send').click()
}
async function response(page, needle) {
  try {
    await page.waitForFunction(
      (n) =>
        document.querySelector('#log').textContent.startsWith('Response') &&
        document.querySelector('#log').textContent.includes(n),
      needle,
    )
  } catch (e) {
    console.log('RESPONSE_DIAGNOSTIC', await page.locator('#log').innerText())
    throw e
  }
}
async function review(page, decision = 'approve') {
  const href = await page.locator('#approval a').getAttribute('href')
  const approval = await context.newPage()
  await approval.goto(href)
  await approval.locator('#login').click()
  await approval.waitForTimeout(600)
  await signIn(approval)
  await approval.locator('#' + decision).waitFor({ state: 'visible' })
  await approval.locator('#' + decision).click()
  await approval.waitForFunction(() =>
    document.querySelector('#status').textContent.includes('Decision saved'),
  )
  await approval.close()
}
try {
  for (const mode of ['oauth_subject', 'caller_context']) {
    const page = await context.newPage()
    await openDemo(page)
    if (mode === 'caller_context') {
      await page.locator('#mode').selectOption(mode)
      await page.waitForFunction(
        () => document.querySelector('#server').value === 'security-context',
      )
    }
    await login(page)
    await init(page)
    const prefix = mode === 'caller_context' ? 'security-context__' : 'security-demo__'
    await page.locator('#tools').click()
    await response(page, 'delete_records')
    await page.locator('#clear').click()
    await page.locator('#resources').click()
    await response(page, '"resources"')
    const resources = JSON.parse((await page.locator('#log').innerText()).split('\n\n')[0].slice(9))
      .result.resources
    assert(resources.length > 0, 'Seeded resource missing')
    await page.locator('#new').click()
    await page.locator('#method').selectOption('resources/read')
    await page.locator('#params').fill(JSON.stringify({ uri: resources[0].uri }))
    await page.locator('#clear').click()
    await page.locator('#send').click()
    await response(page, '"contents"')
    await page.locator('#clear').click()
    await page.locator('#prompts').click()
    await response(page, '"prompts"')
    const prompts = JSON.parse((await page.locator('#log').innerText()).split('\n\n')[0].slice(9))
      .result.prompts
    assert(prompts.length > 0, 'Seeded prompt missing')
    for (const [method, params, expected] of [
      ['prompts/get', { name: prompts[0].name, arguments: { operation: 'demo' } }, '"messages"'],
      [
        'completion/complete',
        {
          ref: { type: 'ref/prompt', name: prompts[0].name },
          argument: { name: 'operation', value: 'd' },
        },
        '"completion"',
      ],
    ]) {
      await page.locator('#new').click()
      await page.locator('#method').selectOption(method)
      await page.locator('#params').fill(JSON.stringify(params))
      await page.locator('#clear').click()
      await page.locator('#send').click()
      await response(page, expected)
    }
    await page.locator('#method').selectOption('tools/call')

    await send(page, prefix + 'echo', { dataset: 'demo', count: 1 })
    await response(page, '"result"')
    await send(page, prefix + 'delete_records', { dataset: 'demo', count: 5 })
    await response(page, 'pending')
    await review(page)
    await send(page, prefix + 'delete_records', { dataset: 'demo', count: 5 }, false)
    await response(page, '"result"')
    await send(page, prefix + 'delete_records', { dataset: 'demo', count: 5 }, false)
    await response(page, 'denied')
    await send(page, prefix + 'delete_records', { dataset: 'demo', count: 5 })
    await response(page, 'pending')
    await review(page)
    await send(page, prefix + 'delete_records', { dataset: 'demo', count: 6 }, false)
    await response(page, 'denied')
    await send(page, prefix + 'delete_records', { dataset: 'demo', count: 5 })
    await response(page, 'pending')
    await review(page, 'deny')
    await send(page, prefix + 'delete_records', { dataset: 'demo', count: 5 }, false)
    await response(page, 'denied')
    await send(page, prefix + 'step_up')
    if (mode === 'oauth_subject') {
      try {
        await page.waitForURL((u) => u.origin !== base)
      } catch (e) {
        console.log('STEPUP_DIAGNOSTIC', await page.locator('#log').innerText())
        throw e
      }
      await signIn(page)
      await page.waitForFunction(() =>
        document.querySelector('#identity')?.textContent.includes('"acr": "2"'),
      )
      await init(page)
    } else {
      await response(page, 'step_up_required')
      const href = await page.locator('#approval a').getAttribute('href')
      assert(!href.includes('session='), 'session credential leaked in challenge')
      const verify = await context.newPage()
      await verify.goto(href)
      await verify.locator('#verify').click()
      await verify.waitForURL((u) => u.origin !== base)
      await signIn(verify)
      await verify.waitForFunction(() =>
        document.querySelector('#status').textContent.includes('Customer session verified'),
      )
      await verify.close()
    }
    await send(page, prefix + 'step_up', {}, false)
    await response(page, '"result"')
    await send(page, prefix + 'export_records', { dataset: 'demo', count: 2 })
    await response(page, 'pending')
    await review(page)
    await send(page, prefix + 'export_records', { dataset: 'demo', count: 2 }, false)
    await response(page, '"result"')
    if (mode === 'caller_context') {
      await send(page, prefix + 'delete_records', { dataset: 'demo', count: 5 })
      await response(page, 'pending')
      const aliceApproval = await page.locator('#approval a').getAttribute('href')
      const aliceSession = await page.evaluate(() =>
        JSON.parse(sessionStorage.getItem('demo-customer-session')),
      )
      const other = await browser.newContext({ ignoreHTTPSErrors: true })
      other.setDefaultTimeout(15000)
      const bob = await other.newPage()
      await bob.goto(base)
      await bob.locator('#mode').selectOption('caller_context')
      await bob.waitForFunction(
        () => document.querySelector('#server').value === 'security-context',
      )
      await login(bob, 'bob')
      await init(bob)
      await send(bob, prefix + 'echo', { dataset: 'demo', count: 1 })
      await response(bob, '"result"')
      const mismatch = await bob.evaluate(async (customer) => {
        const r = await fetch('/application-gateway/mcp/v1/virtual/security-context', {
          method: 'POST',
          headers: {
            Authorization: 'Bearer ' + sessionStorage.getItem('demo-customer-token'),
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            jsonrpc: '2.0',
            id: 99,
            method: 'tools/call',
            params: {
              name: 'security-context__echo',
              arguments: {},
              _meta: {
                'demo/customer': { customerId: customer.customerId, sessionId: customer.sessionId },
              },
            },
          }),
        })
        return r.status
      }, aliceSession)
      assert.equal(mismatch, 403, 'Application must reject another customer session')
      const wrongReview = await other.newPage()
      await wrongReview.goto(aliceApproval)
      await wrongReview.locator('#login').click()
      await wrongReview.waitForTimeout(600)
      await signIn(wrongReview, 'bob')
      await wrongReview.waitForFunction(() =>
        document.querySelector('#status').textContent.includes('unavailable'),
      )
      await other.close()
      console.log(
        'Shared application: Bob works independently, cannot use Alice session or approval',
      )
    }
    console.log(
      mode +
        ': discovery, execution, approval, rejection, changed arguments, replay, real OTP step-up and combined policy passed',
    )
    await page.close()
  }
} finally {
  await browser.close()
}
