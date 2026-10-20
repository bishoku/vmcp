import { expect, test } from 'vitest'
import { normalizeGuidance, renderGuidance } from './guidance'
test('preview follows stable tool references through alias changes without interpreting arguments',()=>{
 const g=normalizeGuidance({toolReferences:[{key:'read',upstream:'alpha',tool:'lookup'}]})
 expect(renderGuidance('{{tool:read}} {{arg:topic}}',g,[{upstream:'alpha',tool:'lookup',alias:'renamed'}],{topic:'{{tool:secret}}'})).toBe('renamed {{tool:secret}}')
 expect(renderGuidance('{{tool:read}}',g,[])).toBe('[Unavailable tool: read]')
})
test('normalizes empty saved collections',()=>{
 expect(normalizeGuidance()).toEqual({instructions:'',resources:[],prompts:[],toolReferences:[],upstreamInstructions:[]})
})
test('preview bounds repeated large arguments before expanding the whole template',()=>{
 expect(renderGuidance('{{arg:text}}'.repeat(1000),normalizeGuidance(),[],{text:'x'.repeat(65536)})).toBe('[Preview exceeds 100,000 characters]')
})
