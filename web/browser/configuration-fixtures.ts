import type { Route } from '@playwright/test'

// Explicit capability responses for presentation fixtures. This is a mock
// server policy, not a fallback from the production /me role label. Security
// scenarios provide their own resource/field decisions instead of this helper.
export function configurationFixture(route: Route, profile: 'administrator' | 'reader' | 'operator' | 'runtime-reader' | 'approver', scopedInvocation = false) {
 // Settings presentation fixtures need a valid singleton response, not the
 // generic collection fallback. Explicit write and denial fixtures stay in each test.
 const path = new URL(route.request().url()).pathname
 if (path === '/api/v1/admission-settings' && route.request().method() === 'GET') return route.fulfill({json:{revision:1,total:1024,perServer:128,perPrincipal:16}})
 if (path !== '/api/v1/configuration-access') return undefined
 const {checks} = route.request().postDataJSON() as {checks:{method:string;path:string;permission?:string}[]}
 return route.fulfill({json:{authorizationRevision:1,allowed:checks.map(check => (scopedInvocation || !/^\/upstreams\/[^/]+\/invocation-security$/.test(check.path)) && (
  profile === 'administrator' ||
  (profile === 'operator' && !(check.permission?.startsWith('control_tokens.') || check.path.startsWith('/admin-tokens'))) ||
  (profile === 'approver' && (check.permission?.startsWith('tool_grants.') || check.path.startsWith('/tool-grants'))) ||
  (profile === 'runtime-reader' && (check.permission === 'omni.preview' || check.path === '/omni/preview')) ||
  (check.permission ? check.permission.endsWith('.read') : check.method === 'GET')
 ))}})
}
