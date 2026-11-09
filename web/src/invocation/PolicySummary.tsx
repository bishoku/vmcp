import type { InvocationRule } from '../api'
import { DetailRow } from '../connections/ServerPanels'
import { identityLabels, retrySummary } from './policy'

export default function PolicySummary({ rule }: { rule: InvocationRule }) {
  return (
    <dl className="divide-y divide-line">
      <DetailRow label="User identity">
        {rule.identity
          ? `${identityLabels[rule.identity.source]}${rule.identity.service ? ` via ${rule.identity.service}` : ''}`
          : 'No identity requirement'}
      </DetailRow>
      <DetailRow label="Step-up authentication">
        {rule.stepUp
          ? `${rule.stepUp.acceptedAcrValues.join(', ')} · max ${rule.stepUp.maxAgeSeconds}s`
          : 'Not required'}
      </DetailRow>
      <DetailRow label="Approval services">
        {rule.approvalServices?.join(', ') || 'Not required'}
      </DetailRow>
      <DetailRow label="Automatic retry">{retrySummary(rule)}</DetailRow>
      <DetailRow label="Read-only classification">
        {rule.readOnly ? 'Operator classified read-only' : 'Not classified read-only'}
      </DetailRow>
      {!!rule.identity?.fields?.length && (
        <DetailRow label="Caller context fields">{rule.identity.fields.join(', ')}</DetailRow>
      )}
      <DetailRow label="Arguments shared with services">
        {rule.shareFields?.join(', ') || 'No arguments shared'}
      </DetailRow>
    </dl>
  )
}
