import { expect, test, type Page } from '@playwright/test'

const headers = { 'X-Refract': '1' }
const bib = '@article{local, title={Old title}, doi={10.1000/local}, note={Keep this}}\n'
const corrected =
  '@article{local, title={Verified title}, author={Real Author}, journal={Journal}, year=2024, doi={10.1000/local}, note={Keep this}}\n'

async function openChecks(page: Page) {
  await page.getByRole('button', { name: 'Citation checks fixture' }).click()
  await page.getByRole('menuitem', { name: 'Citation checks', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'Citation checks', exact: true })).toBeVisible()
}

test('citation checks close when the project goes to the trash', async ({ page, request }) => {
  const project = await (
    await request.post('/api/projects', { headers, data: { name: 'Citation checks fixture' } })
  ).json()
  await request.put(`/api/projects/${project.id}/file?path=main.tex`, {
    headers,
    data: '\\bibliography{refs}\n',
  })
  await request.put(`/api/projects/${project.id}/file?path=refs.bib`, { headers, data: bib })
  await request.patch(`/api/projects/${project.id}`, { headers, data: { main_file: 'main.tex' } })
  await page.goto(`/p/${project.id}`)
  await openChecks(page)
  const dialog = page.getByRole('dialog', { name: 'Citation checks', exact: true })
  await expect(dialog.getByRole('button', { name: 'Check again', exact: true })).toBeEnabled()
  expect((await request.delete(`/api/projects/${project.id}`, { headers })).ok()).toBeTruthy()
  await dialog.getByRole('button', { name: 'Check again', exact: true }).click()
  await expect(dialog).toBeHidden()
  await expect(page.getByRole('status').filter({ hasText: 'This project is in the trash' })).toBeVisible()
  await page.getByRole('button', { name: 'Citation checks fixture' }).click()
  await expect(page.getByRole('menuitem', { name: 'Citation checks', exact: true })).toHaveCount(0)
})

test('citation checks locate problems and keep metadata changes reviewable', async ({ page, request }) => {
  const project = await (
    await request.post('/api/projects', { headers, data: { name: 'Citation checks fixture' } })
  ).json()
  for (const [file, text] of Object.entries({
    'main.tex': '\\cite{local,missing}\n\\bibliography{refs}\n',
    'refs.bib': bib,
    'main_2.tex': '\\cite{freund2007}',
    'experiment1/references.bib': bib,
  })) {
    expect(
      (await request.put(`/api/projects/${project.id}/file?path=${file}`, { headers, data: text })).ok(),
    ).toBeTruthy()
  }
  expect(
    (await request.patch(`/api/projects/${project.id}`, { headers, data: { main_file: 'main.tex' } })).ok(),
  ).toBeTruthy()
  await page.goto(`/p/${project.id}`)
  await page.getByRole('button', { name: 'main.tex', exact: true }).first().click()
  await openChecks(page)
  const dialog = page.getByRole('dialog', { name: 'Citation checks', exact: true })
  await expect(dialog).toContainText('Citation has no bibliography entry.')
  await expect(dialog).toContainText('Missing bibliography fields: author, journal, year')
  await expect(dialog).toContainText('DOIs have not been checked online.')
  await expect(dialog).not.toContainText('freund2007')
  await expect(dialog).not.toContainText('Citation key is defined more than once.')
  await dialog.getByRole('button', { name: 'main.tex:1', exact: true }).click()
  await expect(dialog).toBeHidden()
  await expect(page.locator('.cm-content')).toContainText('\\cite{local,missing}')

  // Metadata transport is scripted; the API's source lookup and no-write
  // guarantee have separate Go tests against an HTTP fixture server.
  await page.route(`**/api/projects/${project.id}/citation-checks/fix`, (route) =>
    route.fulfill({ json: { path: 'refs.bib', before: bib, after: corrected } }),
  )
  await openChecks(page)
  await dialog.getByRole('button', { name: 'Review source metadata' }).click()
  await expect(dialog).toBeHidden()
  await expect(page.locator('.cm-quick-ins').first()).toHaveText('Verified')
  await expect(page.locator('.cm-quick-ins').last()).toContainText('author={Real Author}, journal={Journal}')
  const saved = () => request.get(`/api/projects/${project.id}/file?path=refs.bib`, { headers }).then((r) => r.text())
  expect(await saved()).toBe(bib)
  await page.keyboard.press('Escape')
  await expect(page.locator('.cm-quick-ins')).toHaveCount(0)
  expect(await saved()).toBe(bib)

  await openChecks(page)
  await dialog.getByRole('button', { name: 'Review source metadata' }).click()
  await expect(page.locator('.cm-quick-ins').first()).toBeVisible()
  await page.keyboard.press('Tab')
  await expect(page.locator('.cm-quick-ins')).toHaveCount(0)
  await expect.poll(saved).toBe(corrected)
})

test('online DOI failures are distinguished and stale metadata proposals are refused', async ({ page, request }) => {
  const project = await (
    await request.post('/api/projects', { headers, data: { name: 'Citation checks fixture' } })
  ).json()
  await request.put(`/api/projects/${project.id}/file?path=refs.bib`, { headers, data: bib })
  await request.put(`/api/projects/${project.id}/file?path=main.tex`, { headers, data: '\\bibliography{refs}\n' })
  expect(
    (await request.patch(`/api/projects/${project.id}`, { headers, data: { main_file: 'main.tex' } })).ok(),
  ).toBeTruthy()
  await page.goto(`/p/${project.id}`)
  await page.getByRole('button', { name: 'refs.bib', exact: true }).first().click()
  await page.route(`**/api/projects/${project.id}/citation-checks/doi`, (route) =>
    route.fulfill({ json: { status: 'doi-unavailable' } }),
  )
  await openChecks(page)
  const dialog = page.getByRole('dialog', { name: 'Citation checks', exact: true })
  await dialog.getByRole('button', { name: 'Check DOIs online' }).click()
  await expect(dialog).toContainText('DOI could not be checked. The service may be unavailable; try again later.')
  await expect(dialog.getByRole('status')).toHaveText('DOI check complete: 0 of 1 returned metadata.')
  await dialog.getByRole('button', { name: 'Check again', exact: true }).click()
  await expect(dialog.getByRole('status')).toHaveText('DOI check complete: 0 of 1 returned metadata.')
  await expect(dialog).toContainText('DOI could not be checked. The service may be unavailable; try again later.')
  await page.route(`**/api/projects/${project.id}/citation-checks/fix`, (route) =>
    route.fulfill({ json: { path: 'refs.bib', before: bib + '% changed elsewhere', after: corrected } }),
  )
  await dialog.getByRole('button', { name: 'Review source metadata' }).click()
  await expect(dialog.getByRole('alert')).toHaveText('The bibliography changed. Run the citation checks again.')
  await expect(page.locator('.cm-quick-ins')).toHaveCount(0)
  const saved = await (await request.get(`/api/projects/${project.id}/file?path=refs.bib`, { headers })).text()
  expect(saved).toBe(bib)
})

test('online DOI checks confirm success and explain when no DOIs are available', async ({ page, request }) => {
  const project = await (
    await request.post('/api/projects', { headers, data: { name: 'Citation checks fixture' } })
  ).json()
  await request.put(`/api/projects/${project.id}/file?path=refs.bib`, { headers, data: corrected })
  await request.put(`/api/projects/${project.id}/file?path=main.tex`, {
    headers,
    data: '\\bibliography{refs}\n',
  })
  await request.patch(`/api/projects/${project.id}`, { headers, data: { main_file: 'main.tex' } })
  let onlineRequests = 0
  await page.route(`**/api/projects/${project.id}/citation-checks/doi`, (route) => {
    onlineRequests++
    return route.fulfill({ json: { status: 'resolved' } })
  })
  await page.goto(`/p/${project.id}`)
  await openChecks(page)
  const dialog = page.getByRole('dialog', { name: 'Citation checks', exact: true })
  await dialog.getByRole('button', { name: 'Check DOIs online' }).click()
  await expect(dialog.getByRole('status')).toHaveText('DOI check complete: 1 of 1 returned metadata.')
  await expect(dialog).toContainText('Entry is not cited in the project.')
  await dialog.getByRole('button', { name: 'Check again', exact: true }).click()
  await expect(dialog.getByRole('status')).toHaveText('DOI check complete: 1 of 1 returned metadata.')
  expect(onlineRequests).toBe(1)

  await request.put(`/api/projects/${project.id}/file?path=refs.bib`, {
    headers,
    data: corrected + '@misc{new, title={New DOI}, doi={10.1000/new}}\n',
  })
  await dialog.getByRole('button', { name: 'Check again', exact: true }).click()
  await expect(dialog.getByRole('status')).toHaveText('DOIs checked online: 1 of 2. Check online to verify the rest.')
  expect(onlineRequests).toBe(1)

  await request.put(`/api/projects/${project.id}/file?path=refs.bib`, {
    headers,
    data: corrected.replaceAll('10.1000/local', '10.1000/changed'),
  })
  await dialog.getByRole('button', { name: 'Check again', exact: true }).click()
  await expect(dialog.getByRole('status')).toHaveText('DOIs have not been checked online.')

  await request.put(`/api/projects/${project.id}/file?path=refs.bib`, {
    headers,
    data: '@misc{local, title={No DOI}}\n',
  })
  await dialog.getByRole('button', { name: 'Check DOIs online' }).click()
  await expect(dialog.getByRole('status')).toHaveText('No DOI identifiers found in the loaded bibliographies.')
})

for (const alreadyOpen of [false, true]) {
  test(`CRLF metadata proposals can be rejected and accepted (${alreadyOpen ? 'open' : 'unopened'} bibliography)`, async ({
    page,
    request,
  }) => {
    const project = await (
      await request.post('/api/projects', { headers, data: { name: 'Citation checks fixture' } })
    ).json()
    const original = bib.replaceAll('\n', '\r\n')
    const replacement = corrected.replaceAll('\n', '\r\n')
    await request.put(`/api/projects/${project.id}/file?path=main.tex`, {
      headers,
      data: '\\bibliography{refs}\n',
    })
    await request.put(`/api/projects/${project.id}/file?path=refs.bib`, { headers, data: original })
    await request.patch(`/api/projects/${project.id}`, { headers, data: { main_file: 'main.tex' } })
    await page.goto(`/p/${project.id}`)
    await page
      .getByRole('button', { name: alreadyOpen ? 'refs.bib' : 'main.tex', exact: true })
      .first()
      .click()
    await page.route(`**/api/projects/${project.id}/citation-checks/fix`, (route) =>
      route.fulfill({ json: { path: 'refs.bib', before: original, after: replacement } }),
    )
    const saved = () => request.get(`/api/projects/${project.id}/file?path=refs.bib`, { headers }).then((r) => r.text())
    await openChecks(page)
    await page.getByRole('button', { name: 'Review source metadata' }).click()
    await expect(page.locator('.cm-quick-ins').first()).toBeVisible()
    expect(await saved()).toBe(original)
    await page.keyboard.press('Escape')
    await expect(page.locator('.cm-quick-ins')).toHaveCount(0)
    expect(await saved()).toBe(original)
    await openChecks(page)
    await page.getByRole('button', { name: 'Review source metadata' }).click()
    await expect(page.locator('.cm-quick-ins').first()).toBeVisible()
    await page.keyboard.press('Tab')
    await expect.poll(saved).toBe(corrected)
    await expect(page.locator('.cm-content')).toContainText('Verified title')
  })
}

test('reviewing metadata preserves an unreviewed proposal', async ({ page, request }) => {
  const project = await (
    await request.post('/api/projects', { headers, data: { name: 'Citation checks fixture' } })
  ).json()
  await request.put(`/api/projects/${project.id}/file?path=main.tex`, {
    headers,
    data: '\\bibliography{refs}\n',
  })
  await request.put(`/api/projects/${project.id}/file?path=refs.bib`, { headers, data: bib })
  await request.patch(`/api/projects/${project.id}`, { headers, data: { main_file: 'main.tex' } })
  await page.goto(`/p/${project.id}`)
  let attempts = 0
  await page.route(`**/api/projects/${project.id}/citation-checks/fix`, (route) =>
    route.fulfill({
      json: {
        path: 'refs.bib',
        before: bib,
        after: ++attempts === 1 ? corrected : corrected.replace('Verified', 'Second'),
      },
    }),
  )
  await openChecks(page)
  await page.getByRole('button', { name: 'Review source metadata' }).click()
  await expect(page.locator('.cm-quick-ins').first()).toHaveText('Verified')
  await openChecks(page)
  await page.getByRole('button', { name: 'Review source metadata' }).click()
  const dialog = page.getByRole('dialog', { name: 'Citation checks', exact: true })
  await expect(dialog.getByRole('alert')).toHaveText(
    'Accept or reject the pending proposal before reviewing another one.',
  )
  await expect(page.locator('.cm-quick-ins').first()).toHaveText('Verified')
  await page.keyboard.press('Escape') // close the checks; retain the original proposal
  await expect(dialog).toBeHidden()
  await page.locator('.cm-content').click()
  await page.keyboard.press('Tab')
  await expect
    .poll(async () => (await request.get(`/api/projects/${project.id}/file?path=refs.bib`, { headers })).text())
    .toBe(corrected)
})

test('citation checks translate server errors in both requests', async ({ page, request }) => {
  await page.addInitScript(() => localStorage.setItem('refract-prefs', JSON.stringify({ language: 'es' })))
  const project = await (
    await request.post('/api/projects', { headers, data: { name: 'Citation checks fixture' } })
  ).json()
  await request.put(`/api/projects/${project.id}/file?path=main.tex`, { headers, data: '\\bibliography{refs}\n' })
  await request.put(`/api/projects/${project.id}/file?path=refs.bib`, { headers, data: bib })
  await request.patch(`/api/projects/${project.id}`, { headers, data: { main_file: 'main.tex' } })
  await page.goto(`/p/${project.id}`)
  const endpoint = `**/api/projects/${project.id}/citation-checks`
  const fail = { status: 401, json: { error: { kind: 'login_required', message: 'Raw server error' } } }
  await page.route(endpoint, (route) => route.fulfill(fail))
  await page.getByRole('button', { name: 'Citation checks fixture' }).click()
  await page.getByRole('menuitem', { name: 'Comprobaciones de citas', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Comprobaciones de citas', exact: true })
  await expect(dialog.getByRole('alert')).not.toContainText('Raw server error')
  await expect(dialog.getByRole('alert')).toHaveText('Tu sesión ha terminado. Recarga la página para volver a entrar.')
  await page.unroute(endpoint)
  await dialog.getByRole('button', { name: 'Volver a comprobar', exact: true }).click()
  await page.route(`${endpoint}/fix`, (route) => route.fulfill(fail))
  await dialog.getByRole('button', { name: 'Revisar metadatos de la fuente' }).click()
  await expect(dialog.getByRole('alert')).not.toContainText('Raw server error')
  await expect(dialog.getByRole('alert')).toHaveText('Tu sesión ha terminado. Recarga la página para volver a entrar.')
})
