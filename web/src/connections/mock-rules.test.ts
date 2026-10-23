import {expect,test} from 'vitest'
import {parseMockRules} from './mock-rules'
const rule={name:'ok',priority:1,response:{status:200,jsonBody:{items:[{id:1}]}}}
test('WireMock rules preserve nested matchers and response templates',()=>{
 const value={...rule,request:{bodyPatterns:[{matchesJsonPath:'$.order.items[0].productId'}]},response:{status:200,jsonBody:{sku:"{{jsonPath request.body '$.order.items[0].productId'}}"},transformers:['response-template']}}
 expect(parseMockRules(JSON.stringify([value]))).toEqual([value])
})
test('managed routes, global settings and unsafe response providers cannot be overridden',()=>{
 for(const value of [{...rule,id:'arbitrary'},{...rule,priority:0},{...rule,request:{urlPattern:'.*'}},{...rule,response:{status:200,proxyBaseUrl:'https://example.com'}},{...rule,response:{status:200,bodyFileName:'secret'}},{...rule,response:{status:200,transformers:['custom']}},{...rule,response:{status:200,fixedDelayMilliseconds:120001}},{...rule,request:[]}])expect(()=>parseMockRules(JSON.stringify([value]))).toThrow()
 expect(()=>parseMockRules(JSON.stringify([rule,rule]))).toThrow('unique')
})
