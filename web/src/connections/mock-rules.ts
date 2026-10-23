import type {MockRule} from '../api'

const responseKeys=['status','headers','jsonBody','body','fixedDelayMilliseconds','transformers']
export function parseMockRules(text:string):MockRule[]{
 const rules:unknown=JSON.parse(text)
 if(!Array.isArray(rules)||rules.length>20)throw Error('Use an array of up to 20 response rules.')
 const names=new Set<string>()
 for(const value of rules){
  if(!value||typeof value!=='object'||Array.isArray(value))throw Error('Each rule must be an object.')
  const rule=value as MockRule
  if(Object.keys(rule).some(k=>!['name','priority','request','response'].includes(k)))throw Error('Rules support name, priority, request and response. Method, URL and namespace are managed by the operation.')
  if(typeof rule.name!=='string'||!rule.name.trim()||new TextEncoder().encode(rule.name).length>128||names.has(rule.name))throw Error('Rule names must be nonempty, unique and at most 128 bytes.')
  names.add(rule.name)
  if(!Number.isInteger(rule.priority)||rule.priority<1||rule.priority>1000)throw Error('Priority must be 1–1000; lower values match first.')
  if(rule.request!==undefined&&(!rule.request||typeof rule.request!=='object'||Array.isArray(rule.request)))throw Error('Request must be a matcher object.')
  if(rule.request&&Object.keys(rule.request).some(k=>!['headers','queryParameters','bodyPatterns'].includes(k)))throw Error('Request matching supports headers, queryParameters and bodyPatterns. Method and path come from the operation.')
  if(!rule.response||typeof rule.response!=='object'||Array.isArray(rule.response)||Object.keys(rule.response).some(k=>!responseKeys.includes(k)))throw Error('Response supports status, headers, jsonBody, body, fixedDelayMilliseconds and response-template transformers.')
  const response=rule.response
  if(!Number.isInteger(response.status)||response.status<200||response.status>599)throw Error('Response status must be 200–599.')
  if(response.headers!==undefined&&(!response.headers||typeof response.headers!=='object'||Array.isArray(response.headers)||Object.entries(response.headers).some(([name,value])=>!/^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(name)||typeof value!=='string'||/[\r\n]/.test(value))||new Set(Object.keys(response.headers).map(name=>name.toLowerCase())).size!==Object.keys(response.headers).length))throw Error('Response headers need unique valid names and single-line string values.')
  if(response.body!==undefined&&(typeof response.body!=='string'||response.jsonBody!==undefined))throw Error('Use a string body or jsonBody, not both.')
  if(response.fixedDelayMilliseconds!==undefined&&(!Number.isInteger(response.fixedDelayMilliseconds)||response.fixedDelayMilliseconds<0||response.fixedDelayMilliseconds>120000))throw Error('Delay must be 0–120000 milliseconds.')
  if(response.transformers!==undefined&&JSON.stringify(response.transformers)!=='["response-template"]')throw Error('Only the response-template transformer is supported.')
 }
 return rules as MockRule[]
}
