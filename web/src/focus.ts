/** Native tab order, including one stop per named radio group. */
export function tabStops(root: HTMLElement): HTMLElement[] {
 const candidates=Array.from(root.querySelectorAll<HTMLElement>('a[href], button, input:not([type="hidden"]), select, textarea, summary, [tabindex], [contenteditable="true"]'))
  .filter(el=>el.tabIndex>=0 && !el.matches(':disabled') && !el.closest('[inert], [hidden], [aria-hidden="true"]') && el.getClientRects().length>0 && getComputedStyle(el).visibility!=='hidden')
 return candidates.filter(el=>{
  if (!(el instanceof HTMLInputElement) || el.type!=='radio' || !el.name) return true
  const group=candidates.filter((item): item is HTMLInputElement=>item instanceof HTMLInputElement && item.type==='radio' && item.name===el.name && item.form===el.form)
  return el===(group.find(item=>item.checked)??group[0])
 })
}

export function trapTab(event: {key:string;shiftKey:boolean;preventDefault:()=>void}, root: HTMLElement) {
 if(event.key!=='Tab')return
 const items=tabStops(root),first=items[0],last=items.at(-1)
 if(!first){event.preventDefault();root.focus();return}
 if(event.shiftKey && (document.activeElement===first || document.activeElement===root)) {event.preventDefault();last?.focus()}
 else if(!event.shiftKey && (document.activeElement===last || document.activeElement===root)) {event.preventDefault();first.focus()}
}
