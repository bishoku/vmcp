import type { ActiveFilter } from './SearchControls'

export const filterItem = (label: string, value: string, onRemove: () => void): ActiveFilter => ({ id: label, label, value, onRemove })
