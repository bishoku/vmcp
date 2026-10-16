import { useEffect, useRef } from 'react'
import { useLocation } from 'react-router-dom'

/** Route changes initiated with a keyboard land at the new page, not the removed link. */
export default function KeyboardNavigation() {
 const {pathname}=useLocation()
 const previous=useRef(pathname),keyboard=useRef(false)
 useEffect(()=>{
  const key=(event:KeyboardEvent)=>{if(['Tab','Enter',' ','ArrowUp','ArrowDown','ArrowLeft','ArrowRight'].includes(event.key))keyboard.current=true}
  const pointer=()=>{keyboard.current=false}
  document.addEventListener('keydown',key,true);document.addEventListener('pointerdown',pointer,true)
  return()=>{document.removeEventListener('keydown',key,true);document.removeEventListener('pointerdown',pointer,true)}
 },[])
 useEffect(()=>{
  if(previous.current===pathname)return
  previous.current=pathname
  if(!keyboard.current)return
  const frame=requestAnimationFrame(()=>{
   if(document.querySelector('dialog[open]'))return
   const main=document.getElementById('main-content')
   const target=main?.querySelector<HTMLElement>('h1')??main
   if(!target)return
   target.setAttribute('tabindex','-1')
   main?.scrollTo?.({top:0,behavior:'instant'})
   target.focus({preventScroll:true})
  })
  return()=>cancelAnimationFrame(frame)
 },[pathname])
 return null
}
