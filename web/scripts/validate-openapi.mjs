import fs from 'node:fs'
import YAML from 'yaml'
import Ajv from 'ajv/dist/2020.js'
import addFormats from 'ajv-formats'
const spec=YAML.parse(fs.readFileSync(new URL('../../api/openapi.yaml',import.meta.url),'utf8'))
const ajv=new Ajv({strict:false,allErrors:true});addFormats(ajv)
const ids=new Set()
function walk(value){
 if(!value || typeof value!=='object')return
 if(value.$ref?.startsWith('#/')){let target=spec;for(const part of value.$ref.slice(2).split('/'))target=target?.[part.replaceAll('~1','/').replaceAll('~0','~')];if(!target)throw new Error(`Unresolved ${value.$ref}`)}
 if(value.operationId){if(ids.has(value.operationId))throw new Error('Duplicate operationId');ids.add(value.operationId)}
 for(const v of Object.values(value))walk(v)
}
walk(spec)
const examples=JSON.parse(fs.readFileSync(new URL('../../api/examples.json',import.meta.url),'utf8'))
for(const [schema,example]of Object.entries(examples)){
 const validate=ajv.compile({$ref:`#/components/schemas/${schema}`,components:spec.components})
 if(!validate(example))throw new Error(`${schema}: ${ajv.errorsText(validate.errors)}`)
}
// Unknown fields still fail after composed schemas are repaired.
const create=ajv.compile({$ref:'#/components/schemas/UpstreamCreate',components:spec.components})
if(create({...examples.UpstreamCreate,unexpected:true}))throw new Error('Create schema accepts unknown properties')
console.log(`Validated ${ids.size} operations and ${Object.keys(examples).length} captured examples`)
