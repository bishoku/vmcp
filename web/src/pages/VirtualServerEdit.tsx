import { useNavigate, useParams } from 'react-router-dom'
import VirtualServerEditor from './VirtualServerEditor'
import { serverPath } from '../connections/model'
export default function VirtualServerEdit({create = false}: {create?: boolean}) {
  const {slug = ''} = useParams()
  const navigate = useNavigate()
  return <VirtualServerEditor key={create ? 'new' : slug} initialSelection={create ? undefined : slug} onSaved={saved=>navigate(serverPath(saved))} onCancel={()=>navigate(create ? '/virtual-servers' : serverPath(slug))}/>
}
