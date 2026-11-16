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
