import fs from 'node:fs'
import YAML from 'yaml'

// Checked-in assets let Go builds (including Docker) run without Node or repository files.
const yaml = fs.readFileSync(new URL('../../api/openapi.yaml', import.meta.url), 'utf8')
const assets = new URL('../../core/internal/controlplane/spec/', import.meta.url)
const files = { 'openapi.yaml': yaml, 'openapi.json': JSON.stringify(YAML.parse(yaml), null, 2) + '\n' }
if (!process.argv.includes('--check')) fs.mkdirSync(assets, { recursive: true })
for (const [name, content] of Object.entries(files)) {
  const path = new URL(name, assets)
  if (process.argv.includes('--check')) {
    if (fs.readFileSync(path, 'utf8') !== content) throw new Error(`${name} is stale; run npm run generate:api in web`)
  } else fs.writeFileSync(path, content)
}
