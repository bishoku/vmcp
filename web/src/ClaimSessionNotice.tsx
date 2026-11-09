import { useEffect, useState } from 'react'
import type { Me } from './api'

// This timer only clears browser-held authority snapshots. Backend requests
// independently enforce the stored claim deadline and current policy revision.
export default function ClaimSessionNotice({ me }: { me: Me }) {
  const deadline = me.via === 'oidc' && me.claimsExpire ? Date.parse(me.claimsExpire) : NaN
  const [checked, setChecked] = useState(() => Date.now())
  const expired = Number.isFinite(deadline) && checked >= deadline
  useEffect(() => {
    if (!Number.isFinite(deadline)) return
    const expire = () => {
      setChecked(Date.now())
      window.dispatchEvent(new Event('vmcp:claims-expired'))
    }
    if (Date.now() >= deadline) {
      expire()
      return
    }
    const timer = setTimeout(expire, Math.min(deadline - Date.now(), 2_147_483_647))
    return () => clearTimeout(timer)
  }, [deadline])
  if (!expired) return null
  return (
    <p
      role="status"
      className="mx-4 mt-4 surface-inset surface-white px-4 py-3 text-xs leading-relaxed text-ink-muted sm:mx-6 lg:mx-10"
    >
      Some permissions need a new sign-in. Your other session permissions may still work. Refresh
      claim-based permissions by{' '}
      <a className="text-accent-700 underline" href="/auth/login">
        signing in again
      </a>
      .
    </p>
  )
}
