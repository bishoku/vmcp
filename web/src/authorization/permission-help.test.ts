/// <reference types="node" />
// @vitest-environment node
import { readFileSync } from 'node:fs'
import { expect, test } from 'vitest'
import { describePermission, permissionHelp } from './permission-help'

test('every permission in the server catalog has explicit UI guidance', () => {
  const catalogSource = readFileSync('../core/internal/authz/catalog.go', 'utf8')
  const ids = new Set<string>()
  for (const match of catalogSource.matchAll(/add\("([^"]+)", "([^"]+)"/g)) {
    for (const action of match[2].split(' ')) ids.add(`${match[1]}.${action}`)
  }
  for (const family of ['roles', 'assignments', 'mappings']) {
    for (const action of ['read', 'manage']) ids.add(`access.${family}.${action}`)
  }
  expect(ids.size).toBeGreaterThan(90)
  expect(Object.keys(permissionHelp).sort()).toEqual([...ids].sort())
  for (const id of ids) {
    const help = describePermission(id)
    expect(help.title).not.toBe(id)
    expect(help.description.length).toBeGreaterThan(20)
  }
})

test('unknown permissions do not invent a capability description', () => {
  expect(describePermission('future.action')).toEqual({
    title: 'future.action',
    description: expect.stringContaining('No description is available'),
  })
})
