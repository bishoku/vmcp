import { expect, test } from '@playwright/test'

for(const sso of [false,true])test(`${sso?'SSO and token':'token-only'} sign-in handles errors keyboard submission and memory-only access`,async({page})=>{
 const attempts:string[]=[]
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:sso}}))
 await page.route('**/api/v1/**',route=>{
  const request=route.request(),path=new URL(request.url()).pathname
  if(path.endsWith('/me')){
   const auth=request.headers().authorization
   if(!auth)return route.fulfill({status:401,json:{}})
   attempts.push(auth)
   if(auth==='Bearer unavailable')return route.fulfill({status:503,json:{}})
   return route.fulfill({status:auth==='Bearer accepted'?200:401,json:{name:'Viewer',role:'viewer'}})
  }
  return route.fulfill({json:{items:[]}})
 })
 await page.goto('/connections')
 await expect(page.getByRole('heading',{name:'Sign in to vMCP',exact:true})).toBeVisible()
 if(sso)await expect(page.getByRole('link',{name:'Sign in with SSO',exact:true})).toHaveAttribute('href','/auth/login')
 else await expect(page.getByRole('link',{name:'Sign in with SSO',exact:true})).toHaveCount(0)
 await page.screenshot({path:test.info().outputPath(sso?'sign-in-sso.png':'sign-in-token.png'),animations:'disabled'})
 const token=page.getByLabel('Admin token',{exact:true})
 await token.fill('   ');await token.press('Enter')
 await expect(page.getByRole('alert')).toContainText('Admin token is required.')
 expect(attempts).toEqual([])
 await token.fill('rejected')
 await page.getByRole('button',{name:'Show token',exact:true}).click()
 await expect(token).toHaveAttribute('type','text')
 await page.getByRole('button',{name:'Hide token',exact:true}).click()
 await token.press('Enter')
 await expect(page.getByRole('alert')).toContainText('This token was not accepted')
 await expect(token).toHaveAttribute('aria-invalid','true')
 await page.screenshot({path:test.info().outputPath(sso?'sign-in-sso-error.png':'sign-in-token-error.png'),animations:'disabled'})
 await token.fill('unavailable');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await expect(page.getByRole('alert')).toContainText('Sign-in is unavailable')
 await page.setViewportSize({width:320,height:568})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath(sso?'sign-in-sso-mobile.png':'sign-in-token-mobile.png'),animations:'disabled'})
 await token.fill('accepted');await token.press('Enter')
 await expect(page.getByRole('heading',{name:'Connections',exact:true})).toBeVisible()
 expect(attempts).toEqual(['Bearer rejected','Bearer unavailable','Bearer accepted'])
 expect(await page.evaluate(()=>Object.keys(localStorage))).toEqual([])
 expect(await page.evaluate(()=>Object.keys(sessionStorage))).toEqual([])
 await page.reload();await expect(token).toHaveValue('')
})

test('session recovery retains a draft after failed reauthentication and never replays a write',async({page})=>{
 let writes=0
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:true}}))
 await page.route('**/api/v1/**',route=>{
  const request=route.request(),path=new URL(request.url()).pathname
  if(path.endsWith('/me'))return route.fulfill({status:['Bearer accepted','Bearer replacement'].includes(request.headers().authorization)?200:401,json:{name:'Editor',role:'editor'}})
  if(path.endsWith('/agent-keys')&&request.method()==='POST'){writes++;return route.fulfill({status:401,json:{detail:'Expired'}})}
  return route.fulfill({json:{items:[]}})
 })
 await page.goto('/agent-keys');await page.getByLabel('Admin token',{exact:true}).fill('accepted');await page.getByRole('button',{name:'Continue',exact:true}).click()
 await page.getByRole('button',{name:'New key',exact:true}).click()
 await page.getByLabel('Name',{exact:true}).fill('retained-draft')
 await page.getByRole('button',{name:'Create',exact:true}).click()
 const expired=page.getByRole('dialog',{name:'Session expired',exact:true})
 await expect(expired.getByRole('region',{name:'Continue your work',exact:true})).toBeVisible()
 await expect(expired.getByRole('link',{name:'Sign in with SSO',exact:true})).toHaveAttribute('href','/auth/login')
 await page.screenshot({path:test.info().outputPath('session-recovery.png'),animations:'disabled'})
 await page.keyboard.press('Escape');await expect(expired).toBeVisible()
 await expired.getByLabel('Admin token',{exact:true}).fill('rejected')
 await expired.getByRole('button',{name:'Continue',exact:true}).click()
 await expect(expired.getByRole('alert')).toBeVisible()
 expect(writes).toBe(1)
 await page.setViewportSize({width:390,height:844})
 expect(await expired.locator('section').first().evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
 await expired.getByRole('region',{name:'Restore access',exact:true}).evaluate(el=>el.scrollIntoView({block:'start'}))
 await page.screenshot({path:test.info().outputPath('session-recovery-mobile.png'),animations:'disabled'})
 await expired.getByLabel('Admin token',{exact:true}).fill('replacement');await expired.getByRole('button',{name:'Continue',exact:true}).click()
 await expect(expired).toHaveCount(0)
 await expect(page.getByLabel('Name',{exact:true})).toHaveValue('retained-draft')
 expect(writes).toBe(1)
})
