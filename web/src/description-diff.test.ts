import { expect, test } from 'vitest'
import { descriptionDiff } from './description-diff'

test('aligns unchanged lines around replacements, insertions and deletions', () => {
  expect(descriptionDiff('start\nold\nkeep\nremove\nend', 'start\nnew\nkeep\nend\nadded').lines).toEqual([
    {kind:'same',text:'start'}, {kind:'removed',text:'old'}, {kind:'added',text:'new'},
    {kind:'same',text:'keep'}, {kind:'removed',text:'remove'}, {kind:'same',text:'end'}, {kind:'added',text:'added'},
  ])
})

test('preserves whitespace, blank lines, Unicode, HTML literals and final newlines exactly', () => {
  for (const [source,target] of [
    ['', ''], ['', '🙂\n'], ['one\n', ''], ['same\n', 'same\n'],
    ['  ş🙂\n\n<script>source</script>\n', ' ş🙂\n\n<script>target</script>'],
    ['a\nb\na\nb', 'b\na\nb\na'], ['line\r\n', 'line\n'],
  ]) {
    const {lines}=descriptionDiff(source,target)
    expect(lines.filter(line=>line.kind!=='added').map(line=>line.text).join('\n')).toBe(source)
    expect(lines.filter(line=>line.kind!=='removed').map(line=>line.text).join('\n')).toBe(target)
  }
})

test('bounds work on large changes while retaining exact text and common context', () => {
  const source='start\n'+'old\n'.repeat(2000)+'end'
  const target='start\n'+'new\n'.repeat(2000)+'end'
  const {lines,grouped}=descriptionDiff(source,target)
  expect(grouped).toBe(true)
  expect(lines[0]).toEqual({kind:'same',text:'start'})
  expect(lines.at(-1)).toEqual({kind:'same',text:'end'})
  expect(lines.filter(line=>line.kind!=='added').map(line=>line.text).join('\n')).toBe(source)
  expect(lines.filter(line=>line.kind!=='removed').map(line=>line.text).join('\n')).toBe(target)
})
