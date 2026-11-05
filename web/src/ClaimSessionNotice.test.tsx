import {act,cleanup,render,screen} from '@testing-library/react'
import {afterEach,expect,test,vi} from 'vitest'
import ClaimSessionNotice from './ClaimSessionNotice'
afterEach(()=>{cleanup();vi.useRealTimers();vi.restoreAllMocks()})
test('claim deadline clears stale authority and asks for a fresh sign-in without expiring the whole session',()=>{
 vi.useFakeTimers();vi.setSystemTime(new Date('2026-11-05T10:00:00Z'))
 const expired=vi.fn();window.addEventListener('vmcp:claims-expired',expired)
 const sessionExpired=vi.fn();window.addEventListener('vmcp:session-expired',sessionExpired)
 render(<ClaimSessionNotice me={{name:'User',role:'custom',via:'oidc',claimsExpire:'2026-11-05T10:15:00Z'}}/>)
 expect(screen.queryByRole('status')).toBeNull()
 act(()=>vi.advanceTimersByTime(15*60*1000))
 expect(expired).toHaveBeenCalledTimes(1);expect(sessionExpired).not.toHaveBeenCalled()
 expect(screen.getByRole('link',{name:'signing in again'}).getAttribute('href')).toBe('/auth/login')
 window.removeEventListener('vmcp:claims-expired',expired);window.removeEventListener('vmcp:session-expired',sessionExpired)
})
test('legacy or token sessions do not acquire a claim timeout',()=>{
 render(<ClaimSessionNotice me={{name:'Token',role:'admin',via:'admin-token'}}/>)
 expect(screen.queryByRole('status')).toBeNull()
})
