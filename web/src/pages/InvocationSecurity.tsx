import { useEffect, useState } from 'react'
import { api, type InvocationSettings, type InvocationRule } from '../api'
import { btn, input, primary } from '../styles'

export default function InvocationSecurity() {
 const [settings,setSettings]=useState<InvocationSettings|null>(null)
 const [upstreams,setUpstreams]=useState<{id:string;slug:string}[]>([])
 const [upstream,setUpstream]=useState(''),[tool,setTool]=useState('')
 const [service,setService]=useState(''),[url,setURL]=useState(''),[origin,setOrigin]=useState(''),[secret,setSecret]=useState(''),[ca,setCA]=useState('')
 const [acr,setACR]=useState(''),[age,setAge]=useState(300),[approvals,setApprovals]=useState(''),[fields,setFields]=useState('')
 const [identity,setIdentity]=useState(''),[identityService,setIdentityService]=useState(''),[identityFields,setIdentityFields]=useState('')
 const [error,setError]=useState(''),[notice,setNotice]=useState(''),[busy,setBusy]=useState(false),[dirty,setDirty]=useState(false)
 const split=(value:string)=>value.split(',').map(s=>s.trim()).filter(Boolean)
 useEffect(()=>{let live=true;Promise.all([api.invocationSecurity(),api.invocationUpstreams()]).then(([s,u])=>{if(live){setSettings(s);setUpstreams(u.items)}}).catch(e=>{if(live)setError(String(e))});return()=>{live=false}},[])
 const change=(next:InvocationSettings)=>{setSettings(next);setDirty(true);setNotice('Unsaved changes')}
 const loadRule=(id:string,name:string)=>{const policy=settings?.upstreams[id];const rule=name?policy?.tools?.[name]:policy;setIdentity(rule?.identity?.source||'');setIdentityService(rule?.identity?.service||'');setIdentityFields(rule?.identity?.fields?.join(', ')||'');setACR(rule?.stepUp?.acceptedAcrValues.join(', ')||'');setAge(rule?.stepUp?.maxAgeSeconds||300);setApprovals(rule?.approvalServices?.join(', ')||'');setFields(rule?.shareFields?.join(', ')||'')}
 const selectService=(name:string)=>{setService(name);const s=settings?.services[name];setURL(s?.url||'');setOrigin(s?.approvalOrigin||'');setSecret('');setCA(s?.caPem||'')}
 const applyRule=()=>{if(!settings||!upstream)return;const rule:InvocationRule={};if(identity==='oauth_subject')rule.identity={source:identity};if(identity==='caller_context')rule.identity={source:identity,service:identityService,fields:split(identityFields)};if(acr.trim())rule.stepUp={acceptedAcrValues:split(acr),maxAgeSeconds:age};if(approvals.trim())rule.approvalServices=split(approvals);if(fields.trim())rule.shareFields=split(fields);const old=settings.upstreams[upstream]||{};change({...settings,upstreams:{...settings.upstreams,[upstream]:tool?{...old,tools:{...old.tools,[tool]:rule}}:{...rule,tools:old.tools}}})}
 return <div className="page-layout"><h1 className="page-title">Invocation security</h1><p>Require recent strong authentication and operation-specific approval before execution. Tool rules inherit upstream requirements and can only strengthen them.</p>
 {error&&<p role="alert">{error}</p>}{notice&&<p role="status">{notice}</p>}
 {!settings?<p>Loading security policies…</p>:<>
 <section className="surface-card space-y-3 p-5"><h2>Approval services</h2><label>Existing service<select className={input} value={settings.services[service]?service:''} onChange={e=>selectService(e.target.value)}><option value="">New service</option>{Object.keys(settings.services).map(s=><option key={s}>{s}</option>)}</select></label>
 <label>Service name<input className={input} value={service} onChange={e=>setService(e.target.value)}/></label>
 <label>HTTPS decision endpoint<input className={input} value={url} onChange={e=>setURL(e.target.value)}/></label>
 <label>Trusted approval screen origin<input className={input} value={origin} placeholder="https://approval.example.com" onChange={e=>setOrigin(e.target.value)}/></label>
 <label>Service token<input type="password" autoComplete="new-password" className={input} value={secret} placeholder={settings.services[service]?.hasToken?'Stored; leave blank to keep':'Required for new services'} onChange={e=>setSecret(e.target.value)}/></label>
 <label>Custom CA certificate (optional)<textarea className={input} value={ca} onChange={e=>setCA(e.target.value)}/></label>
 <button className={btn} disabled={!service||!url||!origin} onClick={()=>{change({...settings,services:{...settings.services,[service]:{url,approvalOrigin:origin,token:secret||settings.services[service]?.token,hasToken:settings.services[service]?.hasToken,caPem:ca}}});setSecret('')}}>Stage service</button>
 </section>
 <section className="surface-card space-y-3 p-5"><h2>Upstream and tool policy</h2>
 <label>Upstream<select className={input} value={upstream} onChange={e=>{setUpstream(e.target.value);loadRule(e.target.value,tool)}}><option value="">Select upstream</option>{upstreams.map(u=><option key={u.id} value={u.id}>{u.slug}</option>)}</select></label>
 <label>Original tool name (empty = upstream default)<input className={input} value={tool} onChange={e=>{setTool(e.target.value);loadRule(upstream,e.target.value)}}/></label>
 <label>User identity source<select className={input} value={identity} onChange={e=>setIdentity(e.target.value)}><option value="">Inherit / unspecified</option><option value="oauth_subject">OAuth subject is the user</option><option value="caller_context">Application identity + caller context user</option></select></label>
 {identity==='caller_context'&&<><label>Session verification service<select className={input} value={identityService} onChange={e=>setIdentityService(e.target.value)}><option value="">Select service</option>{Object.keys(settings.services).map(s=><option key={s}>{s}</option>)}</select></label><label>Caller context fields sent for verification<input className={input} placeholder="customerId, sessionId" value={identityFields} onChange={e=>setIdentityFields(e.target.value)}/></label><p>The service verifies the customer session and authentication assurance. These fields must be enabled in the upstream caller context configuration. Gateway credentials continue to identify the application.</p></>}
 <p>Protected policies require an explicit or inherited identity source. A tool cannot replace an inherited source. There is no fallback between sources.</p>
 <label>Accepted ACR values, comma separated<input className={input} placeholder="Empty = no additional step-up requirement" value={acr} onChange={e=>setACR(e.target.value)}/></label>
 <label>Maximum authentication age (seconds)<input className={input} type="number" min={1} max={86400} value={age} onChange={e=>setAge(Number(e.target.value))}/></label>
 <label>Required approval services, comma separated<input className={input} value={approvals} onChange={e=>setApprovals(e.target.value)}/></label>
 <label>Argument fields shared with approval services, comma separated<input className={input} value={fields} onChange={e=>setFields(e.target.value)}/></label>
 <button className={btn} disabled={!upstream} onClick={applyRule}>Stage policy</button>
 <p>Inherited approval services accumulate. Accepted ACR values intersect, and the shortest authentication age wins. Empty tool requirements retain upstream requirements.</p>
 <pre className="overflow-auto text-xs">{JSON.stringify({inherited:settings.upstreams[upstream]&&{...settings.upstreams[upstream],tools:undefined},tool:settings.upstreams[upstream]?.tools?.[tool]},null,2)}</pre>
 </section>
 <button className={primary} disabled={busy||!dirty} onClick={async()=>{setBusy(true);setError('');try{setSettings(await api.saveInvocationSecurity(settings));setDirty(false);setNotice('Invocation policies saved')}catch(e){setError(String(e))}finally{setBusy(false)}}}>{busy?'Saving…':'Save security configuration'}</button>
 </>}
 </div>
}
