import { defineConfig } from '@playwright/test';

// Browser E2E against the running Docker stack (`docker compose up --build`).
// Fake camera/microphone let two headless browsers exchange real WebRTC media.
export default defineConfig({
  testDir: './e2e',
  timeout: 90_000,
  expect: { timeout: 20_000 },
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: process.env.LAMMA_URL ?? 'http://localhost:8080',
    permissions: ['camera', 'microphone'],
    launchOptions: {
      args: ['--use-fake-device-for-media-stream', '--use-fake-ui-for-media-stream'],
    },
    trace: 'retain-on-failure',
  },
});
