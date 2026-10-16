import { afterEach, expect, test, vi } from 'vitest'
import { tabStops, trapTab } from './focus'

afterEach(()=>{document.body.innerHTML='';vi.restoreAllMocks()})
function fixture(markup:string){
 document.body.innerHTML=`<div id="root" tabindex="-1">${markup}</div>`
 vi.spyOn(HTMLElement.prototype,'getClientRects').mockImplementation(()=>[{}] as unknown as DOMRectList)
 return document.getElementById('root')!
}
test('tab stops skip hidden, inert and disabled fields and include one enabled radio per group',()=>{
 const root=fixture('<button id="first">First</button><fieldset disabled><input id="disabled"></fieldset><button hidden>Hidden</button><div inert><button>Inert</button></div><div aria-hidden="true"><button>Decorative</button></div><input type="hidden"><button tabindex="-1">Programmatic</button><input type="radio" name="one" id="one-a"><input type="radio" name="one" id="one-b" checked><input type="radio" name="two" id="two-a" disabled><input type="radio" name="two" id="two-b"><input type="radio" name="two" id="two-c"><a id="last" href="#">Last</a>')
 expect(tabStops(root).map(el=>el.id)).toEqual(['first','one-b','two-b','last'])
})
test('Tab and Shift+Tab cycle only at dialog boundaries',()=>{
 const root=fixture('<button id="first">First</button><input id="middle"><button id="last">Last</button>')
 const preventDefault=vi.fn()
 document.getElementById('last')!.focus();trapTab({key:'Tab',shiftKey:false,preventDefault},root)
 expect(document.activeElement?.id).toBe('first')
 trapTab({key:'Tab',shiftKey:true,preventDefault},root);expect(document.activeElement?.id).toBe('last')
 expect(preventDefault).toHaveBeenCalledTimes(2)
 document.getElementById('middle')!.focus();trapTab({key:'Tab',shiftKey:false,preventDefault},root)
 expect(preventDefault).toHaveBeenCalledTimes(2)
})
test('a dialog with no enabled controls retains focus on its container',()=>{
 const root=fixture('<button disabled>Busy</button>'),preventDefault=vi.fn()
 trapTab({key:'Tab',shiftKey:false,preventDefault},root)
 expect(document.activeElement).toBe(root);expect(preventDefault).toHaveBeenCalledOnce()
})
