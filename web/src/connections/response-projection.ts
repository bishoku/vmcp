export type FieldPath = string[]
type Tree = { whole: boolean; children: Map<string,Tree> }
export const projectionObject = (value:unknown):Record<string,unknown> => value!==null&&typeof value==='object'&&!Array.isArray(value)?value as Record<string,unknown>:{}
export function containerType(value:unknown):string {
 const type=projectionObject(value).type
 return typeof type==='string'?type:Array.isArray(type)&&type.length===2&&type.includes('null')?String(type.find(t=>t!=='null')):''
}
const complex=['allOf','anyOf','oneOf','not','if','then','else','prefixItems','dependentSchemas','dependencies']
export function objectProperties(value:unknown,depth=0):Record<string,unknown>|undefined {
 if(depth>64)return
 const schema=projectionObject(value)
 if(complex.some(key=>Object.hasOwn(schema,key)))return
 if(containerType(schema)==='array')return objectProperties(schema.items,depth+1)
 if(containerType(schema)==='object')return projectionObject(schema.properties)
}
export const pathKey=(path:FieldPath)=>JSON.stringify(path)
export const isPrefix=(parent:FieldPath,child:FieldPath)=>parent.length<=child.length&&parent.every((part,i)=>part===child[i])
export const effectivePaths=(fields:string[]=[],paths:FieldPath[]=[])=>paths.length?paths:fields.map(field=>[field])
function project(value:unknown,node:Tree,depth=0):unknown {
 if(node.whole)return value
 if(depth>64)throw Error('Response projection nesting exceeds 64 levels.')
 const source=projectionObject(value)
 const keyword=complex.find(key=>Object.hasOwn(source,key))
 if(keyword)throw Error(`Cannot select inside a schema containing ${keyword}; select the whole field instead.`)
 const result:Record<string,unknown>={type:source.type}
 for(const key of ['title','description'])if(Object.hasOwn(source,key))result[key]=source[key]
 if(containerType(source)==='array'){
  if(!Object.hasOwn(source,'items'))throw Error('Response array selection requires a declared items schema.')
  result.items=project(source.items,node,depth+1)
  for(const key of ['minItems','maxItems'])if(Object.hasOwn(source,key))result[key]=source[key]
  return result
 }
 if(containerType(source)!=='object')throw Error('Response field paths must traverse object or array schemas (optionally nullable).')
 const properties=projectionObject(source.properties)
 const selected=[...node.children].map(([name,child])=>{
  if(!Object.hasOwn(properties,name))throw Error('Some selected fields are no longer declared in the response schema. Remove them from the selection or restore the fields.')
  return [name,project(properties[name],child,depth+1)]
 })
 result.properties=Object.fromEntries(selected)
 result.required=(Array.isArray(source.required)?source.required:[]).filter(name=>typeof name==='string'&&node.children.has(name))
 result.additionalProperties=false
 return result
}
export function responseProjection(schema:unknown,fields:string[]=[],paths:FieldPath[]=[]):{schema:unknown;error:string} {
 try {
  if(fields.length&&paths.length)throw Error('responseFields and responseFieldPaths cannot both be set.')
  if(!fields.length&&!paths.length)return {schema,error:''}
  if(fields.length&&projectionObject(schema).type!=='object')throw Error('Selected response fields require an object response schema. Restore the schema or choose the full response.')
  if(paths.length>256)throw Error('Configure at most 256 response field paths.')
  const root:Tree={whole:false,children:new Map()}
  for(const path of effectivePaths(fields,paths)){
   if(!path.length||path.length>32)throw Error('Response field paths require between 1 and 32 property names.')
   let current=root
   for(const name of path){
    if(paths.length&&new TextEncoder().encode(name).length>1024)throw Error('Response path property names must not exceed 1024 bytes.')
    if(current.whole)throw Error('Response field paths must not overlap.')
    if(!current.children.has(name))current.children.set(name,{whole:false,children:new Map()})
    current=current.children.get(name)!
   }
   if(current.whole||current.children.size)throw Error('Response field paths must be unique and must not overlap.')
   current.whole=true
  }
  return {schema:project(schema,root),error:''}
 }catch(error){return {schema:{},error:(error as Error).message}}
}
export function schemaAtPath(root:unknown,path:FieldPath):unknown {
 let schema=root
 for(const name of path){const props=objectProperties(schema);if(!props||!Object.hasOwn(props,name))return undefined;schema=props[name]}
 return schema
}
// Narrow a previously selected whole branch by expanding its declared siblings.
function subtract(schema:unknown,prefix:FieldPath,target:FieldPath):FieldPath[] {
 if(prefix.length===target.length)return []
 const properties=objectProperties(schemaAtPath(schema,prefix))??{}
 return Object.keys(properties).flatMap(name=>{
  const child=[...prefix,name]
  return isPrefix(child,target)?subtract(schema,child,target):[child]
 })
}
export function togglePath(schema:unknown,paths:FieldPath[],path:FieldPath,checked:boolean):FieldPath[] {
 if(checked){if(paths.some(p=>isPrefix(p,path)))return paths;return [...paths.filter(p=>!isPrefix(path,p)),path]}
 return paths.flatMap(p=>isPrefix(path,p)?[]:isPrefix(p,path)?subtract(schema,p,path):[p])
}
export function displayPath(schema:unknown,path:FieldPath):string {
 let current=schema,display=''
 for(const name of path){
  while(containerType(current)==='array'){display+='[]';current=projectionObject(current).items}
  display+=/^[A-Za-z_][A-Za-z0-9_]*$/.test(name)?`${display?'.':''}${name}`:`[${JSON.stringify(name)}]`
  current=projectionObject(projectionObject(current).properties)[name]
 }
 return display
}
