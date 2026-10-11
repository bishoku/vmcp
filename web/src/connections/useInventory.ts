import { useEffect, useState } from 'react'
import { api, type AgentKey, type IdentityBinding, type Upstream, type VirtualServer } from '../api'
import { source, useSources } from '../requests'

export function useInventory() {
  const [servers, setServers] = useState<VirtualServer[]>([])
  const [upstreams, setUpstreams] = useState<Upstream[]>([])
  const [keys, setKeys] = useState<AgentKey[]>([])
  const [bindings, setBindings] = useState<IdentityBinding[]>([])
  const [loaded, setLoaded] = useState<string[]>([])
  const [checkedAt, setCheckedAt] = useState<string | null>(null)
  const accept = <T,>(name: string, setter: (data: T) => void) => (data: T) => {
    setter(data)
    setLoaded(previous => [...new Set([...previous, name])])
    setCheckedAt(new Date().toISOString())
  }
  const reads = useSources(() => [
    source('Virtual servers', api.listVirtualServers, accept('servers', setServers)),
    source('Upstreams', api.listUpstreams, accept('upstreams', setUpstreams)),
    source('Agent keys', api.listAgentKeys, accept('keys', setKeys)),
    source('Identity bindings', api.listBindings, accept('bindings', setBindings)),
  ])
  const {load} = reads
  useEffect(() => { const timer = setTimeout(() => { void load() }, 0); return () => clearTimeout(timer) }, [load])
  return {servers, upstreams, keys, bindings, loaded, checkedAt, ...reads}
}
