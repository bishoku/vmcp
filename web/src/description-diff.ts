export type DescriptionDiffLine = { kind: 'same' | 'removed' | 'added'; text: string }

/** Bound the LCS work for long prompts; large changed blocks retain exact text. */
export function descriptionDiff(source: string, target: string): {lines: DescriptionDiffLine[]; grouped: boolean} {
  const before = source ? source.split('\n') : []
  const after = target ? target.split('\n') : []
  let start = 0
  while (start < before.length && start < after.length && before[start] === after[start]) start++
  let end = 0
  while (end < before.length - start && end < after.length - start && before[before.length - end - 1] === after[after.length - end - 1]) end++
  const left = before.slice(start, before.length - end)
  const right = after.slice(start, after.length - end)
  const lines: DescriptionDiffLine[] = before.slice(0, start).map(text => ({kind: 'same', text}))
  const grouped = left.length * right.length > 500_000
  if (grouped) {
    for (const text of left) lines.push({kind: 'removed', text})
    for (const text of right) lines.push({kind: 'added', text})
  } else {
    const width = right.length + 1
    const lengths = new Uint32Array((left.length + 1) * width)
    for (let i = left.length - 1; i >= 0; i--) {
      for (let j = right.length - 1; j >= 0; j--) {
        lengths[i * width + j] = left[i] === right[j] ? 1 + lengths[(i + 1) * width + j + 1] : Math.max(lengths[(i + 1) * width + j], lengths[i * width + j + 1])
      }
    }
    let i = 0, j = 0
    while (i < left.length || j < right.length) {
      if (i < left.length && j < right.length && left[i] === right[j]) {
        lines.push({kind: 'same', text: left[i++]}); j++
      } else if (i < left.length && (j === right.length || lengths[(i + 1) * width + j] >= lengths[i * width + j + 1])) {
        lines.push({kind: 'removed', text: left[i++]})
      } else {
        lines.push({kind: 'added', text: right[j++]})
      }
    }
  }
  for (const text of before.slice(before.length - end)) lines.push({kind: 'same', text})
  return {lines, grouped}
}
