import { LoaderCircle } from 'lucide-react'
import type { ComponentProps } from 'react'

type Props = ComponentProps<'button'> & { variant?: 'primary' | 'secondary' | 'danger' | 'danger-outline' | 'ghost'; loading?: boolean; loadingLabel?: string; iconOnly?: boolean }
export default function Button({ variant = 'secondary', loading = false, loadingLabel, iconOnly = false, disabled, className = '', children, type = 'button', ...props }: Props) {
 return <button {...props} type={type} disabled={disabled || loading} aria-busy={loading || undefined} className={`action-button action-${variant} ${iconOnly ? 'action-icon' : ''} ${loading ? 'action-loading' : ''} ${className}`}>
   {loading && <LoaderCircle aria-hidden="true" className="icon-control shrink-0 motion-spin"/>}
   <span className="action-content">{loading && loadingLabel ? loadingLabel : children}</span>
 </button>
}
