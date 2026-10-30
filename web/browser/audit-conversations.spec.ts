import {expect,test} from '@playwright/test'
import {auditSettings} from './audit-fixtures'
for(const width of [390,1440])test(`conversation audit aggregates and drills down at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900})
 const queries:URLSearchParams[]=[]
 await page.route('**/auth/config',r=>r.fulfill({json:{oidc:false}}))
 await page.route('**/api/v1/**',r=>{
 const u=new URL(r.request().url())
 if(u.pathname.endsWith('/me'))return r.fulfill({json:{name:'Viewer',role:'viewer'}})
 if(u.pathname.endsWith('/audit-settings'))return r.fulfill({json:auditSettings})
 if(u.pathname.endsWith('/audit-events')){queries.push(u.searchParams);if(u.searchParams.get('groupBy')==='conversation')return r.fulfill({json:{items:[{conversationGroupId:'group-a',rootConversationId:'root-chat',actorId:'key-a',firstTime:'2026-10-30T10:00:00Z',lastTime:'2026-10-30T10:01:00Z',lastId:100,records:600,operations:150,errors:0,conversations:3}],nextCursor:null}})
 return r.fulfill({json:{items:[],nextCursor:null}})}
 return r.fulfill({json:{items:[]}})
 })
 await page.goto('/audit?activity=omni')
 await page.getByRole('button',{name:'Conversations',exact:true}).click()
 await expect(page.getByRole('heading',{name:'root-chat',exact:true})).toBeVisible()
 await expect(page.getByText('150 root operations · 0 errors/denials · 3 conversations · 600 records',{exact:true})).toBeVisible()
 expect(queries.at(-1)?.get('activity')).toBe('omni')
 expect(queries.at(-1)?.get('groupBy')).toBe('conversation')
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:test.info().outputPath(`conversation-${width}.png`),animations:'disabled'})
 await page.getByRole('button',{name:'View operations',exact:true}).click()
 await expect.poll(()=>queries.at(-1)?.get('conversationGroupId')).toBe('group-a')
 await expect(page.getByRole('button',{name:'Remove Conversation group ID filter',exact:true})).toBeVisible()
})
