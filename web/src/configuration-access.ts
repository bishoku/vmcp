import { createContext, useContext, useEffect, useState } from 'react'
import { api, type ConfigurationCheck } from './api'

export const configurationPath = (family: string, slug = '') =>
  '/' +
  ({
    virtual_servers: 'virtual-servers',
    agent_keys: 'agent-keys',
    identity_bindings: 'identity-bindings',
    control_tokens: 'admin-tokens',
    tool_grants: 'tool-grants',
    admission: 'admission-settings',
    audit: 'audit-settings',
    config: 'export',
    platform: 'info',
  }[family] ?? family) +
  (slug ? '/' + encodeURIComponent(slug) : '')
/** No allow cache: policy changes are rechecked by the transport before each request. */
export function useConfigurationChecks(checks: ConfigurationCheck[]) {
  const key = JSON.stringify(checks)
  const [state, setState] = useState<{ key: string; allowed: boolean[] }>({ key: '', allowed: [] })
  useEffect(() => {
    const controller = new AbortController()
    const parsed = JSON.parse(key) as ConfigurationCheck[]
    void (async () => {
      const allowed: boolean[] = []
      for (let i = 0; i < parsed.length; i += 64) {
        const result = await api.configurationAccess(parsed.slice(i, i + 64), controller.signal)
        allowed.push(...result.allowed.map((value) => value === true))
      }
      if (!controller.signal.aborted) setState({ key, allowed })
    })().catch(() => {
      if (!controller.signal.aborted) setState({ key, allowed: parsed.map(() => false) })
    })
    return () => controller.abort()
  }, [key])
  return { ready: state.key === key, allowed: state.key === key ? state.allowed : [] }
}
export function useConfigurationPermissions(
  family: string,
  slug: string,
  actions: string[],
  body?: unknown,
) {
  const checks = actions.map((action) => ({
    method: 'GET',
    path: configurationPath(family, slug),
    permission: action === 'ownership.transfer' ? action : family + '.' + action,
    body,
  }))
  const result = useConfigurationChecks(checks)
  const can = (action: string) => result.allowed[actions.indexOf(action)] === true
  return { ...result, can }
}
export function useConfigurationRows(family: string, slugs: string[], actions: string[]) {
  const result = useConfigurationChecks(
    slugs.flatMap((slug) =>
      actions.map((action) => ({
        method: 'GET',
        path: configurationPath(family, slug),
        permission: family + '.' + action,
      })),
    ),
  )
  return (slug: string, action: string) =>
    result.allowed[slugs.indexOf(slug) * actions.length + actions.indexOf(action)] === true
}
/** Merge-patch only changed root fields. Preserves restricted defaults and secrets. */
export function configurationPatch<T extends Record<string, unknown>>(
  body: T,
  baseline: Record<string, unknown>,
): Partial<T> {
  const result = Object.fromEntries(
    Object.entries(body).filter(
      ([key, value]) => JSON.stringify(value) !== JSON.stringify(baseline[key]),
    ),
  ) as Partial<T>
  for (const key of ['tls', 'auth']) {
    if (result[key] && baseline[key] && typeof result[key] === 'object') {
      const nested = Object.fromEntries(
        Object.entries(result[key] as Record<string, unknown>).filter(
          ([field, value]) =>
            JSON.stringify(value) !==
            JSON.stringify((baseline[key] as Record<string, unknown>)[field]),
        ),
      )
      if (Object.keys(nested).length === 0) delete result[key]
      else (result as Record<string, unknown>)[key] = nested
    }
  }
  return result
}

export const ConfigurationFields = createContext<(field: string) => boolean>(() => true)
export const useConfigurationField = (name?: string) => useContext(ConfigurationFields)(name ?? '')

/** A create may rely on server defaults; forbidden optional fields are omitted. */
export function configurationCreate<
  T extends Record<string, unknown>,
  P extends Record<string, string>,
>(body: T, permissions: P, can: (permission: string) => boolean): Omit<T, keyof P> & Partial<T> {
  return Object.fromEntries(
    Object.entries(body).filter(([field]) => !permissions[field] || can(permissions[field])),
  ) as Omit<T, keyof P> & Partial<T>
}
