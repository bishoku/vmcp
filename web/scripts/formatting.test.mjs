import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import {
  chmodSync,
  copyFileSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  symlinkSync,
  unlinkSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const root = fileURLToPath(new URL('../../', import.meta.url))
const npm = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const sourcePackage = JSON.parse(readFileSync(join(root, 'web/package.json'), 'utf8'))
const run = (cwd, command, args) =>
  spawnSync(command, args, {
    cwd,
    encoding: 'utf8',
    shell: process.platform === 'win32' && command === npm,
  })
const output = (result) => result.stdout + result.stderr
function success(result) {
  assert.equal(result.status, 0, output(result))
}
function fixture(t, { git = true } = {}) {
  const dir = mkdtempSync(join(tmpdir(), 'vmcp-format-'))
  t.after(() => rmSync(dir, { recursive: true, force: true }))
  for (const file of [
    '.editorconfig',
    '.gitattributes',
    '.githooks/pre-commit',
    'web/.prettierrc.json',
    'web/.prettierignore',
    'web/.gitignore',
    'web/scripts/check-staged-format.mjs',
    'web/scripts/install-hooks.mjs',
  ]) {
    mkdirSync(dirname(join(dir, file)), { recursive: true })
    copyFileSync(join(root, file), join(dir, file))
  }
  chmodSync(join(dir, '.githooks/pre-commit'), 0o755)
  mkdirSync(join(dir, 'web/src'))
  symlinkSync(join(root, 'web/node_modules'), join(dir, 'web/node_modules'), 'junction')
  const pkg = structuredClone(sourcePackage)
  for (const name of ['build', 'dev', 'preview'])
    pkg.scripts[name] = 'node -e "console.log(\'command ran\')"'
  writeFileSync(join(dir, 'web/package.json'), JSON.stringify(pkg, null, 2) + '\n')
  writeFileSync(join(dir, 'web/src/example.ts'), 'export const value = 1\n')
  if (git) {
    success(run(dir, 'git', ['init', '-q']))
    success(run(dir, 'git', ['config', 'commit.gpgsign', 'false']))
    success(run(dir, 'git', ['config', 'user.name', 'Formatting test']))
    success(run(dir, 'git', ['config', 'user.email', 'formatting@example.invalid']))
    success(run(dir, npm, ['run', 'prepare', '--prefix', 'web']))
    success(run(dir, 'git', ['add', '.']))
    success(run(dir, 'git', ['commit', '-qm', 'Baseline']))
    assert.equal(run(dir, 'git', ['config', '--get', 'core.hooksPath']).stdout.trim(), '.githooks')
  }
  return dir
}

test('pre-commit rejects a malformed staged blob even when the working copy is formatted', (t) => {
  const dir = fixture(t)
  const file = join(dir, 'web/src/example.ts')
  writeFileSync(file, 'export const value=2\n')
  success(run(dir, 'git', ['add', 'web/src/example.ts']))
  writeFileSync(file, 'export const value = 2\n')
  const rejected = run(dir, 'git', ['commit', '-qm', 'Should fail'])
  assert.notEqual(rejected.status, 0)
  assert.match(output(rejected), /Unformatted staged file: web\/src\/example.ts/)
  assert.equal(run(dir, 'git', ['show', ':web/src/example.ts']).stdout, 'export const value=2\n')
  success(run(dir, 'git', ['add', 'web/src/example.ts']))
  success(run(dir, 'git', ['commit', '-qm', 'Formatted']))
})

test('renames and paths with spaces are checked, while deleted files are skipped', (t) => {
  const dir = fixture(t)
  success(run(dir, 'git', ['mv', 'web/src/example.ts', 'web/src/with spaces.ts']))
  writeFileSync(join(dir, 'web/src/with spaces.ts'), 'export const value=3\n')
  success(run(dir, 'git', ['add', 'web/src/with spaces.ts']))
  assert.notEqual(run(dir, 'git', ['commit', '-qm', 'Unformatted rename']).status, 0)
  writeFileSync(join(dir, 'web/src/with spaces.ts'), 'export const value = 3\n')
  success(run(dir, 'git', ['add', 'web/src/with spaces.ts']))
  success(run(dir, 'git', ['commit', '-qm', 'Formatted rename']))
  success(run(dir, 'git', ['rm', 'web/src/with spaces.ts']))
  success(run(dir, 'git', ['commit', '-qm', 'Deletion']))
})

test('ignored artifacts are excluded and syntax errors reject commits', (t) => {
  const dir = fixture(t)
  mkdirSync(join(dir, 'web/dist'))
  writeFileSync(join(dir, 'web/dist/generated.js'), 'const unformatted=1')
  success(run(dir, 'git', ['add', '-f', 'web/dist/generated.js']))
  success(run(dir, 'git', ['commit', '-qm', 'Ignored generated fixture']))
  writeFileSync(join(dir, 'web/src/example.ts'), 'export const =\n')
  success(run(dir, 'git', ['add', 'web/src/example.ts']))
  const rejected = run(dir, 'git', ['commit', '-qm', 'Invalid syntax'])
  assert.notEqual(rejected.status, 0)
  assert.match(output(rejected), /Cannot check staged file/)
})

test('missing web dependencies block web commits but leave other changes usable', (t) => {
  const dir = fixture(t)
  unlinkSync(join(dir, 'web/node_modules'))
  writeFileSync(join(dir, 'README.md'), 'Documentation\n')
  success(run(dir, 'git', ['add', 'README.md']))
  success(run(dir, 'git', ['commit', '-qm', 'Docs only']))
  writeFileSync(join(dir, 'web/src/example.ts'), 'export const value = 4\n')
  success(run(dir, 'git', ['add', 'web/src/example.ts']))
  const rejected = run(dir, 'git', ['commit', '-qm', 'Missing formatter'])
  assert.notEqual(rejected.status, 0)
  assert.match(output(rejected), /npm ci --prefix web/)
})

test('source archives and Docker dependency layers can prepare without Git metadata', (t) => {
  const dir = fixture(t, { git: false })
  success(run(dir, npm, ['run', 'prepare', '--prefix', 'web']))
})

for (const command of ['dev', 'build', 'preview'])
  test(`${command} formats first; format:check never rewrites the violation`, (t) => {
    const dir = fixture(t, { git: false })
    const file = join(dir, 'web/src/example.ts')
    const bad = 'export const value=5\n'
    writeFileSync(file, bad)
    const rejected = run(dir, npm, ['run', 'format:check', '--prefix', 'web'])
    assert.notEqual(rejected.status, 0)
    assert.equal(readFileSync(file, 'utf8'), bad)
    const result = run(dir, npm, ['run', command, '--prefix', 'web'])
    success(result)
    assert.match(output(result), /command ran/)
    assert.equal(readFileSync(file, 'utf8'), 'export const value = 5\n')
    success(run(dir, npm, ['run', 'format:check', '--prefix', 'web']))
  })

test('tracked attributes preserve LF checkout and binary bytes with autocrlf enabled', (t) => {
  const dir = fixture(t)
  const image = Buffer.from([137, 80, 78, 71, 0, 13, 10, 26, 10])
  writeFileSync(join(dir, 'web/src/asset.png'), image)
  success(run(dir, 'git', ['add', 'web/src/asset.png']))
  success(run(dir, 'git', ['commit', '-qm', 'Binary fixture']))
  success(run(dir, 'git', ['config', 'core.autocrlf', 'true']))
  success(run(dir, 'git', ['checkout-index', '--force', '--all']))
  for (const file of ['web/src/example.ts', '.githooks/pre-commit']) {
    assert.equal(readFileSync(join(dir, file), 'utf8').includes('\r\n'), false)
  }
  assert.deepEqual(readFileSync(join(dir, 'web/src/asset.png')), image)
})
