import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
export default defineConfig({plugins:[react()],test:{setupFiles:['./test-setup.ts'],environment:'jsdom',include:['src/**/*.test.{ts,tsx}'],restoreMocks:true}})
