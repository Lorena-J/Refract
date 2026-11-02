import { expect, test, type Page, type APIRequestContext } from '@playwright/test'

test.setTimeout(30_000)

const headers = { 'X-Refract': '1' }
const source = String.raw`\documentclass{article}
\begin{document}
\begin{abstract}
One two three four five six seven eight.
\end{abstract}
\section{Background}
Tail text.
\end{document}
`

async function openProject(page: Page, request: APIRequestContext, ai = false) {
  await page.addInitScript(() => {
    if (!localStorage.getItem('refract-prefs'))
      localStorage.setItem('refract-prefs', JSON.stringify({ language: 'en' }))
  })
  await page.route('**/api/chatgpt/status', async (route) => {
    const response = await route.fetch()
    await route.fulfill({ json: { ...(await response.json()), ai_ready: ai } })
  })
  const project = await (await request.post('/api/projects', { headers, data: { name: 'Word counter' } })).json()
  await request.put(`/api/projects/${project.id}/file?path=main.tex`, { headers, data: source })
  await request.patch(`/api/projects/${project.id}`, { headers, data: { main_file: 'main.tex' } })
  await page.goto(`/p/${project.id}`)
  await page.getByRole('button', { name: 'main.tex' }).first().click()
  await expect(page.locator('.cm-content')).toBeVisible()
  return project
}

async function openLimit(page: Page, line = '\\begin{abstract}') {
  await page.locator('.cm-line').filter({ hasText: line }).click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Word limit…', exact: true }).click()
  await expect(page.getByRole('dialog')).toHaveAccessibleName('Word limit')
}

async function abstractLimit(page: Page, max = '3') {
  await openLimit(page)
  await page.getByLabel('Maximum words', { exact: true }).fill(max)
  await page.getByRole('button', { name: 'Set limit', exact: true }).click()
  await expect(page.getByRole('dialog')).not.toBeVisible()
}

const shorter = (page: Page) => page.getByRole('button', { name: 'Suggest a shorter version', exact: true })

test('account menu toggles selection counts and remembers the preference after reload', async ({ page, request }) => {
  await openProject(page, request)
  await expect(page.locator('main .word-count-toggle')).toHaveCount(0)
  await page.getByRole('button', { name: 'Account menu', exact: true }).click()
  const toggle = page.getByRole('menuitemcheckbox', { name: 'Word count', exact: true })
  await expect(toggle).not.toBeChecked()
  await toggle.click()
  await expect(toggle).toBeChecked()
  await page.screenshot({ path: test.info().outputPath('account-word-count-toggle.png'), animations: 'disabled' })
  await expect(page.getByText(/Counter: \d+ words?/)).toHaveCount(0)
  await page.keyboard.press('Escape')
  await page.locator('.cm-content').click()
  await page.keyboard.press('ControlOrMeta+a')
  await expect(page.getByText('Counter: 11 words', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Account menu', exact: true }).click()
  await toggle.click()
  await expect(toggle).not.toBeChecked()
  await expect(page.getByText(/Counter: \d+ words?/)).toHaveCount(0)
  await toggle.click()
  await expect(page.getByText('Counter: 11 words', { exact: true })).toBeVisible()
  await page.keyboard.press('Escape')
  await page
    .locator('.cm-line')
    .filter({ hasText: 'One two three' })
    .dblclick({ position: { x: 10, y: 8 } })
  await expect(page.getByText('Counter: 1 word', { exact: true })).toBeVisible()
  await page.locator('.cm-content').click()
  await expect(page.getByText(/Counter: \d+ words?/)).toHaveCount(0)
  await page.reload()
  await page.getByRole('button', { name: 'main.tex' }).first().click()
  await page.getByRole('button', { name: 'Account menu', exact: true }).click()
  await expect(toggle).toBeChecked()
  await expect(page.getByText(/Counter: \d+ words?/)).toHaveCount(0)
})

test('section context menu sets, edits and removes a limit with the counter off', async ({ page, request }) => {
  await openProject(page, request)
  await abstractLimit(page)
  await expect(page.locator('.cm-word-excess')).toHaveCount(5)
  await expect(shorter(page)).toBeVisible()
  await expect(page.getByText(/Counter: \d+ words?/)).toHaveCount(0)
  await expect(page.getByText('8 / 3', { exact: true })).toHaveCount(0)
  await page.screenshot({ path: test.info().outputPath('word-limit-warning.png') })
  await openLimit(page, 'One two three')
  await expect(page.getByLabel('Maximum words')).toHaveValue('3')
  await expect(page.getByRole('dialog').locator('input')).toHaveCount(1)
  await page.getByLabel('Maximum words').fill('7')
  await page.getByRole('button', { name: 'Set limit', exact: true }).click()
  await expect(page.locator('.cm-word-excess')).toHaveCount(1)
  await page.reload()
  await page.getByRole('button', { name: 'main.tex' }).first().click()
  await expect(page.locator('.cm-word-excess')).toHaveCount(1)
  await openLimit(page)
  await expect(page.getByLabel('Maximum words')).toHaveValue('7')
  await page.screenshot({ path: test.info().outputPath('word-limit-dialog.png') })
  await page.getByRole('button', { name: 'Remove limit', exact: true }).click()
  await expect(page.locator('.cm-word-excess')).toHaveCount(0)
  await expect(shorter(page)).toHaveCount(0)
})

test('context menu also supports arbitrary selected fragments and limits follow typing', async ({ page, request }) => {
  await openProject(page, request)
  await page.locator('.cm-content').click()
  await page.keyboard.press('ControlOrMeta+a')
  await page.locator('.cm-line').filter({ hasText: 'One two three' }).click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Word limit for selection…', exact: true }).click()
  await page.getByLabel('Maximum words', { exact: true }).fill('11')
  await page.getByRole('button', { name: 'Set limit', exact: true }).click()
  await expect(page.locator('.cm-word-excess')).toHaveCount(0)
  await page.locator('.cm-content').click()
  await page.keyboard.press('ControlOrMeta+End')
  await page.keyboard.type('Extra prose')
  await expect(page.locator('.cm-word-excess')).toHaveCount(2)
  await expect(shorter(page)).toBeVisible()
})

test('floating action opens Assistant settings when AI is not connected', async ({ page, request }) => {
  await openProject(page, request)
  await abstractLimit(page)
  await shorter(page).click()
  await expect(page.getByRole('dialog')).toContainText('Assistant')
})

test('AI suggestions are counted and reviewed before replacement', async ({ page, request }) => {
  const project = await openProject(page, request, true)
  await page.route('**/api/quick', (route) => {
    const body = route.request().postDataJSON()
    expect(body.instruction).toContain('at most 3 readable words')
    expect(body.instruction).toContain('essential ideas')
    return route.fulfill({ json: { text: 'One essential idea.' } })
  })
  await abstractLimit(page)
  await shorter(page).click()
  await expect(page.getByText('8 → 3 words', { exact: true })).toBeVisible()
  await expect(page.locator('.cm-quick-ins').first()).toBeVisible()
  expect(await (await request.get(`/api/projects/${project.id}/file?path=main.tex`, { headers })).text()).toContain(
    'seven eight',
  )
  await page.getByRole('button', { name: 'Accept', exact: true }).click()
  await expect(page.locator('.cm-word-excess')).toHaveCount(0)
  await expect(shorter(page)).toHaveCount(0)
  await expect
    .poll(async () => (await request.get(`/api/projects/${project.id}/file?path=main.tex`, { headers })).text())
    .toContain('One essential idea.')
})

test('over-limit AI responses are rejected', async ({ page, request }) => {
  await openProject(page, request, true)
  await page.route('**/api/quick', (route) => route.fulfill({ json: { text: 'This response has too many words.' } }))
  await abstractLimit(page)
  await shorter(page).click()
  await expect(page.getByText('The suggestion does not meet the word limit. Try again.')).toBeVisible()
  await expect(page.locator('.cm-quick-ins')).toHaveCount(0)
  await expect(page.locator('.cm-word-excess')).toHaveCount(5)
})

test('AI results are discarded after an edit during the request', async ({ page, request }) => {
  await openProject(page, request, true)
  let release!: () => void
  const ready = new Promise<void>((resolve) => {
    release = resolve
  })
  let started!: () => void
  const requested = new Promise<void>((resolve) => {
    started = resolve
  })
  await page.route('**/api/quick', async (route) => {
    started()
    await ready
    await route.fulfill({ json: { text: 'One essential idea.' } })
  })
  await abstractLimit(page)
  await shorter(page).click()
  await requested
  await page.locator('.cm-content').click()
  await page.keyboard.press('ControlOrMeta+End')
  await page.keyboard.type('% new edit')
  release()
  await expect(page.getByText('The text changed while the assistant was working. Try again.')).toBeVisible()
  await expect(page.locator('.cm-quick-ins')).toHaveCount(0)
})

test('a reduction can be rejected without changing the document', async ({ page, request }) => {
  await openProject(page, request, true)
  await page.route('**/api/quick', (route) => route.fulfill({ json: { text: 'One essential idea.' } }))
  await abstractLimit(page)
  await shorter(page).click()
  await expect(page.getByText('8 → 3 words', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Reject', exact: true }).click()
  await expect(page.locator('.cm-word-excess')).toHaveCount(5)
  await expect(page.locator('.cm-quick-ins')).toHaveCount(0)
  await expect(shorter(page)).toBeVisible()
})

test('the same toggle controls the PDF selection count', async ({ page, request }) => {
  test.setTimeout(120_000)
  await openProject(page, request)
  await page.keyboard.press('ControlOrMeta+Enter')
  await expect(page.getByRole('button', { name: 'Compile status and problems' })).toContainText('Compiled', {
    timeout: 90_000,
  })
  const layer = page.locator('[data-page="1"] .refract-pdf-text')
  await expect(layer).toContainText('One two three')
  await layer.evaluate((el) => {
    const range = document.createRange()
    range.selectNodeContents(el)
    const selection = window.getSelection()!
    selection.removeAllRanges()
    selection.addRange(range)
    document.dispatchEvent(new Event('selectionchange'))
  })
  await expect(page.getByText(/Counter: \d+ words?/)).toHaveCount(0)
  await page.getByRole('button', { name: 'Account menu', exact: true }).click()
  const toggle = page.getByRole('menuitemcheckbox', { name: 'Word count', exact: true })
  await toggle.click()
  await expect(page.getByText(/Counter: \d+ words/)).toBeVisible()
  await toggle.click()
  await expect(page.getByText(/Counter: \d+ words?/)).toHaveCount(0)
})
