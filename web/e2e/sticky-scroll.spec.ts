import { expect, test } from '@playwright/test'

test('sticky scroll shows enclosing LaTeX structures and the editor setting persists', async ({ page, request }) => {
  const headers = { 'X-Refract': '1' }
  const source = [
    '\\documentclass{article}',
    '\\begin{document}',
    '\\section[Short]{First \\emph{section}}',
    '\\subsection*{Nested heading}',
    '\\begin{quote}',
    '% \\end{quote}',
    '\\begin{verbatim}',
    '\\section{Literal heading}',
    '\\end{quote}',
    'Literal body',
    '\\end{verbatim}',
    'Quote body',
    '\\end{quote}',
    'Section body',
    '\\section{Second section}',
    'Second section body',
    '\\end{document}',
    ...Array.from({ length: 50 }, (_, i) => '% Tail ' + i),
  ]
  const project = await (await request.post('/api/projects', { headers, data: { name: 'Sticky scroll' } })).json()
  const upload = await request.put('/api/projects/' + project.id + '/file?path=main.tex', {
    headers,
    data: source.join('\n'),
  })
  expect(upload.ok()).toBe(true)
  await page.goto('/p/' + project.id)
  await page.getByRole('button', { name: 'main.tex' }).first().click()

  const scroller = page.locator('.cm-scroller')
  const panel = page.locator('.cm-sticky-scroll')
  const rows = panel.locator(':scope > div')
  const scrollToLine = (text: string) =>
    scroller.evaluate(
      (el, index) => (el.scrollTop = (index + 0.5) * parseFloat(getComputedStyle(el).lineHeight)),
      source.indexOf(text),
    )

  await expect(panel).toBeHidden()
  await scrollToLine('Literal body')
  await expect(rows).toHaveText(source.slice(1, 5).concat(source[6]))
  await scrollToLine('Quote body')
  await expect(rows).toHaveText(source.slice(1, 5))
  await scrollToLine('Section body')
  await expect(rows).toHaveText(source.slice(1, 4))
  await scrollToLine('Second section body')
  await expect(rows).toHaveText([source[1], source[14]])
  await scrollToLine('% Tail 0')
  await expect(panel).toBeHidden()

  async function openEditorSettings() {
    await page.getByRole('button', { name: 'Account menu', exact: true }).click()
    await page.getByRole('menuitem', { name: 'Settings', exact: true }).click()
    await page.getByRole('tab', { name: 'Editor', exact: true }).click()
  }

  await scrollToLine('Quote body')
  await expect(panel).toBeVisible()
  await openEditorSettings()
  const toggle = page.getByRole('switch', { name: 'Sticky scroll', exact: true })
  await expect(toggle).toBeChecked()
  await toggle.click()
  await expect(toggle).not.toBeChecked()
  await expect(panel).toHaveCount(0)

  await page.reload()
  await page.getByRole('button', { name: 'main.tex' }).first().click()
  await openEditorSettings()
  await expect(toggle).not.toBeChecked()
  await toggle.click()
  await page.keyboard.press('Escape')
  await scrollToLine('Quote body')
  await expect(rows).toHaveText(source.slice(1, 5))
})
