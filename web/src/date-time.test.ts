import { expect, test } from 'vitest'
import { auditDateErrors, localDateTime, parseLocalDateTime } from './date-time'
import { dateError } from './form-validation'
test('local validation rejects calendar overflow and retains leap days and precision',()=>{
 expect(parseLocalDateTime('2027-02-30T10:00')).toBeNull()
 expect(parseLocalDateTime('2027-02-29T10:00')).toBeNull()
 expect(parseLocalDateTime('2028-02-29T10:00:12.789')?.getMilliseconds()).toBe(789)
 expect(parseLocalDateTime('2027-01-01T24:00')).toBeNull()
 expect(dateError('')).toBeUndefined();expect(dateError('2027-04-05T06:07')).toBeUndefined();expect(dateError('invalid')).toBeTruthy()
})
test('local display handles missing and invalid values and round trips the local minute',()=>{
 expect(localDateTime(null)).toBe('');expect(localDateTime('invalid')).toBe('')
 const iso='2027-02-03T04:05:06.789Z'
 expect(parseLocalDateTime(localDateTime(iso))!.getTime()).toBe(new Date(iso).getTime()-6789)
})
test('audit ranges reject invalid links and reversed instants, allowing open and equal bounds',()=>{
 expect(auditDateErrors({from:'invalid'}).from).toBeTruthy();expect(auditDateErrors({to:'invalid'}).to).toBeTruthy()
 expect(auditDateErrors({from:'2026-10-15T10:00Z',to:'2026-10-15T12:00+03:00'}).to).toBeTruthy()
 expect(auditDateErrors({from:'2026-10-15T10:00Z',to:'2026-10-15T13:00+03:00'})).toEqual({})
 expect(auditDateErrors({from:'2026-10-15T10:00Z'})).toEqual({})
})
