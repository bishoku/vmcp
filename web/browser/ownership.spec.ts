import {expect,test} from '@playwright/test'
for(const width of [390,1440])test(`team project and tool approval workflow ${width}`,async({page})=>{
 await page.setViewportSize({width,height:1000})
 const teams:Record<string,unknown>[]=[],projects:Record<string,unknown>[]=[{slug:'default',name:'Default',ownerTeam:'',version:1}],grants:Record<string,unknown>[]=[]
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',r=>{const req=r.request(),path=new URL(req.url()).pathname
 if(path.endsWith('/me'))return r.fulfill({json:{role:'admin',name:'Platform admin'}})
 const items=path.endsWith('/teams')?teams:path.endsWith('/projects')?projects:path.endsWith('/tool-grants')?grants:null
 if(items){if(req.method()==='POST'){const v=req.postDataJSON(),i=items.findIndex(old=>v.slug?old.slug===v.slug:old.virtualServer===v.virtualServer&&old.tool===v.tool);if(i>=0)items[i]={...v,version:Number(v.version)+1};else items.push({...v,version:1});return r.fulfill({json:{ok:true}})}return r.fulfill({json:{items}})}
 if(path.endsWith('/virtual-servers'))return r.fulfill({json:{items:[{slug:'chat',name:'Customer chat'}]}})
 if(path.endsWith('/tool-catalog'))return r.fulfill({json:{items:[{upstream:'crm',ownerTeam:'crm',name:'get_customer',description:'Read customer'}]}})
 return r.fulfill({json:{items:[]}})
 })
 await page.goto('/ownership')
 const ts=page.locator('section').filter({has:page.getByRole('heading',{name:'Teams',exact:true})})
 await ts.getByLabel('Slug',{exact:true}).fill('crm');await ts.getByLabel('Name',{exact:true}).fill('CRM owners');await ts.getByLabel('Issuer',{exact:true}).fill('https://idp.example');await ts.getByLabel('Group',{exact:true}).fill('crm-owners')
 await ts.getByRole('button',{name:'Save team',exact:true}).click();await expect.poll(()=>teams.length).toBe(1)
 // Refresh mounts the owner choices from the persisted team endpoint.
 await page.reload()
 const ps=page.locator('section').filter({has:page.getByRole('heading',{name:'Projects',exact:true})})
 await ps.getByLabel('Slug',{exact:true}).fill('chat');await ps.getByLabel('Name',{exact:true}).fill('Customer chat');await ps.getByLabel('Owner team').selectOption('crm');await ps.getByRole('button',{name:'Save project',exact:true}).click();await expect.poll(()=>projects.length).toBe(2)
 await page.getByRole('combobox',{name:'Virtual server',exact:true}).selectOption('chat');await page.getByRole('combobox',{name:'Tool',exact:true}).selectOption('crm/get_customer');await page.getByLabel('Reason',{exact:true}).fill('Customer support')
 await page.getByRole('button',{name:'Request access',exact:true}).click();await expect.poll(()=>grants[0]?.status).toBe('pending')
 await page.getByRole('button',{name:'Approve',exact:true}).click();await expect.poll(()=>grants[0]?.status).toBe('approved')
 await page.getByRole('button',{name:'Revoke',exact:true}).click();await expect.poll(()=>grants[0]?.status).toBe('revoked')
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath(`ownership-${width}.png`),fullPage:true})
})
