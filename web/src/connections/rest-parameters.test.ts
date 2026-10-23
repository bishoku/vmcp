import {expect,test} from 'vitest'
import {hasParameterDefault,parameterValueError,parseParameterDefault,type RESTParameter} from './rest-parameters'
const parameter:RESTParameter={name:'type',in:'query',required:false,schema:{type:'integer'}}
test('absence is distinct from zero, false and empty values',()=>{
 expect(hasParameterDefault(parameter)).toBe(false)
 for(const value of [0,false,''])expect(hasParameterDefault({...parameter,defaultValue:value})).toBe(true)
 expect(parameterValueError({...parameter,exposeToMcp:false})).toContain('fixed value')
 expect(parameterValueError({...parameter,defaultValue:0})).toBe('')
 expect(parameterValueError({...parameter,defaultValue:'0'})).toContain('type')
 expect(parameterValueError({...parameter,in:'path',schema:{type:'string'},defaultValue:'../x'})).toContain('unsafe')
})
test('numeric editing keeps incomplete and imprecise input invalid',()=>{
 expect(parseParameterDefault('0','integer')).toBe(0)
 expect(parseParameterDefault('-','integer')).toBe('-')
 expect(parseParameterDefault('','integer')).toBe('')
 expect(parseParameterDefault('9007199254740993','integer')).toBe('9007199254740993')
 expect(parseParameterDefault('1.5','integer')).toBe('1.5')
 expect(parseParameterDefault('1.5','number')).toBe(1.5)
 expect(parseParameterDefault('false','string')).toBe('false')
})
