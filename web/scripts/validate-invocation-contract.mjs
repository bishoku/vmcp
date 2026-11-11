import assert from 'node:assert/strict'
import fs from 'node:fs'
import YAML from 'yaml'
import Ajv from 'ajv'
import addFormats from 'ajv-formats'

const contract = YAML.parse(
  fs.readFileSync(new URL('../../api/invocation-service.openapi.yaml', import.meta.url), 'utf8'),
)
assert.equal(contract.openapi, '3.0.3')
const ajv = new Ajv({ strict: false, allErrors: true })
addFormats(ajv)
const id = 'urn:vmcp:invocation-service'
ajv.addSchema({ $id: id, components: contract.components })
const operation = contract.paths['/check'].post
for (const content of [
  operation.requestBody.content['application/json'],
  operation.responses['200'].content['application/json'],
]) {
  const validate = ajv.compile({ $ref: id + content.schema.$ref })
  for (const [name, example] of Object.entries(content.examples)) {
    assert.ok(validate(example.value), `${name}: ${ajv.errorsText(validate.errors)}`)
  }
}
const decision = ajv.compile({ $ref: id + '#/components/schemas/Decision' })
for (const value of [
  { status: 'allowed' },
  { status: 'pending', approvalId: 'id' },
  { status: 'denied', extra: true },
  { status: 'unknown' },
]) {
  assert.equal(decision(value), false, 'Invalid decision accepted by the contract')
}
console.log('Invocation service contract examples and negative cases validated.')
