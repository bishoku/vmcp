import { render as renderReact } from '@testing-library/react'
import type { ReactNode } from 'react'
import { RoleContext } from './interaction'
/** Component regressions exercise the editor with an explicitly authorized role. */
export const render = (ui: ReactNode) => renderReact(<RoleContext.Provider value="admin">{ui}</RoleContext.Provider>)
