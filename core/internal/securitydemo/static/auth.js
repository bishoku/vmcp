export const config=await fetch('/config').then(r=>r.json());
const approval=location.pathname==='/approve.html';
const customer=location.pathname==='/session.html'||(!approval&&sessionStorage.getItem('demo-mode')==='caller_context');
const tokenKey=approval?'demo-approval-token':customer?'demo-customer-token':'demo-token';
const clientId=approval?config.approvalClientId:customer?config.customerClientId:config.clientId;
const enc=v=>btoa(String.fromCharCode(...new Uint8Array(v))).replaceAll('+','-').replaceAll('/','_').replaceAll('=','');
export function token(){return sessionStorage.getItem(tokenKey)||''}
export function clearToken(){sessionStorage.removeItem(tokenKey)}
export async function login(acr,maxAge){
 const verifier=enc(crypto.getRandomValues(new Uint8Array(32))),state=crypto.randomUUID();
 const redirect=location.origin+location.pathname;
 sessionStorage.setItem('demo-auth',JSON.stringify({verifier,state,redirect,returnURL:location.href,clientId,tokenKey}));
 const u=new URL(config.issuer+'/protocol/openid-connect/auth');
 const p={client_id:clientId,redirect_uri:redirect,response_type:'code',scope:'openid',state,code_challenge:enc(await crypto.subtle.digest('SHA-256',new TextEncoder().encode(verifier))),code_challenge_method:'S256'};
 if(acr){p.acr_values=acr;p.claims=JSON.stringify({id_token:{acr:{essential:true,values:acr.split(' ')}}})}
 if(maxAge!==undefined)p.max_age=String(maxAge);
 if(!approval&&!acr)p.prompt='login';
 u.search=new URLSearchParams(p);location.assign(u);
}
export async function callback(){
 const q=new URLSearchParams(location.search);if(!q.has('code')&&!q.has('error'))return;
 const stored=JSON.parse(sessionStorage.getItem('demo-auth')||'null');sessionStorage.removeItem('demo-auth');
 if(!stored||q.get('state')!==stored.state||stored.clientId!==clientId||stored.tokenKey!==tokenKey)throw Error('OAuth state or client mismatch');
 if(q.has('error'))throw Error('Identity provider denied authentication');
 const r=await fetch('/oauth/token',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({grant_type:'authorization_code',client_id:clientId,code:q.get('code'),redirect_uri:stored.redirect,code_verifier:stored.verifier})});
 if(!r.ok)throw Error('Token exchange failed');const t=await r.json();sessionStorage.setItem(tokenKey,t.access_token);history.replaceState({},'',stored.returnURL);
}
export function summary(){try{const p=token().split('.')[1];const c=JSON.parse(atob(p.replaceAll('-','+').replaceAll('_','/')));return {subject:c.sub,acr:c.acr,auth_time:c.auth_time,expires:c.exp,note:'Display only; the receiving backend verifies the token.'}}catch{return {status:'Not signed in'}}}
