import {expect,test} from 'vitest'
import fixtures from './testdata/response-projection.json'
import {responseProjection,togglePath,displayPath,effectivePaths} from './response-projection'
for(const fixture of fixtures)test(`output schema matches Go projection fixture: ${fixture.name}`,()=>{
 const result=responseProjection(fixture.schema,[],fixture.paths)
 expect(result.error).toBe('');expect(result.schema).toEqual(fixture.expectedSchema)
})
test('literal property names stay distinct and parent-child paths cannot overlap',()=>{
 const schema=JSON.parse('{"type":"object","properties":{"a.b":{"type":"object","properties":{"/name":{"type":"string"},"__proto__":{"type":"integer"}}}}}')
 expect(responseProjection(schema,[],[['a.b','/name'],['a.b','__proto__']]).error).toBe('')
 expect(displayPath(schema,['a.b','/name'])).toBe('["a.b"]["/name"]')
 expect(responseProjection(schema,[],[['a.b'],['a.b','/name']]).error).toContain('overlap')
 expect(responseProjection(schema,['a.b'],[['a.b','/name']]).error).toContain('cannot both')
 expect(responseProjection(schema,[],[['missing']]).error).toContain('no longer declared')
 expect(({} as Record<string,unknown>).polluted).toBeUndefined()
})
test('narrowing a selected parent expands declared siblings without widening other selections',()=>{
 const schema=fixtures[0].schema
 const paths=togglePath(schema,[['order']],['order','items','price'],false)
 expect(paths).toEqual([['order','id'],['order','items','sku'],['order','secret']])
 expect(responseProjection(schema,[],paths).error).toBe('')
 expect(togglePath(schema,paths,['order','items'],true)).toEqual([['order','id'],['order','secret'],['order','items']])
 expect(effectivePaths(['a.b'])).toEqual([['a.b']])
})
test('root arrays and nested arrays display their implicit item traversal',()=>{
 expect(displayPath(fixtures[1].schema,['id'])).toBe('[][].id')
 expect(displayPath(fixtures[0].schema,['order','items','sku'])).toBe('order.items[].sku')
})
