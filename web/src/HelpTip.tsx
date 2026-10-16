import { CircleHelp } from 'lucide-react'

export default function HelpTip({ content, label }: { content: string; label: string }) {
  return <button type="button" data-tooltip={content} data-tooltip-help="true" aria-label={label} className="action-button action-ghost action-icon action-compact"><CircleHelp aria-hidden="true" className="icon-inline"/></button>
}
