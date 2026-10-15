import { createContext, useContext } from 'react'

export interface Notification {
  title: string
  description?: string
  tone?: 'success' | 'info' | 'warning'
  key?: string
  duration?: number
}
export const NotificationContext = createContext<{ available: boolean; notify: (notice: Notification) => void }>({ available: false, notify: () => {} })
export const useNotifications = () => useContext(NotificationContext)
