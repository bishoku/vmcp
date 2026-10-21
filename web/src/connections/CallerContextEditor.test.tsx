import {useState} from 'react'
import {cleanup,fireEvent,render,screen} from '@testing-library/react'
import {afterEach,expect,test} from 'vitest'
import CallerContextEditor from './CallerContextEditor'
import type {CallerContextConfig} from '../api'
afterEach(cleanup)
function Draft({kind='rest'}:{kind?:'rest'|'mcp'}){
 const[value,setValue]=useState<CallerContextConfig>({enabled:false,metaKey:'bank/context',fields:[]})
 return <><CallerContextEditor value={value} kind={kind} onChange={setValue}/><output data-testid="config">{JSON.stringify(value)}</output></>
}
test('configures REST metadata headers and persists field options in draft',()=>{
 render(<Draft/>);fireEvent.click(screen.getByLabelText('Forward caller context'))
 fireEvent.click(screen.getByText('Add context field'))
 fireEvent.change(screen.getByLabelText('Context field 1'),{target:{value:'token'}})
 fireEvent.change(screen.getByLabelText('Context header 1'),{target:{value:'Authorization'}})
 fireEvent.click(screen.getByLabelText('Bearer prefix for field 1'))
 fireEvent.click(screen.getByLabelText('Require context field 1'))
 expect(JSON.parse(screen.getByTestId('config').textContent!)).toEqual({enabled:true,metaKey:'bank/context',fields:[{name:'token',header:'Authorization',bearer:true,required:true}]})
 fireEvent.click(screen.getByText('Remove context field 1'))
 expect(JSON.parse(screen.getByTestId('config').textContent!).fields).toEqual([])
})
test('MCP has metadata field selection without HTTP mapping',()=>{
 render(<Draft kind="mcp"/>);fireEvent.click(screen.getByLabelText('Forward caller context'));fireEvent.click(screen.getByText('Add context field'))
 expect(screen.queryByLabelText('Context header 1')).toBeNull()
 expect(screen.queryByLabelText('Bearer prefix for field 1')).toBeNull()
})
test('read-only settings cannot be changed',()=>{
 render(<CallerContextEditor kind="rest" value={{enabled:true,metaKey:'bank/context',fields:[{name:'token',header:'Authorization'}]}}/>)
 expect(screen.getByLabelText('Metadata namespace').closest('fieldset')?.disabled).toBe(true)
})
