import { useMemo } from 'react'
import { descriptionDiff, type DescriptionDiffLine } from './description-diff'

type Cell = {text: string; number: number; changed: boolean}
type Row = {left?: Cell; right?: Cell}
function comparisonRows(lines: DescriptionDiffLine[]): Row[] {
  const rows: Row[] = []
  let leftNumber = 0, rightNumber = 0, index = 0
  while (index < lines.length) {
    if (lines[index].kind === 'same') {
      const text = lines[index++].text
      rows.push({left: {text, number: ++leftNumber, changed: false}, right: {text, number: ++rightNumber, changed: false}})
      continue
    }
    const removed: Cell[] = [], added: Cell[] = []
    while (index < lines.length && lines[index].kind !== 'same') {
      const line = lines[index++]
      if (line.kind === 'removed') removed.push({text: line.text, number: ++leftNumber, changed: true})
      else added.push({text: line.text, number: ++rightNumber, changed: true})
    }
    for (let i = 0; i < Math.max(removed.length, added.length); i++) rows.push({left: removed[i], right: added[i]})
  }
  return rows
}

export default function DescriptionComparison({upstream, override}: {upstream: string; override: string}) {
  const diff = useMemo(() => descriptionDiff(upstream, override || upstream), [upstream, override])
  const rows = useMemo(() => comparisonRows(diff.lines), [diff.lines])
  const removed = diff.lines.filter(line => line.kind === 'removed').length
  const added = diff.lines.filter(line => line.kind === 'added').length
  return <section aria-label="Description comparison" className="space-y-3">
    <div className="flex flex-wrap items-center justify-between gap-3 text-xs leading-relaxed">
      <p className="text-ink-muted">{override ? 'Current upstream compared with the override in this draft. Whitespace is preserved.' : 'No override. The agent inherits the upstream description.'}</p>
      <p role="status" className="font-medium">{added || removed ? `${removed} removed · ${added} added lines` : 'No differences'}</p>
    </div>
    {diff.grouped && <p className="text-xs text-ink-muted">Large changed sections are grouped as removed and added blocks; all text is shown.</p>}
    <div role="table" aria-label="Upstream and override line differences" className="overflow-hidden rounded-xl border border-line text-xs">
      <div role="row" className="grid grid-cols-2 border-b border-line bg-canvas font-semibold"><div role="columnheader" className="border-r border-line p-3">Upstream · removed lines marked −</div><div role="columnheader" className="p-3">{override ? 'Override draft' : 'Inherited description'} · added lines marked +</div></div>
      {rows.map((row, index) => <div role="row" key={index} className="grid grid-cols-2">
        {(['left', 'right'] as const).map(side => {
          const cell = row[side]
          return <div key={side} role="cell" className={`flex min-w-0 gap-2 px-3 py-1.5 ${side === 'left' ? 'border-r border-line' : ''} ${cell?.changed ? side === 'left' ? 'bg-red-50 text-red-800' : 'bg-emerald-50 text-emerald-800' : cell ? 'bg-surface text-ink' : 'bg-canvas'}`}>
            <span aria-hidden="true" className="w-7 shrink-0 select-none text-right text-ink-muted">{cell?.number}</span>
            <span className="w-3 shrink-0"><span className="sr-only">{cell?.changed ? side === 'left' ? 'Removed: ' : 'Added: ' : ''}</span><span aria-hidden="true">{cell?.changed ? side === 'left' ? '−' : '+' : ''}</span></span>
            <span className="min-w-0 whitespace-pre-wrap break-words leading-relaxed [overflow-wrap:anywhere]">{cell?.text || '\u00a0'}</span>
          </div>
        })}
      </div>)}
      {!rows.length && <p className="p-4 text-ink-muted">No description provided.</p>}
    </div>
  </section>
}
