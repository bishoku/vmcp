// Runs in the Node test process for UI-only suites. Live tests exercise the Go endpoint.
import Ajv from 'ajv/dist/2020.js'

export function schemaCheckFixture(checks: { schema: unknown; sample?: unknown }[]) {
  const ajv = new Ajv({ strict: false, allErrors: true, validateFormats: false })
  return {
    issues: checks.map(({ schema, sample }) => {
      try {
        const validate = ajv.compile(schema as object)
        if (sample === undefined || validate(sample)) return []
        return (validate.errors ?? []).map(
          (e) =>
            `${e.instancePath || '/'}: ${e.message}${e.keyword === 'required' ? ' (' + e.params.missingProperty + ')' : ''}`,
        )
      } catch (err) {
        return [String(err)]
      }
    }),
  }
}
