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
  if (!create && !upstream) return <div className="p-10 space-y-4"><h1 className="text-2xl font-semibold">Upstream</h1>{loading ? <p role="status">Loading saved configuration…</p> : <p>{readError ? 'Upstream configuration is unavailable.' : `Upstream “${slug}” was not found.`}</p>}{readError && <ErrorBox>{readError}</ErrorBox>}<button className={btn} onClick={()=>load()} disabled={loading}>Retry</button><Link className={btn} to="/upstreams">Back to upstreams</Link></div>
  return <>{readError && <ErrorBox>{readError}</ErrorBox>}<UpstreamDetailForm key={`${slug}:${revision}`} upstream={create ? null : upstream!} onClose={()=>navigate('/upstreams')} onReload={latest=>{setReplacement(latest);setRevision(v=>v+1)}} onSaved={saved=>{
    setReplacement(saved);setRevision(v=>v+1)
    if(create) navigate(`/upstreams/${encodeURIComponent(saved.slug)}`)
  }}/></>
}
