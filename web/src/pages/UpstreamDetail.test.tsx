import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { render } from '../test-utils'
import { api, ApiError } from '../api'
import { upstream } from '../connections/fixtures'
import UpstreamDetail from './UpstreamDetail'
import { RoleContext } from '../interaction'

beforeEach(()=>{
  vi.spyOn(api,'listUpstreams').mockResolvedValue([upstream])
  vi.spyOn(api,'listTools').mockResolvedValue([{name:'lookup'}])
  vi.spyOn(api,'listCatalog').mockResolvedValue({resources:[],resourceTemplates:[],prompts:[],capabilities:{}})
  vi.spyOn(api,'patchUpstream').mockResolvedValue({...upstream,version:3,name:'Updated'})
})
afterEach(()=>{cleanup();vi.restoreAllMocks()})
function open(viewer=false) {
  render(<MemoryRouter initialEntries={['/upstreams/alpha']}><Routes><Route path="/upstreams/:slug" element={viewer ? <RoleContext.Provider value="viewer"><UpstreamDetail/></RoleContext.Provider> : <UpstreamDetail/>}/></Routes></MemoryRouter>)
}
test('opens a full read-only page and saving returns to read-only with the latest version',async()=>{
  open();const name=await screen.findByLabelText('Name')
  expect((name as HTMLInputElement).disabled || name.closest('fieldset')?.disabled).toBe(true)
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(screen.queryByRole('button',{name:'Save'})).toBeNull()
  fireEvent.click(screen.getByRole('button',{name:'Edit upstream'}))
  fireEvent.change(name,{target:{value:'Updated'}})
  vi.mocked(api.listUpstreams).mockResolvedValue([{...upstream,version:3,name:'Updated'}])
  fireEvent.click(screen.getByRole('button',{name:'Save'}))
  await waitFor(()=>expect(api.patchUpstream).toHaveBeenCalledWith('alpha',expect.objectContaining({name:'Updated'}),2))
  expect(await screen.findByRole('button',{name:'Edit upstream'})).toBeTruthy()
  expect(screen.getByRole('heading',{name:'Updated'})).toBeTruthy()
  expect((screen.getByLabelText('Name').closest('fieldset') as HTMLFieldSetElement).disabled).toBe(true)
})
test('Cancel confirms discarding dirty changes and restores saved values',async()=>{
  const confirm=vi.spyOn(window,'confirm').mockReturnValue(false)
  open();await screen.findByLabelText('Name')
  fireEvent.click(screen.getByRole('button',{name:'Edit upstream'}))
  fireEvent.change(screen.getByLabelText('Name'),{target:{value:'Draft'}})
  fireEvent.click(screen.getByRole('button',{name:'Cancel'}))
  expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('Draft')
  confirm.mockReturnValue(true)
  fireEvent.click(screen.getByRole('button',{name:'Cancel'}))
  await waitFor(()=>expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('Alpha'))
  expect(api.patchUpstream).not.toHaveBeenCalled()
})
test('viewers cannot enter edit mode or modify stored configuration',async()=>{
  open(true);await screen.findByLabelText('Name')
  expect(screen.queryByRole('button',{name:'Edit upstream'})).toBeNull()
  expect((screen.getByLabelText('URL').closest('fieldset') as HTMLFieldSetElement).disabled).toBe(true)
})
test('a stale write retains the draft and requires explicit recovery',async()=>{
  vi.mocked(api.patchUpstream).mockRejectedValue(new ApiError(412,'This item changed in another session.'))
  open();await screen.findByLabelText('Name');fireEvent.click(screen.getByRole('button',{name:'Edit upstream'}))
  fireEvent.change(screen.getByLabelText('Name'),{target:{value:'Draft'}})
  fireEvent.click(screen.getByRole('button',{name:'Save'}))
  await screen.findByText('Reload latest and discard draft')
  expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('Draft')
})
test('missing upstream has a useful error state and list navigation',async()=>{
  vi.mocked(api.listUpstreams).mockResolvedValue([]);open()
  expect(await screen.findByText('Upstream “alpha” was not found.')).toBeTruthy()
  expect(screen.getByRole('link',{name:'Back to upstreams'}).getAttribute('href')).toBe('/upstreams')
})
test('creating an upstream opens its saved read-only detail page',async()=>{
  vi.spyOn(api,'createUpstream').mockResolvedValue(upstream)
  render(<MemoryRouter initialEntries={['/upstreams/new']}><Routes><Route path="/upstreams/new" element={<UpstreamDetail create/>}/><Route path="/upstreams/:slug" element={<UpstreamDetail/>}/></Routes></MemoryRouter>)
  fireEvent.change(screen.getByLabelText('Slug'),{target:{value:'alpha'}})
  fireEvent.change(screen.getByLabelText('Name'),{target:{value:'Alpha'}})
  fireEvent.change(screen.getByLabelText('URL'),{target:{value:'https://example.com/mcp'}})
  fireEvent.click(screen.getByRole('button',{name:'Save'}))
  expect(await screen.findByRole('button',{name:'Edit upstream'})).toBeTruthy()
  expect((screen.getByLabelText('Name').closest('fieldset') as HTMLFieldSetElement).disabled).toBe(true)
})

test('upstream transport selection offers only HTTP endpoint transports',async()=>{
  open();await screen.findByLabelText('Name')
  fireEvent.click(screen.getByRole('button',{name:'Edit upstream'}))
  const options=screen.getAllByRole('option')
    .map(option=>(option as HTMLOptionElement).value)
  expect(options).toEqual(['streamable_http','sse'])
})
