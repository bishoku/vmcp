import type {MockRule} from '../api'
import {objectSchema} from './schema-editor'

export type Condition = {source:'queryParameters'|'headers'|'bodyPatterns';field:string;operator:'equalTo'|'contains'|'matches'|'present'|'absent'|'anyEqualTo';value:string}
export type ConditionEntry = {source:Condition['source'];key:string|number;raw:unknown;condition?:Condition}
const ops=['equalTo','contains','matches'] as const
export function conditions(request:MockRule['request']):ConditionEntry[]{
 const result:ConditionEntry[]=[]
 for(const source of ['queryParameters','headers'] as const){
  for(const [key,raw] of Object.entries(objectSchema(request?.[source]))){
   const m=objectSchema(raw),keys=Object.keys(m)
   const operator=ops.find(op=>keys.length===1&&typeof m[op]==='string')
   const condition:Condition|undefined=operator?{source,field:key,operator,value:m[operator] as string}:keys.length===1&&typeof m.absent==='boolean'?{source,field:key,operator:m.absent?'absent':'present',value:''}:undefined
   result.push({source,key,raw,condition})
  }
 }
 if(Array.isArray(request?.bodyPatterns))request.bodyPatterns.forEach((raw,key)=>{
  const m=objectSchema(raw),path=m.matchesJsonPath,p=objectSchema(path),keys=Object.keys(p)
  const operator=ops.find(op=>keys.length===2&&typeof p.expression==='string'&&typeof p[op]==='string')
  const condition:Condition|undefined=Object.keys(m).length!==1?undefined:typeof path==='string'?{source:'bodyPatterns',field:path,operator:'present',value:''}:operator?{source:'bodyPatterns',field:p.expression as string,operator,value:p[operator] as string}:undefined
  result.push({source:'bodyPatterns',key,raw,condition})
 })
 return result
}
export function removeCondition(request:MockRule['request'],entry:ConditionEntry):MockRule['request']{
 const next={...request}
 if(entry.source==='bodyPatterns')next.bodyPatterns=(next.bodyPatterns as unknown[]).filter((_,i)=>i!==entry.key)
 else {const map={...objectSchema(next[entry.source])};delete map[entry.key];next[entry.source]=map}
 return next
}
export function putCondition(request:MockRule['request'],condition:Condition,editing?:ConditionEntry):MockRule['request']{
 if(!condition.field.trim())throw Error('Enter a field name or JSONPath.')
 if(condition.source==='bodyPatterns'&&!condition.field.startsWith('$'))throw Error('Body JSONPath must start with $.')
 if(condition.source==='bodyPatterns'&&condition.operator==='absent')throw Error('Use an explicit JSONPath predicate for missing body fields in JSON view.')
 // Nested matchers coerce selected JSON to text; array equality must use a filter instead.
 if(condition.source==='bodyPatterns'&&condition.field.includes('[*]')&&condition.operator!=='present'&&condition.operator!=='anyEqualTo')throw Error('For array values use a JSONPath filter, such as $.items[?(@.productId == "SKU")], with Is present. A wildcard selects an array, not an individual value.')
 if(condition.operator==='anyEqualTo'){
  if(condition.source!=='bodyPatterns'||!condition.field.includes('[*]'))throw Error('Choose a body schema path containing an array wildcard [*].')
  const index=condition.field.lastIndexOf('[*]'),prefix=condition.field.slice(0,index),suffix=condition.field.slice(index+3)
  let value:unknown=condition.value
  try{value=JSON.parse(condition.value)}catch{/* Unquoted values are strings. */}
  if(value!==null&&typeof value==='object')throw Error('Array comparison supports string, number, boolean or null values.')
  condition={...condition,field:`${prefix}[?(@${suffix} == ${JSON.stringify(value)})]`,operator:'present'}
 }
 const next=editing?removeCondition(request,editing):{...request}
 const matcher=condition.operator==='present'||condition.operator==='absent'?{absent:condition.operator==='absent'}:{[condition.operator]:condition.value}
 if(condition.source==='bodyPatterns')next!.bodyPatterns=[...(Array.isArray(next?.bodyPatterns)?next.bodyPatterns:[]),{matchesJsonPath:condition.operator==='present'?condition.field:{expression:condition.field,...matcher}}]
 else {
  const map=objectSchema(next?.[condition.source])
  const duplicate=Object.keys(map).some(key=>condition.source==='headers'?key.toLowerCase()===condition.field.toLowerCase():key===condition.field)
  if(duplicate)throw Error('This field already has a condition. Edit it or use JSON view for combined matchers.')
  next![condition.source]={...map,[condition.field]:matcher}
 }
 return next
}
export function bodyPaths(schema:unknown):string[]{
 const result:string[]=[]
 const visit=(value:unknown,path:string,depth:number)=>{
  if(depth>20||result.length>=200)return
  const s=objectSchema(value)
  if(path!=='$')result.push(path)
  for(const [key,child] of Object.entries(objectSchema(s.properties)))visit(child,`${path}[${JSON.stringify(key)}]`,depth+1)
  if(s.items)visit(s.items,`${path}[*]`,depth+1)
 }
 visit(schema,'$',0);return result
}
export function isFallback(rule:MockRule){
 return !conditions(rule.request).length&&Object.entries(rule.request??{}).every(([key,value])=>['headers','queryParameters'].includes(key)?Object.keys(objectSchema(value)).length===0:key==='bodyPatterns'&&Array.isArray(value)&&value.length===0)
}
export function orderedRules(rules:MockRule[]){return rules.map((rule,index)=>({rule,index})).sort((a,b)=>a.rule.priority-b.rule.priority||a.index-b.index)}
export function moveRule(rules:MockRule[],index:number,direction:-1|1):MockRule[]{
 const ordered=orderedRules(rules),position=ordered.findIndex(item=>item.index===index),target=position+direction
 if(target<0||target>=ordered.length)return rules
 ;[ordered[position],ordered[target]]=[ordered[target],ordered[position]]
 return ordered.map(({rule},i)=>({...rule,priority:i+1}))
}
