import { defineConfig } from '@playwright/test'
import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { randomBytes } from 'node:crypto'

// Browser tests against the real binary (run `make build` first; needs tectonic on PATH),
// with a scripted model (e2e/fake-model.mjs) for the assistant.
// Two projects run every spec: Chrome (the installed Google Chrome, no download) and WebKit
// (Playwright's build, `npx playwright install webkit`), the engine of the macOS app's WKWebView.
// `make e2e BROWSER=chrome|webkit|all` picks which.
// Ports can be moved (REFRACT_E2E_PORT, REFRACT_E2E_MODEL_PORT) to run two checkouts side by side.
const port = Number(process.env.REFRACT_E2E_PORT ?? 18090)
const modelPort = Number(process.env.REFRACT_E2E_MODEL_PORT ?? 18091)
const dataDir = mkdtempSync(join(tmpdir(), 'refract-e2e-'))

export default defineConfig({
  testDir: 'e2e',
  timeout: 120_000,
  // On GitHub, failures also show as annotations on the run and the pull request.
  reporter: process.env.GITHUB_ACTIONS ? [['github'], ['list']] : 'list',
  use: { baseURL: `http://127.0.0.1:${port}`, viewport: { width: 1500, height: 850 } },
  projects: [
    { name: 'chrome', use: { channel: 'chrome' } },
    { name: 'webkit', use: { browserName: 'webkit' } },
  ],
  webServer: [
    {
      command: 'node e2e/fake-model.mjs',
      url: `http://127.0.0.1:${modelPort}/api/v1/models`,
      env: { REFRACT_E2E_MODEL_PORT: String(modelPort) },
    },
    {
      command: '../refract',
      url: `http://127.0.0.1:${port}/healthz`,
      env: {
        REFRACT_AUTH: 'none',
        REFRACT_ADDR: `127.0.0.1:${port}`,
        REFRACT_DATA_DIR: dataDir,
        REFRACT_SECRET_KEY: randomBytes(32).toString('base64'),
      },
    },
  ],
})
