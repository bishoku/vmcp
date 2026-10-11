import { defineConfig } from '@playwright/test'
export default defineConfig({testDir:'browser',use:{baseURL:'http://127.0.0.1:4174'},webServer:{command:'npm run dev -- --host 127.0.0.1 --port 4174',url:'http://127.0.0.1:4174',reuseExistingServer:false},reporter:'list'})
