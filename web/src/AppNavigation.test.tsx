import {RoleContext} from './interaction'
import {cleanup,render,screen} from '@testing-library/react'
import {MemoryRouter} from 'react-router-dom'
import {afterEach,expect,test,vi} from 'vitest'
import AppNavigation from './AppNavigation'
afterEach(()=>{cleanup();vi.unstubAllGlobals()})
test('custom sessions are offered only the enforced audit surface',()=>{
 vi.stubGlobal('matchMedia',()=>({matches:true,addEventListener:vi.fn(),removeEventListener:vi.fn()}))
 render(<RoleContext.Provider value="custom"><MemoryRouter><AppNavigation me={{name:'Auditor',role:'custom',via:'oidc'}} signOut={<button>Sign out</button>}/></MemoryRouter></RoleContext.Provider>)
 expect(screen.getByRole('link',{name:'Audit Log'}).getAttribute('href')).toBe('/audit')
 for(const name of ['Upstreams','Virtual Servers','Agent Keys','Identities','Settings'])expect(screen.queryByRole('link',{name})).toBeNull()
})

test.each(['viewer','editor','team','custom','admin'] as const)('%s sees access management only with the global admin role',role=>{
 vi.stubGlobal('matchMedia',()=>({matches:true,addEventListener:vi.fn(),removeEventListener:vi.fn()}))
 render(<MemoryRouter><AppNavigation me={{name:'User',role,via:'oidc'}} signOut={<button>Sign out</button>}/></MemoryRouter>)
 expect(screen.queryAllByRole('link',{name:'Access management'})).toHaveLength(role==='admin'?1:0)
})
