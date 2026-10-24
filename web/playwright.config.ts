import { defineConfig } from '@playwright/test'

const baseURL = 'http://127.0.0.1:4174'

export default defineConfig({
  testDir: 'browser',
  testIgnore: '**/live/**',
  // Large developer hosts otherwise start dozens of uncached browser sessions.
  workers: process.env.CI ? 2 : 4,
  use: {
    baseURL,
    trace: 'retain-on-failure',
  },
  webServer: {
    // API routing disables the browser cache. Serve the bundled app so repeated
    // document loads do not fetch the entire development module graph and HMR client.
    command: 'npm run build && npm run preview -- --host 127.0.0.1 --port 4174 --strictPort',
    env: { VITE_PUBLIC_GATEWAY_URL: baseURL },
    url: baseURL,
    reuseExistingServer: false,
    timeout: 120_000,
  },
  reporter: 'list',
})
