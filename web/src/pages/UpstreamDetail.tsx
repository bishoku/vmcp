import { EmptyState, LoadingState } from '../Feedback'
import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, type Upstream } from '../api'
import { source, useSources } from '../requests'
import { ErrorBox } from '../ui'
import { btn } from '../styles'
import UpstreamDetailForm from './UpstreamDetailForm'

export default function UpstreamDetail({create = false}: {create?: boolean}) {
  const { slug = '' } = useParams()
  const navigate = useNavigate()
  const [items, setItems] = useState<Upstream[]>([])
  const [replacement, setReplacement] = useState<Upstream | null>(null)
  const [revision, setRevision] = useState(0)
  const { load, loading, readError } = useSources(() => create ? [] : [source('Upstreams', api.listUpstreams, setItems)])
  useEffect(() => {void load()}, [load])
  const upstream = (replacement?.slug === slug ? replacement : null) ?? items.find(item => item.slug === slug)
  if (!create && !upstream) return <div className="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-10 lg:py-10"><h1 className="page-title">Upstream</h1>{loading?<LoadingState label="Loading saved configuration…" detail="Reading the saved upstream inventory."/>:readError?<ErrorBox title="Upstream configuration is unavailable." onRetry={load} retryLabel="Retry" busy={loading}>{readError}</ErrorBox>:<div className="rounded-card border border-line bg-surface shadow-card"><EmptyState title={`Upstream “${slug}” was not found.`} description="Check the identifier or return to the upstream inventory."/></div>}<div className="flex flex-wrap gap-3"><Link className={btn} to="/upstreams">Back to upstreams</Link>{!loading&&!readError&&<button className={btn} onClick={()=>void load()}>Retry</button>}</div></div>

  return <>{readError && <ErrorBox title="Saved configuration could not be loaded" onRetry={load} busy={loading}>{readError}</ErrorBox>}<UpstreamDetailForm key={`${slug}:${revision}`} upstream={create ? null : upstream!} onClose={()=>navigate('/upstreams')} onReload={latest=>{setReplacement(latest);setRevision(v=>v+1)}} onSaved={saved=>{
    setReplacement(saved);setRevision(v=>v+1)
    if(create) navigate(`/upstreams/${encodeURIComponent(saved.slug)}`)
  }}/></>
}
