import{token,clearToken,login,callback,summary}from './auth.js';
const $=id=>document.getElementById(id);let session='',counter=0,operation=sessionStorage.getItem('demo-operation')||crypto.randomUUID();
const mode=sessionStorage.getItem('demo-mode')||'oauth_subject';$('mode').value=mode;
$('mode').onchange=()=>{sessionStorage.setItem('demo-mode',$('mode').value);sessionStorage.removeItem('demo-server');sessionStorage.removeItem('demo-params');sessionStorage.removeItem('demo-operation');location.assign('/')};
$('model-description').textContent=mode==='caller_context'?'The application backend uses one agent key for all customers. Customer sessions travel through caller context and are verified by the external security service.':'The gateway access token represents the end user. vMCP verifies its authentication assurance.';
let customerSession=JSON.parse(sessionStorage.getItem('demo-customer-session')||'null');
const log=(title,value)=>{$('log').textContent=title+'\n'+JSON.stringify(value,null,2)+'\n\n'+$('log').textContent};
function showOperation(){sessionStorage.setItem('demo-operation',operation);$('operation').textContent='Operation ID: '+operation}showOperation();
try{await callback()}catch(e){log('Authentication error',e.message)}$('identity').textContent=JSON.stringify(summary(),null,2);
if(mode==='caller_context'&&token()){
 try{
 const current=summary();if(customerSession?.user?.subject!==current.subject)customerSession=null;
 let r=await fetch('/customer-sessions',{method:'POST',headers:{'Authorization':'Bearer '+token(),'Content-Type':'application/json'},body:JSON.stringify({sessionId:customerSession?.sessionId||''})});
 if(!r.ok)throw Error('Customer session could not be established. Sign in again.');customerSession=await r.json();sessionStorage.setItem('demo-customer-session',JSON.stringify(customerSession));
 $('customer').textContent=JSON.stringify({applicationIdentity:'Shared agent key (kept on application backend)',customerId:customerSession.customerId,session:'Opaque application session established',acr:customerSession.user.acr},null,2);
 }catch(e){log('Customer session error',e.message)}
}
$('login').onclick=()=>login();$('stepup').onclick=()=>login('2',0);$('logout').onclick=()=>{clearToken();sessionStorage.removeItem('demo-customer-session');session='';$('identity').textContent='Local session cleared. Keycloak SSO may still be active.'};
$('new').onclick=()=>{operation=crypto.randomUUID();showOperation()};$('clear').onclick=()=>{$('log').textContent=''};
$('server').value=sessionStorage.getItem('demo-server')||(mode==='caller_context'?'security-context':'security-demo');$('params').value=sessionStorage.getItem('demo-params')||(mode==='caller_context'?JSON.stringify({name:'security-context__delete_records',arguments:{dataset:'demo',count:5}}):$('params').value);
async function rpc(method,params={},id=++counter){
 const server=$('server').value.trim();sessionStorage.setItem('demo-server',server);
 const prefix=mode==='caller_context'?'/application-gateway':'/gateway';
const path=server==='omni'?prefix+'/mcp/v1/omni':prefix+'/mcp/v1/virtual/'+encodeURIComponent(server);
if(mode==='caller_context'&&customerSession)params._meta={...params._meta,'demo/customer':{customerId:customerSession.customerId,sessionId:customerSession.sessionId},'io.vmcp/operationId':operation};
 const headers={'Authorization':'Bearer '+token(),'Content-Type':'application/json','Accept':'application/json, text/event-stream','MCP-Protocol-Version':'2025-06-18'};if(session)headers['Mcp-Session-Id']=session;
 const body={jsonrpc:'2.0',method,params};if(id!==null)body.id=id;const display=structuredClone(body);if(display.params?._meta?.['demo/customer'])display.params._meta['demo/customer'].sessionId='[REDACTED]';log('Request',display);
 const r=await fetch(path,{method:'POST',headers,body:JSON.stringify(body)});
 if(r.headers.get('Mcp-Session-Id'))session=r.headers.get('Mcp-Session-Id');
 if(r.status===401){const challenge=r.headers.get('WWW-Authenticate')||'';log('Authentication required',{challenge});if(challenge.includes('insufficient_user_authentication')){const a=/acr_values="([^"]+)"/.exec(challenge)?.[1];const age=/max_age="([0-9]+)"/.exec(challenge)?.[1];await login(a,age===undefined?undefined:Number(age))}return}
 if(r.status===202||r.status===204)return;
 if(r.headers.get('Content-Type')?.includes('text/event-stream')){
  const reader=r.body.getReader(),decoder=new TextDecoder();let buffer='';
  while(true){const chunk=await reader.read();if(chunk.done)break;buffer+=decoder.decode(chunk.value,{stream:true});let at;while((at=buffer.indexOf('\n\n'))>=0){const event=buffer.slice(0,at);buffer=buffer.slice(at+2);const data=event.split('\n').filter(x=>x.startsWith('data:')).map(x=>x.slice(5).trim()).join('\n');if(data){const msg=JSON.parse(data);await message(msg,path,headers);if(msg.id===id&&(msg.result||msg.error)){await reader.cancel();return msg}}}}
  return;
 }
 const text=await r.text();let data;try{data=JSON.parse(text)}catch{throw Error('HTTP '+r.status+': '+text.slice(0,300))}await message(data,path,headers);return data;
}
async function message(data,path,headers){
 log('Response',data);
 if(mode==='oauth_subject'&&data.error?.message==='insufficient_user_authentication'){const step=data.error.data;if(step?.acceptedAcrValues?.length){await login(step.acceptedAcrValues.join(' '),step.maxAgeSeconds);return}}
 if(data.method&&data.id!==undefined){
  let result;
  if(data.method==='elicitation/create'){const accepted=confirm(data.params.message);let content={};if(accepted)for(const [key,spec]of Object.entries(data.params.requestedSchema?.properties||{})){const v=prompt(key);content[key]=spec.type==='boolean'?v==='true':['number','integer'].includes(spec.type)?Number(v):v}result={action:accepted?'accept':'decline',...(accepted?{content}:{})}}
  else if(data.method==='sampling/createMessage'){const text=prompt('Demo sampling response (no external model is called)');result={role:'assistant',content:{type:'text',text:text||''},model:'human-demo',stopReason:'endTurn'}}
  else if(data.method==='roots/list')result={roots:[]};
  const response=result?{jsonrpc:'2.0',id:data.id,result}:{jsonrpc:'2.0',id:data.id,error:{code:-32601,message:'Unsupported demo capability'}};
  await fetch(path,{method:'POST',headers,body:JSON.stringify(response)});
 }
 const decision=data.error?.data?.decision;if((decision?.status==='pending'||decision?.status==='step_up_required')&&decision.url){const u=new URL(decision.url);if(u.origin!==location.origin)throw Error('Unexpected approval origin');const a=document.createElement('a');a.href=u.href;a.target='_blank';a.rel='noopener';a.textContent=decision.status==='step_up_required'?'Verify this customer session':'Review this operation in the approval service';$('approval').replaceChildren(a)}
}
const run=fn=>async()=>{try{await fn()}catch(e){log('Error',e.message)}};
$('connect').onclick=run(async()=>{session='';await rpc('initialize',{protocolVersion:'2025-06-18',capabilities:{elicitation:{},sampling:{},roots:{}},clientInfo:{name:'vmcp-protocol-lab',version:'1'}});await rpc('notifications/initialized',{},null)});
for(const [id,method]of [['tools','tools/list'],['resources','resources/list'],['prompts','prompts/list']])$(id).onclick=run(()=>rpc(method));
const send=run(()=>{sessionStorage.setItem('demo-params',$('params').value);const params=JSON.parse($('params').value);params._meta={...params._meta,'io.vmcp/operationId':operation};return rpc($('method').value,params)});$('send').onclick=send;$('continue').onclick=send;
