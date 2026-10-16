import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import DescriptionOverride from './DescriptionOverride'
import { descriptionLength } from './description'
import { ConfirmationContext } from './confirmation'

beforeEach(() => {
  HTMLDialogElement.prototype.showModal = function() { this.open = true }
  HTMLDialogElement.prototype.close = function() { this.open = false }
})
afterEach(() => { cleanup(); vi.restoreAllMocks(); Reflect.deleteProperty(HTMLDialogElement.prototype, 'showModal'); Reflect.deleteProperty(HTMLDialogElement.prototype, 'close') })
const original = 'Instructions:\n  preserve spacing\n<literal> & text'
function Field({change = vi.fn()}: {change?: (value: string) => void}) {
  const [value, setValue] = useState('Saved override')
  return <DescriptionOverride name="tools.0.description" identity="alpha/echo" exposedName="say" value={value} upstream={original} drift onChange={text => {setValue(text);change(text)}}/>
}
const open = () => fireEvent.click(screen.getByRole('button', {name: 'Edit description for alpha/echo'}))

test('edits stay local across views, preserve whitespace, and apply only to the parent draft', () => {
  const change = vi.fn();render(<Field change={change}/>);open()
  const draft = 'New instructions\n\n  indentation\n<script>literal</script>'
  fireEvent.change(screen.getByRole('textbox', {name:'Description override'}), {target:{value:draft}})
  fireEvent.click(screen.getByRole('button', {name:'Upstream'}))
  expect(screen.getByRole('region', {name:'Upstream description'}).textContent).toContain(original)
  fireEvent.click(screen.getByRole('button', {name:'Agent view'}))
  const preview=screen.getByRole('region', {name:'Agent description'})
  expect(within(preview).getByRole('heading').textContent).toBe('say')
  expect(preview.textContent).toContain(draft)
  expect(preview.querySelector('script')).toBeNull()
  expect(change).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', {name:'Apply to draft'}))
  expect(change).toHaveBeenCalledExactlyOnceWith(draft)
  expect(screen.queryByRole('dialog')).toBeNull()
})

test('cancel confirms local changes and keeps both saved text and unapplied edits when declined', async () => {
  const confirm=vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true)
  const change=vi.fn();render(<ConfirmationContext.Provider value={confirm}><Field change={change}/></ConfirmationContext.Provider>);open()
  fireEvent.change(screen.getByRole('textbox'), {target:{value:'Unapplied'}})
  await act(async () => { fireEvent.click(screen.getByRole('button', {name:'Cancel'})) })
  await vi.waitFor(() => expect(confirm).toHaveBeenCalledTimes(1))
  await vi.waitFor(() => expect((screen.getByRole('button', {name:'Cancel'}) as HTMLButtonElement).disabled).toBe(false))
  expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe('Unapplied')
  await act(async () => { fireEvent.click(screen.getByRole('button', {name:'Cancel'})) })
  await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(change).not.toHaveBeenCalled();open()
  expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe('Saved override')
})

test('Unicode boundary allows 20,000 characters and preserves excess text while blocking apply', () => {
  render(<Field/>);open()
  const boundary='🙂'.repeat(20_000)
  expect(descriptionLength(boundary)).toBe(20_000)
  fireEvent.change(screen.getByRole('textbox'), {target:{value:boundary}})
  expect((screen.getByRole('button', {name:'Apply to draft'}) as HTMLButtonElement).disabled).toBe(false)
  fireEvent.change(screen.getByRole('textbox'), {target:{value:boundary+'ş'}})
  expect((screen.getByRole('button', {name:'Apply to draft'}) as HTMLButtonElement).disabled).toBe(true)
  expect(screen.getByRole('alert').textContent).toContain('20,000')
  expect((screen.getByRole('textbox') as HTMLTextAreaElement).value).toBe(boundary+'ş')
})

test('clearing override previews upstream text and applies an empty override', () => {
  const change=vi.fn();render(<Field change={change}/>);open()
  fireEvent.click(screen.getByRole('button', {name:'Use upstream description'}))
  fireEvent.click(screen.getByRole('button', {name:'Agent view'}))
  expect(screen.getByRole('region', {name:'Agent description'}).textContent).toContain(original)
  fireEvent.click(screen.getByRole('button', {name:'Apply to draft'}))
  expect(change).toHaveBeenCalledWith('')
})

test('read-only inspection shows the full long upstream source without applying override limits', () => {
  const upstream='x'.repeat(21_000)
  render(<DescriptionOverride name="catalog.0.description" identity="alpha/review" exposedName="alpha__review" value="" upstream={upstream}/>)
  fireEvent.click(screen.getByRole('button', {name:'Inspect description for alpha/review'}))
  expect(screen.getByRole('region', {name:'Agent description'}).textContent).toContain(upstream)
  expect(screen.queryByRole('textbox')).toBeNull()
  expect(screen.queryByRole('alert')).toBeNull()
  expect(screen.queryByRole('button', {name:'Apply to draft'})).toBeNull()
})
