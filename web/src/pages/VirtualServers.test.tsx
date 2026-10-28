import { render } from '../test-utils'
import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { api, type Upstream, type VirtualServer } from '../api'
import VirtualServerEditor from './VirtualServerEditor'
import VirtualServers from './VirtualServers'
import { MemoryRouter } from 'react-router-dom'

const server: VirtualServer = {
  slug: 'demo', name: 'Demo server', description: '', version: 7,
  enabled: true, authRequired: true, endpoint: '/mcp/v1/virtual/demo',
  policy: { default: 'allow', tools: { first: 'deny', second: 'read_only' } },
  tools: [
    { upstream: 'alpha', tool: 'one', alias: 'first', exposedName: 'first', upstreamDescription: '', descriptionDrift: false, orphaned: false },
    { upstream: 'alpha', tool: 'two', alias: 'second', exposedName: 'second', upstreamDescription: '', descriptionDrift: false, orphaned: false },
  ],
}
beforeEach(() => {
  vi.stubGlobal('matchMedia',vi.fn(()=>({matches:true,addEventListener:vi.fn(),removeEventListener:vi.fn()})))
  vi.spyOn(api, 'listVirtualServers').mockResolvedValue([server])
  vi.spyOn(api, 'getVirtualServer').mockResolvedValue(server)
  vi.spyOn(api, 'listUpstreams').mockResolvedValue([{ slug: 'alpha', name: 'Alpha' }] as Upstream[])
  vi.spyOn(api, 'listTools').mockResolvedValue([{ name: 'one' }, { name: 'two' }])
  vi.spyOn(api, 'listCatalog').mockResolvedValue({ resources: [], resourceTemplates: [], prompts: [], capabilities: {} })
  vi.spyOn(api, 'putVirtualServer').mockResolvedValue(server)
})
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllEnvs(); vi.unstubAllGlobals() })
async function open() {
  render(<MemoryRouter><VirtualServerEditor initialSelection="demo" /></MemoryRouter>)
  await screen.findByLabelText('Alias for alpha/one')
}
async function save() {
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(api.putVirtualServer).toHaveBeenCalledTimes(1))
  return vi.mocked(api.putVirtualServer).mock.calls[0]
}
test('renaming and clearing aliases preserve deny/read-only policies and the original version', async () => {
  await open()
  fireEvent.change(screen.getByLabelText('Alias for alpha/one'), { target: { value: 'renamed' } })
  fireEvent.change(screen.getByLabelText('Alias for alpha/two'), { target: { value: '' } })
  const [slug, body, version] = await save()
  expect(slug).toBe('demo')
  expect(version).toBe(7)
  expect(body.policy).toEqual({ default: 'allow', tools: { renamed: 'deny', alpha__two: 'read_only' } })
})
test('temporary duplicates block saving and swapping aliases keeps policies with their tools', async () => {
  await open()
  fireEvent.change(screen.getByLabelText('Alias for alpha/one'), { target: { value: 'second' } })
  expect((screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement).disabled).toBe(true)
  fireEvent.change(screen.getByLabelText('Alias for alpha/two'), { target: { value: 'first' } })
  const [, body] = await save()
  expect(body.policy).toEqual({ default: 'allow', tools: { second: 'deny', first: 'read_only' } })
})
test('removing a selection removes its obsolete policy from the saved configuration', async () => {
  await open()
  fireEvent.click(screen.getAllByRole('button', { name: 'Remove' })[0])
  const [, body] = await save()
  expect(body.tools).toHaveLength(1)
  expect(body.policy?.tools).toEqual({ second: 'read_only' })
})
test('shows and copies the configured public gateway endpoint', async () => {
  vi.stubEnv('VITE_PUBLIC_GATEWAY_URL', 'http://localhost:8800')
  const copy = vi.fn().mockResolvedValue(undefined)
  const original = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: copy } })
  try {
    await open()
    expect(screen.getByText('http://localhost:8800/mcp/v1/virtual/demo')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Copy endpoint' }))
    expect(copy).toHaveBeenCalledWith('http://localhost:8800/mcp/v1/virtual/demo')
  } finally {
    if (original) Object.defineProperty(navigator, 'clipboard', original)
    else Reflect.deleteProperty(navigator, 'clipboard')
  }
})
test('without public gateway configuration only a path is shown and endpoint copying is disabled', async () => {
  vi.stubEnv('VITE_PUBLIC_GATEWAY_URL', '')
  await open()
  expect(screen.getByText('/mcp/v1/virtual/demo')).toBeTruthy()
  expect(screen.getByText(/Public gateway URL is not configured/).textContent).toContain('not configured')
  expect((screen.getByRole('button', { name: 'Copy endpoint' }) as HTMLButtonElement).disabled).toBe(true)
})
test('failed stale save retains edits until explicit reload, then uses the latest version',async()=>{
 const {ApiError}=await import('../api')
 vi.mocked(api.putVirtualServer).mockRejectedValueOnce(new ApiError(412,'This item changed in another session.'))
 const confirm=vi.spyOn(window,'confirm').mockReturnValue(true)
 await open()
 fireEvent.change(screen.getByLabelText('Alias for alpha/one'),{target:{value:'unsaved-alias'}})
 fireEvent.click(screen.getByRole('button',{name:'Save'}))
 const reload=await screen.findByText('Reload latest and discard draft')
 expect((screen.getByLabelText('Alias for alpha/one') as HTMLInputElement).value).toBe('unsaved-alias')
 vi.mocked(api.getVirtualServer).mockResolvedValue({...server,version:8})
 fireEvent.click(reload)
 await waitFor(()=>expect((screen.getByLabelText('Alias for alpha/one') as HTMLInputElement).value).toBe('first'))
 expect(confirm).toHaveBeenCalledTimes(1)
 fireEvent.click(screen.getByRole('button',{name:'Save'}))
 await waitFor(()=>expect(api.putVirtualServer).toHaveBeenCalledTimes(2))
 expect(vi.mocked(api.putVirtualServer).mock.calls[1][2]).toBe(8)
})
test('the virtual server inventory does not depend on upstream discovery',async()=>{
 vi.mocked(api.listUpstreams).mockRejectedValue(new Error('catalog offline'))
 render(<MemoryRouter><VirtualServers/></MemoryRouter>);expect(await screen.findByRole('link',{name:'Demo server'})).toBeTruthy()
 expect(api.listUpstreams).not.toHaveBeenCalled()
 expect(screen.getByRole('article',{name:'Demo server'})).toBeTruthy()
})
test('successful save announces the saved server without retaining a dirty draft',async()=>{
 await open();await save()
 expect(await screen.findByText('Saved Demo server. Refreshing the saved configuration.')).toBeTruthy()
 expect(screen.queryByText(/Unsaved changes/)).toBeNull()
})
test('card links open saved details and search/status filters do not open an editor',async()=>{
  vi.mocked(api.listVirtualServers).mockResolvedValue([server,{...server,slug:'disabled',name:'Offline server',enabled:false}])
  render(<MemoryRouter><VirtualServers/></MemoryRouter>)
  const link=await screen.findByRole('link',{name:'Demo server'})
  expect(link.getAttribute('href')).toBe('/virtual-servers/demo/overview')
  expect(screen.queryByLabelText('Name')).toBeNull()
  fireEvent.change(screen.getByLabelText('Search virtual servers'),{target:{value:'Offline'}})
  expect(screen.queryByRole('link',{name:'Demo server'})).toBeNull()
  expect(screen.getByRole('link',{name:'Offline server'})).toBeTruthy()
  fireEvent.change(screen.getByLabelText('Virtual server status'),{target:{value:'enabled'}})
  expect(screen.getByText('No virtual servers match your filters.')).toBeTruthy()
})

test('tool upstream and description filters keep exposed tools and their policies intact',async()=>{
 vi.mocked(api.listUpstreams).mockResolvedValue([{slug:'alpha',name:'Alpha'},{slug:'beta',name:'Beta'}] as Upstream[])
 vi.mocked(api.listTools).mockImplementation(async slug=>slug==='alpha'?[{name:'one',description:'Find customer records'},{name:'two'}]:[{name:'external',description:'Other tool'}])
 await open()
 fireEvent.change(screen.getByLabelText('Tool upstream'),{target:{value:'beta'}})
 expect(await screen.findByRole('button',{name:/external.*Other tool/})).toBeTruthy()
 expect(screen.queryByRole('button',{name:/one.*Find customer records/})).toBeNull()
 expect((screen.getByLabelText('Alias for alpha/one') as HTMLInputElement).value).toBe('first')
 fireEvent.change(screen.getByLabelText('Tool upstream'),{target:{value:'alpha'}})
 fireEvent.change(screen.getByLabelText('Filter tools'),{target:{value:'customer'}})
 expect(screen.getByRole('button',{name:/one.*Find customer records/})).toBeTruthy()
 const [,body]=await save()
 expect(body.policy?.tools).toEqual({first:'deny',second:'read_only'})
})

test('loads full guidance before editing and preserves it on unrelated saves',async()=>{
 const guidance={instructions:'Keep this guide',resources:[{name:'guide',title:'Guide',description:'',text:'Long reference material',mimeType:'text/plain' as const,enabled:true}],prompts:[],toolReferences:[],upstreamInstructions:[]}
 vi.mocked(api.listVirtualServers).mockResolvedValue([{...server,hasGuidance:true}])
 vi.mocked(api.getVirtualServer).mockResolvedValue({...server,hasGuidance:true,guidance})
 await open()
 expect(api.getVirtualServer).toHaveBeenCalledWith('demo',expect.any(AbortSignal))
 const [,body]=await save()
 expect(body.guidance).toEqual(guidance)
})
