import { useState } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test } from 'vitest'
import RequestHeaderMappings from './RequestHeaderMappings'
import type { RequestHeaderMapping } from '../api'
beforeEach(()=>{HTMLDialogElement.prototype.showModal=function(){this.open=true};HTMLDialogElement.prototype.close=function(){this.open=false}})
afterEach(()=>{cleanup();Reflect.deleteProperty(HTMLDialogElement.prototype,'showModal');Reflect.deleteProperty(HTMLDialogElement.prototype,'close')})
function Draft(){const [rules,setRules]=useState<RequestHeaderMapping[]>([]);return <><RequestHeaderMappings value={rules} onChange={setRules}/><output data-testid="rules">{JSON.stringify(rules)}</output></>}
test('modal applies mappings only to draft and cancellation discards changes',()=>{
 render(<Draft/>);fireEvent.click(screen.getByText('Configure mappings'));fireEvent.click(screen.getByText('Add header mapping'))
 fireEvent.change(screen.getByLabelText('Source header 1'),{target:{value:'X-Agent-Token'}})
 expect((screen.getByLabelText('Target header 1') as HTMLInputElement).value).toBe('X-Agent-Token')
 fireEvent.change(screen.getByLabelText('Target header 1'),{target:{value:'X-User-Token'}})
 fireEvent.click(screen.getByLabelText('Require header 1'))
 expect(screen.getByTestId('rules').textContent).toBe('[]')
 fireEvent.click(screen.getByText('Use mappings'))
 expect(JSON.parse(screen.getByTestId('rules').textContent!)).toEqual([{source:'X-Agent-Token',target:'X-User-Token',required:true}])
 fireEvent.click(screen.getByText('Configure mappings'));fireEvent.click(screen.getByText('Remove mapping 1'));fireEvent.click(screen.getByText('Cancel'))
 expect(JSON.parse(screen.getByTestId('rules').textContent!)).toHaveLength(1)
})
test('invalid or duplicate targets stay in modal',()=>{
 render(<Draft/>);fireEvent.click(screen.getByText('Configure mappings'));fireEvent.click(screen.getByText('Add header mapping'));fireEvent.click(screen.getByText('Use mappings'))
 expect(screen.getByRole('alert').textContent).toContain('valid source and target')
 fireEvent.change(screen.getByLabelText('Source header 1'),{target:{value:'X-One'}})
 fireEvent.click(screen.getByText('Add header mapping'))
 fireEvent.change(screen.getByLabelText('Source header 2'),{target:{value:'X-Two'}})
 fireEvent.change(screen.getByLabelText('Target header 2'),{target:{value:'x-one'}})
 fireEvent.click(screen.getByText('Use mappings'));expect(screen.getByRole('alert').textContent).toContain('unique')
 expect(screen.getByTestId('rules').textContent).toBe('[]')
})
test('read-only mappings have no editor',()=>{render(<RequestHeaderMappings value={[{source:'X-One',target:'X-Two'}]}/>);expect(screen.queryByText('Configure mappings')).toBeNull();expect(screen.getByText('X-Two')).toBeTruthy()})
