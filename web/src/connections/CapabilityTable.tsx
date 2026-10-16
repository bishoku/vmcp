import DescriptionOverride from '../DescriptionOverride'
import { StatusBadge, TagBadge } from '../Badge'
import { DataTable } from '../DataTable'
import { useTableSort } from '../table-sorting'
import { EmptyState } from '../Feedback'
import type { Capability } from './model'
export default function CapabilityTable({objects,limit}: {objects: Capability[];limit?:number}) {
  const sorting=useTableSort(objects,{Kind:o=>o.kind,'Upstream / original':o=>`${o.upstream}/${o.original}`,'Exposed name':o=>o.exposed,Policy:o=>o.policy,Discovery:o=>o.missing?'missing':'discovered'})
  return <div className="mt-4"><p className="mb-3 text-xs leading-relaxed text-ink-muted">Configured capabilities; missing objects are not exposed. Read-only tool policy requires the upstream readOnlyHint.</p><DataTable caption="Configured capabilities" columns={['Kind','Upstream / original','Exposed name','Description','Policy','Discovery']} sorting={sorting} primary={2}>{sorting.rows.slice(0,limit??objects.length).map((object,index)=><tr key={JSON.stringify([object.kind,object.upstream,object.original,index])} className="border-t border-line hover:bg-canvas/50">
      <td className="px-4 py-3"><TagBadge label={object.kind}/></td><td className="break-all px-4 py-3 font-mono text-xs">{object.upstream} / {object.original}</td>
      <td className="break-all px-4 py-3 font-mono text-xs">{object.exposed}</td><td className="max-w-md min-w-64 px-4 py-3"><DescriptionOverride name={`capabilities.${index}.description`} identity={`${object.upstream}/${object.original}`} exposedName={object.exposed} value={object.description ?? ''} upstream={object.upstreamDescription ?? ''} drift={object.descriptionDrift}/></td><td className="px-4 py-3 text-xs"><StatusBadge status={object.policy}/></td><td className="px-4 py-3"><StatusBadge status={object.missing?'missing object':'discovered'} label={object.missing?'Missing object':'Discovered'}/></td>
    </tr>)}</DataTable>{objects.length===0 && <EmptyState title="No capabilities selected." compact/>}</div>
}
