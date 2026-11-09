import { execFileSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { join } from 'node:path'

const root = fileURLToPath(new URL('../../', import.meta.url))
const web = join(root, 'web')
const git = (...args) => execFileSync('git', args, { cwd: root, encoding: 'utf8' })
const files = git('diff', '--cached', '--name-only', '--diff-filter=ACMR', '-z', '--', 'web/')
  .split('\0')
  .filter(Boolean)

if (files.length) {
  let prettier
  try {
    prettier = await import('prettier')
  } catch {
    console.error('Web formatting check requires dependencies. Run npm ci --prefix web.')
    process.exit(1)
  }
  process.chdir(web)
  let failed = false
  for (const file of files) {
    const filepath = join(root, file)
    const info = await prettier.getFileInfo(filepath, {
      ignorePath: [join(web, '.gitignore'), join(web, '.prettierignore')],
    })
    if (info.ignored || !info.inferredParser) continue
    try {
      // Inspect the index, not the working copy: partially staged files must also pass.
      const source = git('show', `:${file}`)
      const options = await prettier.resolveConfig(filepath, { editorconfig: true })
      if (!(await prettier.check(source, { ...options, filepath }))) {
        console.error(`Unformatted staged file: ${file}`)
        failed = true
      }
    } catch (error) {
      console.error(`Cannot check staged file ${file}: ${error.message}`)
      failed = true
    }
  }
  if (failed) {
    console.error(
      'Run npm run format --prefix web, review and stage the intended changes, then retry.',
    )
    process.exit(1)
  }
}
