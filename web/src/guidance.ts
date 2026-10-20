import type { Guidance, ToolSelection } from './api'
export function normalizeGuidance(g?: Partial<Guidance> | null): Guidance {
  return { instructions:g?.instructions??'',resources:g?.resources??[],prompts:(g?.prompts??[]).map(p=>({...p,arguments:p.arguments??[],messages:p.messages??[]})),upstreamInstructions:g?.upstreamInstructions??[],toolReferences:g?.toolReferences??[] }
}
// Preview mirrors literal substitution and bounds expansion before allocating it.
export function renderGuidance(text:string,g:Guidance,tools:ToolSelection[],args:Record<string,string>={}) {
  const parts:string[]=[]
  let cursor=0, count=0
  const append=(value:string)=>{
    count+=Array.from(value).length
    if(count>100000)return false
    parts.push(value)
    return true
  }
  for(const match of text.matchAll(/\{\{(tool|arg):([a-zA-Z0-9_.-]+)\}\}/g)) {
    if(!append(text.slice(cursor,match.index)))return '[Preview exceeds 100,000 characters]'
    const [,kind,key]=match
    let value=args[key]??`[Missing argument: ${key}]`
    if(kind==='tool') {
      const ref=g.toolReferences.find(r=>r.key===key)
      const tool=ref&&tools.find(t=>t.upstream===ref.upstream&&t.tool===ref.tool)
      value=tool?(tool.alias||`${tool.upstream}__${tool.tool}`):`[Unavailable tool: ${key}]`
    }
    if(!append(value))return '[Preview exceeds 100,000 characters]'
    cursor=match.index+match[0].length
  }
  return append(text.slice(cursor))?parts.join(''):'[Preview exceeds 100,000 characters]'
}
