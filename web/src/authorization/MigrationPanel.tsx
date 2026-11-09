import { ClipboardCheck, ShieldCheck, Database, LockKeyhole } from 'lucide-react'
import { DetailRow, ServerPanel, SummaryTile } from '../connections/ServerPanels'
import type { AccessStatus } from './client'
const labels: Record<string, string> = {
  oidc_claim_sessions_not_connected: 'Encrypted OIDC claim sessions are not connected.',
  untrustedEnabledMappings: 'Enabled mappings refer to an unconfigured issuer.',
  untrustedEnabledOIDCAssignments: 'Enabled OIDC assignments refer to an unconfigured issuer.',
  inactiveTokenAssignments: 'Enabled assignments refer to inactive or missing tokens.',
  unpinnedAuditResourceAssignments: 'Audit resource assignments are missing an ownership period.',
}
export default function MigrationPanel({ status }: { status: AccessStatus }) {
  return (
    <section aria-label="Authorization access status" className="space-y-6">
      <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
        <SummaryTile
          label="Policy revision"
          value={status.authorizationRevision}
          detail="Current authorization snapshot"
          icon={ShieldCheck}
        />
        <SummaryTile
          label="Schema version"
          value={status.schemaVersion}
          detail="Last applied database migration"
          icon={Database}
        />
        <SummaryTile
          label="Access warnings"
          value={status.blockers.length}
          detail="Operational review is still required"
          icon={ClipboardCheck}
        />
        <SummaryTile
          label="Legacy audit events"
          value={status.counts.legacyAuditEvents ?? 0}
          detail="Not automatically assigned to an owner"
          icon={LockKeyhole}
        />
      </div>
      <div className="grid items-start gap-6 xl:grid-cols-2">
        <ServerPanel
          title="Authorization policy"
          description="Reported state only; no permission or provenance changes are performed"
          icon={ShieldCheck}
        >
          <dl className="divide-y divide-line panel-inset">
            {Object.entries({
              'Configured issuer': status.trustedIssuer || 'Not configured',
              'Claim sessions': status.claimMappings,
              'External OAuth bearer': status.externalOAuthBearer ? 'Accepted' : 'Not accepted',
            }).map(([label, value]) => (
              <DetailRow key={label} label={label}>
                <span className="break-all">{value}</span>
              </DetailRow>
            ))}
          </dl>
        </ServerPanel>
        <ServerPanel
          title="Access warnings"
          description="A clean inventory does not certify administrator sign-in or operational readiness"
          icon={ClipboardCheck}
        >
          <div className="panel-body">
            {status.blockers.length ? (
              <ul className="space-y-3 text-xs leading-relaxed">
                {status.blockers.map((b) => (
                  <li className="surface-inset p-3" key={b}>
                    {labels[b] ?? b}
                  </li>
                ))}
              </ul>
            ) : (
              <p className="supporting-copy">
                No reported blockers. Review identity-provider access and recovery separately.
              </p>
            )}
          </div>
        </ServerPanel>
      </div>
      <ServerPanel
        title="Aggregate inventory"
        description="Counts do not classify or rewrite historical payload ownership"
        icon={Database}
      >
        <dl className="grid gap-x-8 panel-inset sm:grid-cols-2">
          {Object.entries(status.counts).map(([label, value]) => (
            <DetailRow key={label} label={label.replace(/([a-z])([A-Z])/g, '$1 $2')}>
              {value}
            </DetailRow>
          ))}
        </dl>
      </ServerPanel>
    </section>
  )
}
