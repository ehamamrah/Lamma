/// <reference types="vitest/config" />
import { fileURLToPath, URL } from 'node:url';
import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';
import { visionBase } from './scripts/vision-paths.mjs';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  define: { __VISION_BASE__: JSON.stringify(visionBase) },
  // ES workers: the vision worker loads the MediaPipe runtime with import().
  worker: { format: 'es' },
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: {
    // `npm run dev` with the Docker stack running: API calls go through Nginx.
    // The API's CSRF guard expects the stack's public origin, so the dev
    // server presents it when proxying.
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        configure: (proxy) => {
          proxy.on('proxyReq', (req) => req.setHeader('origin', 'http://localhost:8080'));
        },
      },
    },
  },
  build: { sourcemap: false, chunkSizeWarningLimit: 900 },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    css: false,
  },
});
