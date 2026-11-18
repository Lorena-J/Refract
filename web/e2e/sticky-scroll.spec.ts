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
  const lineHeight = () => scroller.evaluate((el) => parseFloat(getComputedStyle(el).lineHeight))
  // Scroll by a number of lines: the headers describe the first line left visible under them.
  const scrollTo = (lines: number) =>
    scroller.evaluate((el, lines) => (el.scrollTop = lines * parseFloat(getComputedStyle(el).lineHeight)), lines)
  const lastRowOffset = async () =>
    ((await rows.last().boundingBox())!.y - (await panel.boundingBox())!.y) / (await lineHeight())

  await expect(panel).toBeHidden()
  await scrollTo(4.5) // "Literal body" under five headers
  await expect(rows).toHaveText(source.slice(1, 5).concat(source[6]))
  await scrollTo(7.5) // "Quote body"
  await expect(rows).toHaveText(source.slice(1, 5))
  await scrollTo(10.5) // "Section body"
  await expect(rows).toHaveText(source.slice(1, 4))
  expect(await lastRowOffset()).toBeCloseTo(2, 1)

  // The next section pushes the headers of the one that ends out, a pixel at a time.
  await scrollTo(11.5)
  await expect(rows).toHaveText(source.slice(1, 4))
  await expect.poll(lastRowOffset).toBeCloseTo(1.5, 1)
  await scrollTo(12.5)
  await expect(rows).toHaveText(source.slice(1, 3))
  await expect.poll(lastRowOffset).toBeCloseTo(0.5, 1)
  await scrollTo(13.5) // "Second section body"
  await expect(rows).toHaveText([source[1], source[14]])
  await expect.poll(lastRowOffset).toBeCloseTo(1, 1)
  // A header sticks as its own line reaches the row, and is drawn like that line.
  const header = page.locator('.cm-line').filter({ hasText: source[14] })
  expect((await rows.last().boundingBox())!.x).toBeCloseTo((await header.boundingBox())!.x + 6, 0)
  expect(
    await rows
      .last()
      .locator('span')
      .first()
      .evaluate((el) => getComputedStyle(el).color),
  ).toBe(
    await header
      .locator('span')
      .first()
      .evaluate((el) => getComputedStyle(el).color),
  )
  await scrollTo(17.5) // past \end{document}
  await expect(panel).toBeHidden()

  async function openEditorSettings() {
    await page.getByRole('button', { name: 'Account menu', exact: true }).click()
    await page.getByRole('menuitem', { name: 'Settings', exact: true }).click()
    await page.getByRole('tab', { name: 'Editor', exact: true }).click()
  }

  await scrollTo(7.5)
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
  await scrollTo(7.5)
  await expect(rows).toHaveText(source.slice(1, 5))
})
