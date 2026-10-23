import {useEffect,useState} from 'react'
import {api,type Upstream,type MockStatus} from '../api'
import {btn} from '../styles'
import {useCapabilities} from '../interaction'
export default function MockUpstreamStatus({upstream}:{upstream:Upstream}){
 const [status,setStatus]=useState<MockStatus|null>(null),[error,setError]=useState(''),[readError,setReadError]=useState(''),[busy,setBusy]=useState(false)
 const {manageTokens:admin}=useCapabilities()
 useEffect(()=>{const controller=new AbortController();const load=()=>api.mockStatus(upstream.slug,controller.signal).then(value=>{setStatus(value);setReadError('')}).catch(e=>{if(!controller.signal.aborted)setReadError((e as Error).message)});void load();const timer=setInterval(()=>void load(),5000);return()=>{controller.abort();clearInterval(timer)}},[upstream.slug,upstream.version])
 return <section role="region" className="surface-card panel-body layout-stack" aria-label="Mock upstream status"><div><h3 className="text-sm font-semibold">WireMock publication</h3><p role="status" className="mt-1 text-sm">{status?.state??'Checking…'}{status?.ruleCount!==undefined?` · ${status.ruleCount} rules`:''}</p></div>{status?.message&&<p className="text-xs text-ink-muted">{status.message}</p>}{readError&&<p role="alert" className="text-xs text-red-700">{readError}</p>}{error&&<p role="alert" className="text-xs text-red-700">{error}</p>}{admin&&<button type="button" className={`${btn} self-start`} disabled={busy} onClick={async()=>{setBusy(true);setError('');try{setStatus(await api.syncMock(upstream.slug,upstream.version))}catch(e){setError((e as Error).message)}finally{setBusy(false)}}}>{busy?'Applying…':'Apply mock definitions now'}</button>}</section>
}
