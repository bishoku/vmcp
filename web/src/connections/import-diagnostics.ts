/** Keep the server's exact reason visible; add a concrete next action. */
export function importResolution(reason: string): string {
 const text=reason.toLowerCase()
 if(text.includes('header, cookie'))return 'Configure fixed service headers or supported request header mappings first. Remove declarations from an import copy only if the API contract is preserved. Dynamic cookie/form parameters are not supported.'
 if(text.includes('recursive')||text.includes('references'))return 'Expand local references and remove recursive cycles in a copy of the schema, or define a bounded inline schema manually.'
 if(text.includes('discriminator')||text.includes('dynamic'))return 'Replace this construct with an explicit inline schema before importing.'
 if(text.includes('write-only'))return 'Review this write-only field and configure a safe response projection manually. Removing its schema marker alone does not remove sensitive data from the actual response.'
 if(text.includes('application/json')||text.includes('multipart')||text.includes('binary'))return 'This adapter supports JSON requests and responses. Choose a JSON media type in a copy of the definition; binary/form uploads are not supported.'
 if(text.includes('servers')||text.includes('schemes'))return 'Create a separate upstream for this endpoint and use its base URL; remove the operation-specific server override from the import copy.'
 if(text.includes('2xx'))return 'Declare a concrete supported success status, for example 200 or 201, with its response schema.'
 if(text.includes('parameter'))return 'Use scalar string, integer, number or boolean path/query parameters with supported serialization. Define more complex inputs manually.'
 if(text.includes('complexity')||text.includes('size limit')||text.includes('oversized'))return 'Split the API definition or simplify this operation’s schema before importing.'
 if(text.includes('http method'))return 'REST tools support GET, POST, PUT, PATCH and DELETE.'
 return 'Correct the reported definition or configure this operation manually. Other available operations can still be imported.'
}
