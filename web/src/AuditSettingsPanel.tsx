import { useEffect, useState } from 'react'
import { ShieldCheck } from 'lucide-react'
import { api, type AuditSettings, type AuditRule, type AuditPreview } from './api'
import { useCapabilities, useDraft, useOperation } from './interaction'
import { ErrorBox } from './ui'
import { ServerPanel } from './connections/ServerPanels'
import { btn, input, primary } from './styles'

const scopes=['virtual','upstream','control','system','reads'] as const
const labels:Record<string,string>={virtual:'Virtual-server calls',upstream:'Upstream calls',control:'Management API',system:'System payload scope',reads:'Read API calls'}
export default function AuditSettingsPanel({scope='global',slug=''}:{scope?:'global'|'virtual'|'upstream';slug?:string}){
 const {manageTokens}=useCapabilities()
 if(!manageTokens)return null
 return <AuditEditor key={`${scope}:${slug}`} scope={scope} slug={slug}/>
}
function AuditEditor({scope,slug}:{scope:'global'|'virtual'|'upstream';slug:string}){
 const [draft,setDraft,resetDraft,openDraft]=useDraft<AuditSettings|null>(null,()=>scope+slug)
 const [error,setError]=useState(''),[notice,setNotice]=useState('')
 const {busy,run}=useOperation()
 const [vs,setVs]=useState(scope==='virtual'?slug:''),[up,setUp]=useState(scope==='upstream'?slug:'')
 const [servers,setServers]=useState<string[]>([]),[upstreams,setUpstreams]=useState<string[]>([])
 const [previewScope,setPreviewScope]=useState(scope==='global'?'control':scope)
 const [requestText,setRequestText]=useState('{\n  "email": "person@example.com",\n  "token": "example-secret"\n}')
 const [responseText,setResponseText]=useState('{\n  "result": "person@example.com"\n}')
 const [results,setResults]=useState<{request:AuditPreview;response:AuditPreview}|null>(null)
 const [consumer,setConsumer]=useState('')
 useEffect(()=>{const controller=new AbortController();api.auditSettings(controller.signal).then(v=>{if(!controller.signal.aborted)resetDraft(v)}).catch(e=>{if(!controller.signal.aborted)setError(String(e))});
 api.listUpstreams(controller.signal).then(v=>{if(!controller.signal.aborted)setUpstreams(v.map(x=>x.slug))}).catch(()=>{});
 api.listVirtualServers(controller.signal).then(v=>{if(!controller.signal.aborted)setServers(v.map(x=>x.slug))}).catch(()=>{});
 return()=>controller.abort()
 // Reset only when the panel is mounted for a different entity.
 // eslint-disable-next-line react-hooks/exhaustive-deps
 },[scope,slug])
 const change=(next:AuditSettings)=>{setDraft(next);setResults(null);setNotice('')}
 const rules=draft?(scope==='global'?draft.rules:(scope==='virtual'?draft.virtualServers[slug]:draft.upstreams[slug])??[]):[]
 const setRules=(next:AuditRule[])=>{if(!draft)return;if(scope==='global')change({...draft,rules:next});else if(scope==='virtual')change({...draft,virtualServers:{...draft.virtualServers,[slug]:next}});else change({...draft,upstreams:{...draft.upstreams,[slug]:next}})}
 const test=async()=>{if(!draft)return;setError('');setResults(null);await run(async()=>{try{const request=JSON.parse(requestText),response=JSON.parse(responseText);const common={editScope:scope,settings:draft,scope:previewScope,virtualServer:vs,upstream:up};const [a,b]=await Promise.all([api.previewAudit({...common,direction:'request',payload:request}),api.previewAudit({...common,direction:'response',payload:response})]);setResults({request:a,response:b})}catch(e){setError(e instanceof Error?e.message:String(e))}})}
 const save=async()=>{if(!draft)return;setError('');await run(async()=>{try{const saved=await api.saveAuditSettings(draft);resetDraft(saved);setResults(null);setNotice('Audit settings saved. New operations use the updated policy after configuration refresh.')}catch(e){setError(e instanceof Error?e.message:String(e))}})}
 return <ServerPanel title={scope==='global'?'Audit capture & masking':'Audit payload masking'} description={scope==='global'?'Durability, payload retention and global masking rules':'Local rules add to global protection and apply to both requests and responses.'} icon={ShieldCheck}>
 <fieldset disabled={busy} className="space-y-5 panel-body">
 {error&&<ErrorBox>{error}<button className={btn} onClick={()=>{void api.auditSettings().then(v=>{openDraft(v);setError('');setResults(null)}).catch(e=>setError(String(e)))}}>Reload audit settings</button></ErrorBox>}{notice&&<p role="status" className="text-sm">{notice}</p>}
 {!draft?<p className="text-sm text-ink-muted">Loading audit settings…</p>:<>
 {scope==='global'&&<>
 <label className="block text-sm">Audit mode<select aria-label="Audit mode" className={input} value={draft.mode} onChange={e=>change({...draft,mode:e.target.value as AuditSettings['mode']})}><option value="">Deployment default (control plane: strict)</option><option value="strict">Strict — wait for durable audit storage</option><option value="relax">Relax — asynchronous, records may be lost</option></select></label>
 <p className="text-xs text-ink-muted">Strict can block calls when audit storage fails. Relax queues metadata and payload together without waiting for database storage.</p>
 <div className="grid gap-4 md:grid-cols-2"><label className="text-sm">Payload retention (days)<input aria-label="Payload retention days" type="number" min={1} max={3650} className={input} value={draft.retentionDays} onChange={e=>change({...draft,retentionDays:Number(e.target.value)})}/></label><label className="text-sm">Maximum stored payload (bytes)<input aria-label="Maximum audit payload bytes" type="number" min={256} max={1048576} className={input} value={draft.maxPayloadBytes} onChange={e=>change({...draft,maxPayloadBytes:Number(e.target.value)})}/></label></div>
 <fieldset className="space-y-2"><legend className="mb-2 text-sm font-medium">Payload capture</legend>{scopes.map(s=><div key={s} className="flex flex-wrap items-center gap-4 text-sm"><span className="min-w-40">{labels[s]}</span>{(['request','response'] as const).map(direction=><label key={direction} className="flex gap-2"><input type="checkbox" checked={draft.scopes[s]?.[direction]??false} onChange={e=>change({...draft,scopes:{...draft.scopes,[s]:{...(draft.scopes[s]??{request:false,response:false}),[direction]:e.target.checked}}})}/>{direction}</label>)}</div>)}</fieldset>
 <fieldset className="space-y-2"><legend className="mb-2 text-sm font-medium">Payload export by consumer</legend>{Object.entries(draft.exportPayloads??{}).map(([name,enabled])=><label key={name} className="flex gap-2 text-sm"><input type="checkbox" checked={enabled} onChange={e=>change({...draft,exportPayloads:{...draft.exportPayloads,[name]:e.target.checked}})}/>{name}</label>)}<div className="flex gap-2"><input aria-label="Exporter consumer name" className={input} value={consumer} placeholder="Consumer name" onChange={e=>setConsumer(e.target.value)}/><button className={btn} disabled={!consumer||busy} onClick={()=>{change({...draft,exportPayloads:{...draft.exportPayloads,[consumer]:true}});setConsumer('')}}>Add consumer</button></div><p className="text-xs text-ink-muted">Payload exporters require the encryption key. Consumers without an enabled setting receive metadata only.</p></fieldset>
 </>}
 {scope!=='global'&&<p className="text-xs text-ink-muted">Inherited global rules: {draft.rules.map(r=>r.name).join(', ')||'none'}. Global rules and mandatory secret filters cannot be disabled here. Capture is controlled in Settings.</p>}
 <fieldset className="space-y-3"><legend className="mb-2 text-sm font-medium">{scope==='global'?'Global':'Local'} RE2 masking rules</legend>{rules.map((rule,i)=><div className="flex flex-wrap gap-2" key={i}><input aria-label={`Audit rule ${i+1} name`} className={input} value={rule.name} placeholder="Rule name" onChange={e=>setRules(rules.map((r,j)=>i===j?{...r,name:e.target.value}:r))}/><input aria-label={`Audit rule ${i+1} regex`} className={`${input} font-mono`} value={rule.pattern} placeholder="RE2 regex" onChange={e=>setRules(rules.map((r,j)=>i===j?{...r,pattern:e.target.value}:r))}/><button className={btn} onClick={()=>setRules(rules.filter((_,j)=>j!==i))}>Remove rule {i+1}</button></div>)}<button className={btn} disabled={rules.length>=20||busy} onClick={()=>setRules([...rules,{name:'',pattern:''}])}>Add masking rule</button></fieldset>
 <details className="rounded-lg border border-line p-4"><summary className="cursor-pointer text-sm font-medium">Test request / response audit payload</summary><div className="mt-4 space-y-4">
 <p className="text-xs text-ink-muted">Uses unsaved rules and the production masking engine. Samples are not saved. Select the boundary and related server to include its rules. Output represents the selected boundary, before encryption.</p>
 <div className="grid gap-3 md:grid-cols-3"><label className="text-sm">Capture scope<select aria-label="Preview capture scope" className={input} value={previewScope} onChange={e=>{setPreviewScope(e.target.value);setResults(null)}}>{scopes.map(s=><option key={s} value={s}>{labels[s]}</option>)}</select></label><label className="text-sm">Virtual server<select aria-label="Preview virtual server" className={input} disabled={scope==='virtual'} value={vs} onChange={e=>{setVs(e.target.value);setResults(null)}}><option value="">No virtual-server context</option>{[...new Set([...servers,...(scope==='virtual'?[slug]:[])])].map(s=><option key={s}>{s}</option>)}</select></label><label className="text-sm">Upstream<select aria-label="Preview upstream" className={input} disabled={scope==='upstream'} value={up} onChange={e=>{setUp(e.target.value);setResults(null)}}><option value="">No upstream context</option>{[...new Set([...upstreams,...(scope==='upstream'?[slug]:[])])].map(s=><option key={s}>{s}</option>)}</select></label></div>
 <div className="grid gap-4 md:grid-cols-2"><label className="text-sm">Request JSON<textarea aria-label="Example audit request" className={`${input} min-h-40 font-mono`} value={requestText} onChange={e=>{setRequestText(e.target.value);setResults(null)}}/></label><label className="text-sm">Response JSON<textarea aria-label="Example audit response" className={`${input} min-h-40 font-mono`} value={responseText} onChange={e=>{setResponseText(e.target.value);setResults(null)}}/></label></div>
 <button className={btn} disabled={busy} onClick={()=>void test()}>Preview audit payloads</button>
 {results&&<div className="grid gap-4 md:grid-cols-2">{(['request','response'] as const).map(direction=><div key={direction}><h4 className="text-sm font-medium">{direction} · {results[direction].state}</h4><p className="text-xs text-ink-muted">{results[direction].originalBytes} original bytes · {results[direction].storedBytes} stored bytes · {results[direction].redactions} redactions</p><pre className="mt-2 overflow-auto whitespace-pre-wrap break-all surface-inset p-3 text-xs">{JSON.stringify(results[direction].content??null,null,2)}</pre>{results[direction].matches.map(m=><p className="text-xs" key={`${m.source}:${m.name}`}>{m.source} / {m.name}: {m.count}</p>)}</div>)}</div>}
 </div></details>
 <p className="text-xs text-ink-muted">Routine discovery and subscription maintenance use logs and metrics. System catalog changes contain audit metadata only; enabling system payload capture does not record routine maintenance.</p>
 <div className="flex items-center justify-between gap-3"><span className="text-xs text-ink-muted">Policy revision {draft.revision}</span><button className={primary} disabled={busy} onClick={()=>void save()}>Save audit settings</button></div>
 </>}
 </fieldset></ServerPanel>
}
