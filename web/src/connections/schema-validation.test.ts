import {expect,test} from 'vitest'
import {checkSchema,schemaLimitError} from './schema-validation'
test('reports schema and example mismatch with a field path',async()=>{
 expect(await checkSchema({type:'object',properties:{items:{type:'array',items:{type:'object',properties:{quantity:{type:'integer'}}}}}},{value:{items:[{quantity:1.5}]}})).toEqual(['/items/0/quantity: must be integer'])
 expect(await checkSchema({type:'object',required:['id']},{value:{}})).toEqual(['/: must have required property \'id\' (id)'])
 expect(await checkSchema({type:'object',properties:{id:{type:['integer','null']}}},{value:{id:null}})).toEqual([])
})
test('rejects oversized/invalid/referenced schemas before applying',async()=>{
 expect(schemaLimitError({description:'x'.repeat(65536)})).toContain('64 KiB')
 expect(await checkSchema({type:'not-a-type'})).not.toEqual([])
 expect(await checkSchema({properties:{id:{$ref:'https://example.com/schema'}}})).toEqual(['Schema references are not supported. Use expanded inline schemas.'])
 expect(await checkSchema(undefined)).toEqual([])
})
