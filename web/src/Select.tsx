import type { ComponentPropsWithRef } from 'react'
import { input } from './styles'

/** Keeps native labels, validation, keyboard navigation and form submission intact. */
export default function Select({ className = input, ref, ...props }: ComponentPropsWithRef<'select'>) {
  return <select {...props} ref={ref} className={`${className} vmcp-select`}/>
}
