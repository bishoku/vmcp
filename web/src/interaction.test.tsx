import { act, cleanup, fireEvent, render, renderHook, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { capabilities, useDraft, useOperation } from './interaction'
import { CopyButton, StaleRecovery } from './ui'
afterEach(()=>{cleanup();vi.restoreAllMocks()})
test.each([
 ['viewer',false,false],['editor',true,false],['admin',true,true],
] as const)('%s capabilities match server authorization', (role,edit,manageTokens)=>{
 expect(capabilities(role)).toEqual({edit,manageTokens})
})
test('dirty drafts guard switching and dismissal; success resets without confirmation',()=>{
 const confirm=vi.spyOn(window,'confirm').mockReturnValue(false)
 const {result}=renderHook(()=>useDraft<{slug:string;text:string}|null>(null,d=>d?.slug))
 act(()=>result.current[1]({slug:'a',text:'original'}))
 act(()=>result.current[1]({slug:'a',text:'edited'}))
 act(()=>result.current[1]({slug:'b',text:'other'}))
 expect(result.current[0]?.text).toBe('edited')
 act(()=>result.current[1](null));expect(result.current[0]).not.toBeNull()
 confirm.mockReturnValue(true)
 act(()=>result.current[1]({slug:'b',text:'other'}))
 expect(result.current[0]?.slug).toBe('b')
 confirm.mockClear();act(()=>result.current[2](null));expect(confirm).not.toHaveBeenCalled()
})
test('operation latch prevents overlapping submissions and unlocks after failure',async()=>{
 const {result}=renderHook(useOperation)
 let reject!:(error:Error)=>void
 const pending=new Promise<void>((_,fail)=>{reject=fail})
 const operation=vi.fn(()=>pending)
 let first!:Promise<void>
 act(()=>{first=result.current.run(operation);void result.current.run(operation)})
 expect(operation).toHaveBeenCalledTimes(1);expect(result.current.busy).toBe(true)
 await act(async()=>{reject(new Error('write failed'));await expect(first).rejects.toThrow('write failed')})
 expect(result.current.busy).toBe(false)
 await act(()=>result.current.run(async()=>{}))
})
test('clipboard rejection provides manual recovery without claiming success',async()=>{
 vi.stubGlobal('navigator',{clipboard:{writeText:vi.fn().mockRejectedValue(new Error('denied'))}})
 render(<CopyButton value="secret" />);fireEvent.click(screen.getByRole('button',{name:'Copy'}))
 expect((await screen.findByRole('alert')).textContent).toContain('copy it manually')
 expect(screen.queryByText('Copied')).toBeNull();vi.unstubAllGlobals()
})
test('stale recovery requires explicit draft discard and never retries a write',async()=>{
 const reload=vi.fn().mockResolvedValue(undefined)
 const confirm=vi.spyOn(window,'confirm').mockReturnValue(false)
 render(<StaleRecovery message="This item changed in another session." reload={reload}/>)
 fireEvent.click(screen.getByRole('button'));expect(reload).not.toHaveBeenCalled()
 confirm.mockReturnValue(true);fireEvent.click(screen.getByRole('button'))
 await act(async()=>{});expect(reload).toHaveBeenCalledTimes(1)
})
