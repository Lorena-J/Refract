import { expect, test } from '@playwright/test'

const doc = '\\documentclass{article}\n\\begin{document}\nHello from the smoke test.\n\\end{document}\n'
const api = { 'X-Refract': '1' }

// Open project → edit → compile → see PDF.
test('open project, edit, compile, see the PDF', async ({ page, request }) => {
  const project = await (await request.post('/api/projects', { headers: api, data: { name: 'Smoke' } })).json()
  expect((await request.put(`/api/projects/${project.id}/file?path=main.tex`, { headers: api, data: doc })).ok()).toBeTruthy()
  expect((await request.patch(`/api/projects/${project.id}`, { headers: api, data: { main_file: 'main.tex' } })).ok()).toBeTruthy()

  await page.goto(`/p/${project.id}`)
  await page.getByRole('button', { name: 'main.tex' }).first().click()
  const editor = page.locator('.cm-content')
  await editor.click()
  await page.keyboard.press('ControlOrMeta+End')
  await page.keyboard.type('% edited in the browser\n')
  await page.keyboard.press('ControlOrMeta+Enter') // saves, then compiles

  await expect(page.getByRole('button', { name: 'Compile status and problems' })).toContainText(/Compiled/, { timeout: 90_000 })
  const canvas = page.locator('[data-page="1"] canvas')
  await expect.poll(() => canvas.evaluate((c: HTMLCanvasElement) => c.width)).toBeGreaterThan(0)

  // The edit reached the disk before compiling.
  const saved = await (await request.get(`/api/projects/${project.id}/file?path=main.tex`, { headers: api })).text()
  expect(saved).toContain('% edited in the browser')
})
