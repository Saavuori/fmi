import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Backend origin for the dev proxy; API_PROXY overrides it when the backend
// runs on a non-default port (e.g. API_PROXY=http://127.0.0.1:8090).
const apiTarget = process.env.API_PROXY ?? 'http://127.0.0.1:8080'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    host: true,
    proxy: {
      // Every mode here is poll-style, so unlike the Fintraffic sibling there is
      // no WebSocket route that has to be matched ahead of /api.
      '/api': {
        target: apiTarget,
        changeOrigin: true,
      },
    },
  },
})
