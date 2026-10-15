import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import NotificationsProvider from './NotificationsProvider'
import { useNotifications } from './notifications'
import { CopyButton } from './ui'

afterEach(()=>{cleanup();vi.useRealTimers();vi.unstubAllGlobals()})
function Trigger(){const {notify}=useNotifications();return <><button onClick={()=>notify({title:'Saved',key:'save'})}>Save</button><button onClick={()=>notify({title:'Updated'})}>Update</button></>}
test('deduplicates, bounds the stack, preserves focus and dismisses',()=>{
 render(<NotificationsProvider><Trigger/></NotificationsProvider>)
 const save=screen.getByRole('button',{name:'Save'});save.focus()
 fireEvent.click(save);fireEvent.click(save)
 expect(screen.getAllByTestId('toast')).toHaveLength(1);expect(document.activeElement).toBe(save)
 for(let i=0;i<4;i++)fireEvent.click(screen.getByRole('button',{name:'Update'}))
 expect(screen.getAllByTestId('toast')).toHaveLength(3)
 fireEvent.click(screen.getAllByRole('button',{name:'Dismiss notification: Updated'})[0])
 expect(screen.getAllByTestId('toast')).toHaveLength(2)
})
test('timeout pauses while hovered and focused then resumes with remaining time',()=>{
 vi.useFakeTimers();render(<NotificationsProvider><Trigger/></NotificationsProvider>)
 fireEvent.click(screen.getByRole('button',{name:'Save'}))
 act(()=>vi.advanceTimersByTime(2000));fireEvent.mouseEnter(screen.getByTestId('toast'))
 act(()=>vi.advanceTimersByTime(10000));expect(screen.getByTestId('toast')).toBeTruthy()
 const close=screen.getByRole('button',{name:'Dismiss notification: Saved'})
 act(()=>close.focus());fireEvent.mouseLeave(screen.getByTestId('toast'))
 act(()=>vi.advanceTimersByTime(10000));expect(screen.getByTestId('toast')).toBeTruthy()
 act(()=>screen.getByRole('button',{name:'Save'}).focus())
 act(()=>vi.advanceTimersByTime(3999));expect(screen.getByTestId('toast')).toBeTruthy()
 act(()=>vi.advanceTimersByTime(1));expect(screen.queryByTestId('toast')).toBeNull()
})
test('copy reports only completion and never includes secret material',async()=>{
 const writeText=vi.fn().mockResolvedValue(undefined)
 vi.stubGlobal('navigator',{clipboard:{writeText}})
 render(<NotificationsProvider><CopyButton value="private-secret-material"/></NotificationsProvider>)
 fireEvent.click(screen.getByRole('button',{name:'Copy'}))
 await act(async()=>{})
 expect(screen.getByTestId('toast').textContent).toContain('Copied to clipboard')
 expect(screen.getByTestId('toast').textContent).not.toContain('private-secret-material')
 writeText.mockRejectedValue(new Error('denied'))
 fireEvent.click(screen.getByRole('button',{name:'Dismiss notification: Copied to clipboard'}))
 fireEvent.click(screen.getByRole('button',{name:'Copy'}));await act(async()=>{})
 expect(screen.queryByTestId('toast')).toBeNull();expect(screen.getByRole('alert').textContent).toContain('manually')
})
