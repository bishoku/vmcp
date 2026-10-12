// Opt-in browser regression: running generator + web's Playwright installation.
const {chromium} = require('../../web/node_modules/@playwright/test');
const assert = require('node:assert/strict');
(async () => {
  const browser = await chromium.launch({headless:true});
  try {
    const page = await browser.newPage();
    let started;
    await page.route('**/api/**', async route => {
      const path = new URL(route.request().url()).pathname;
      const payload = path === '/api/servers' ? [{slug:'demo',name:'Demo server'}]
        : path === '/api/tools' ? [{name:'sum',description:'Add numbers',inputSchema:{type:'object'}}]
        : path === '/api/status' ? {state:started?'running':'idle'} : {};
      if(path === '/api/start') started = route.request().postDataJSON();
      await route.fulfill({json:payload});
    });
    await page.goto('http://localhost:3302');
    await page.locator('#server').selectOption('demo');
    await page.locator('#key').fill('browser-test-key');
    assert.equal(await page.locator('#key').getAttribute('type'),'password');
    await page.getByRole('button',{name:'Discover tools'}).click();
    await page.locator('#tool').selectOption('sum');
    await page.locator('#arguments').fill('{"a":4,"b":7}');
    await page.getByRole('button',{name:'Start traffic'}).click();
    await page.getByRole('status').filter({hasText:'Traffic started'}).waitFor();
    assert.equal(started.tool,'sum');
    assert.deepEqual(started.arguments,{a:4,b:7});
    assert.equal(await page.locator('#key').inputValue(),'');
    await page.getByRole('button',{name:'Stop',exact:true}).click();
    await page.getByRole('status').filter({hasText:'Stopping new requests'}).waitFor();
    console.log('Traffic browser regression passed');
  } finally {await browser.close();}
})().catch(error => {console.error(error);process.exitCode=1;});
