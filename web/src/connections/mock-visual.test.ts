import {expect,test} from 'vitest'
import {bodyPaths,conditions,isFallback,moveRule,putCondition,removeCondition} from './mock-visual'
import type {MockRule} from '../api'

test('visual changes preserve advanced body and header matchers exactly',()=>{
 const request={headers:{'X-Complex':{and:[{contains:'a'},{matches:'.+'}]}},bodyPatterns:[{equalToJson:{items:[{id:1}]},ignoreArrayOrder:true}]}
 const changed=putCondition(request,{source:'queryParameters',field:'type',operator:'equalTo',value:'1'})!
 expect(changed.headers).toEqual(request.headers);expect(changed.bodyPatterns).toEqual(request.bodyPatterns)
 const entries=conditions(changed)
 expect(entries.filter(e=>!e.condition)).toHaveLength(2)
 expect(removeCondition(changed,entries.find(e=>e.source==='queryParameters')!)).toEqual({...request,queryParameters:{}})
 expect(request).not.toHaveProperty('queryParameters')
})
test('conditions support present, absent, regex and editing without duplicate overwrite',()=>{
 const request=putCondition(undefined,{source:'headers',field:'X-Mode',operator:'present',value:''})
 expect(request).toEqual({headers:{'X-Mode':{absent:false}}})
 expect(()=>putCondition(request,{source:'headers',field:'x-mode',operator:'absent',value:''})).toThrow('already')
 const entry=conditions(request)[0]
 const changed=putCondition(request,{source:'headers',field:'X-Test',operator:'matches',value:'^ok$'},entry)
 expect(changed).toEqual({headers:{'X-Test':{matches:'^ok$'}}})
 expect(conditions(changed)[0].condition?.operator).toBe('matches')
})
test('nested schema paths escape keys and include arrays; any item compares typed values',()=>{
 const schema={type:'object',properties:{order:{type:'object',properties:{items:{type:'array',items:{type:'object',properties:{'product.id':{type:'string'},quantity:{type:'integer'}}}}}}}}
 const paths=bodyPaths(schema)
 expect(paths).toContain('$["order"]["items"][*]["product.id"]')
 const result=putCondition(undefined,{source:'bodyPatterns',field:'$["order"]["items"][*]["quantity"]',operator:'anyEqualTo',value:'2'})
 expect(result).toEqual({bodyPatterns:[{matchesJsonPath:'$["order"]["items"][?(@["quantity"] == 2)]'}]})
 const escaped=putCondition(undefined,{source:'bodyPatterns',field:'$.items[*].id',operator:'anyEqualTo',value:'SKU"X'})
 expect(escaped).toEqual({bodyPatterns:[{matchesJsonPath:'$.items[?(@.id == "SKU\\"X")]'}]})
 expect(()=>putCondition(undefined,{source:'bodyPatterns',field:'$.items[*].id',operator:'equalTo',value:'SKU'})).toThrow('wildcard')
})
test('moving rules normalizes priorities in actual matching order and fallback is explicit',()=>{
 const rules:MockRule[]=[{name:'fallback',priority:10,response:{status:200}},{name:'conditional',priority:1,request:{queryParameters:{id:{absent:false}}},response:{status:201}}]
 expect(isFallback(rules[0])).toBe(true);expect(isFallback(rules[1])).toBe(false)
 expect(moveRule(rules,0,-1).map(r=>[r.name,r.priority])).toEqual([['fallback',1],['conditional',2]])
 expect(rules[0].priority).toBe(10)
})

import fixtures from './mock-visual-fixtures.json'
import type {Condition} from './mock-visual'
test.each(fixtures)('generated matcher contract for real WireMock: $name',fixture=>{
 expect(putCondition(undefined,fixture.condition as Condition)).toEqual(fixture.request)
})
