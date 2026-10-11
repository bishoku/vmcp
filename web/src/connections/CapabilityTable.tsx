import type { Capability } from './model'
export default function CapabilityTable({objects}: {objects: Capability[]}) {
  return <div className="mt-4 overflow-x-auto"><table className="w-full text-left text-sm">
    <caption className="pb-2 text-left text-ink-muted">Configured capabilities; missing objects are not exposed. Read-only tool policy requires the upstream readOnlyHint.</caption>
    <thead><tr>{['Kind','Upstream / original','Exposed name','Policy','Discovery'].map(title=><th className="p-2" key={title}>{title}</th>)}</tr></thead>
    <tbody>{objects.map((object,index)=><tr key={JSON.stringify([object.kind,object.upstream,object.original,index])} className="border-t border-line">
      <td className="p-2">{object.kind}</td><td className="break-all p-2">{object.upstream} / {object.original}</td>
      <td className="break-all p-2">{object.exposed}</td><td className="p-2">{object.policy}</td><td className="p-2">{object.missing ? 'Missing object' : 'Discovered'}</td>
    </tr>)}</tbody>
  </table>{objects.length===0 && <p className="py-4 text-ink-muted">No capabilities selected.</p>}</div>
}
