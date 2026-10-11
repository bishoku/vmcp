import {defineConfig} from '@playwright/test'
export default defineConfig({testDir:'browser/live',outputDir:'test-results-live',workers:1,timeout:90_000,use:{baseURL:process.env.VMCP_LIVE_WEB},reporter:'list'})
