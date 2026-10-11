import { type ComponentType } from 'react'
import { useSearchParams } from 'react-router-dom'

export interface SelectionProps { initialSelection?: string }
export default function ManagementRoute({page: Page}: {page: ComponentType<SelectionProps>}) {
  const [params] = useSearchParams()
  const selection = params.get('inspect') ?? undefined
  return <Page key={selection ?? 'list'} initialSelection={selection}/>
}
