import { vi } from 'vitest'

// Existing component regressions use an explicit synchronous server-policy fixture.
// Browser tests exercise the real asynchronous hooks and request preflight.
// Role labels here choose test fixtures only; production never infers permissions from them.
vi.mock('./src/configuration-access', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./src/configuration-access')>()
  const { useContext } = await import('react')
  const { RoleContext } = await import('./src/interaction')
  function usePolicy() {
    const role = useContext(RoleContext)
    return (permission: string, method = 'GET') =>
      role === 'admin' ||
      (role !== 'custom' &&
        method === 'GET' &&
        (!permission || permission.endsWith('.read') || permission === 'config.export')) ||
      (['editor', 'team'].includes(role) && !permission.startsWith('control_tokens.'))
  }
  return {
    ...actual,
    useConfigurationChecks: (checks: import('./src/api').ConfigurationCheck[]) => {
      const allow = usePolicy()
      return {
        ready: true,
        allowed: checks.map((check) => allow(check.permission ?? '', check.method)),
      }
    },
    useConfigurationPermissions: (family: string, _slug: string, actions: string[]) => {
      const allow = usePolicy()
      return {
        ready: true,
        allowed: actions.map((action) => allow(family + '.' + action, 'POST')),
        can: (action: string) => allow(family + '.' + action, 'POST'),
      }
    },
    useConfigurationRows: (family: string, _slugs: string[], _actions: string[]) => {
      const allow = usePolicy()
      return (_slug: string, action: string) => allow(family + '.' + action, 'POST')
    },
  }
})
