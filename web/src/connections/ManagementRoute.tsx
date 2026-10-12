import { type ComponentType } from 'react'
import { Navigate, useSearchParams } from 'react-router-dom'

export interface SelectionProps { initialSelection?: string }
export default function ManagementRoute({page: Page, detailBase}: {page: ComponentType<SelectionProps>; detailBase?: string}) {
  const [params] = useSearchParams()
  const selection = params.get('inspect') ?? undefined
  if(selection && detailBase) return <Navigate replace to={`${detailBase}/${encodeURIComponent(selection)}`}/>
  return <Page key={selection ?? 'list'} initialSelection={selection}/>
}
