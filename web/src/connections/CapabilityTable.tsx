import type { Capability } from './model'
export default function CapabilityTable({objects}: {objects: Capability[]}) {
  return <div className="mt-4 overflow-x-auto"><table className="w-full min-w-[720px] text-left text-sm">
    <caption className="pb-3 text-left text-xs leading-relaxed text-ink-muted">Configured capabilities; missing objects are not exposed. Read-only tool policy requires the upstream readOnlyHint.</caption>
    <thead className="bg-canvas text-xs text-ink-muted"><tr>{['Kind','Upstream / original','Exposed name','Policy','Discovery'].map(title=><th scope="col" className="px-4 py-3 font-medium" key={title}>{title}</th>)}</tr></thead>
    <tbody>{objects.map((object,index)=><tr key={JSON.stringify([object.kind,object.upstream,object.original,index])} className="border-t border-line hover:bg-canvas/50">
      <td className="px-4 py-3"><span className="rounded-md bg-canvas px-2 py-1 text-xs font-medium">{object.kind}</span></td><td className="break-all px-4 py-3 font-mono text-xs">{object.upstream} / {object.original}</td>
      <td className="break-all px-4 py-3 font-mono text-xs">{object.exposed}</td><td className="px-4 py-3 text-xs">{object.policy.replaceAll('_',' ')}</td><td className="px-4 py-3"><span className={`whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium ${object.missing?'bg-amber-50 text-amber-700':'bg-canvas text-ink-muted'}`}>{object.missing ? 'Missing object' : 'Discovered'}</span></td>
    </tr>)}</tbody>
  </table>{objects.length===0 && <p className="py-4 text-ink-muted">No capabilities selected.</p>}</div>
}
