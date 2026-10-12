declare global {
  interface Window { __VMCP_CONFIG__?: { publicGatewayURL?: string } }
}

/** Public gateway configuration is intentionally separate from the UI origin. */
export function connectionEndpoint(path: string, base = window.__VMCP_CONFIG__?.publicGatewayURL ?? import.meta.env.VITE_PUBLIC_GATEWAY_URL ?? ''): string | null {
  if (!base.trim()) return null
  try {
    const url = new URL(base)
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash) return null
    if (!path.startsWith('/mcp/') || path.includes('?') || path.includes('#')) return null
    return `${url.href.replace(/\/+$/, '')}${path}`
  } catch {
    return null
  }
}
