import { expect, test } from '@playwright/test'

test('changing the password replaces the displayed sessions', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('refract-prefs', JSON.stringify({ language: 'en' })))
  const email = 'reader@example.test'
  await page.route('**/api/me', (route) => route.fulfill({ json: { id: 'reader', email, can_sign_out: true } }))
  await page.route('**/api/account', (route) =>
    route.fulfill({
      json: {
        email,
        has_password: true,
        email_verified: true,
        lang: 'en',
        projects: 0,
        mail: false,
        oidc: '',
        recent_sign_in: true,
      },
    }),
  )

  const now = new Date().toISOString()
  const session = { created_at: now, last_seen_at: now, user_agent: '' }
  const current = { ...session, id: 'original', ip: '192.0.2.1', current: true }
  const other = { ...session, id: 'other', ip: '192.0.2.2', current: false }
  const replacement = { ...session, id: 'replacement', ip: '192.0.2.3', current: true }
  let passwordChanged = false
  await page.route('**/api/account/sessions', (route) =>
    route.fulfill({ json: passwordChanged ? [replacement] : [current, other] }),
  )
  await page.route('**/api/account/password', async (route) => {
    passwordChanged = true
    await route.fulfill({ status: 204 })
  })

  await page.goto('/settings/account')
  await expect(page.getByText(current.ip, { exact: false })).toBeVisible()
  const signOut = page.getByRole('button', { name: /Sign out of Unknown browser, 192\.0\.2\.2/ })
  await expect(signOut).toBeVisible()

  await page.getByLabel('Current password', { exact: true }).fill('old password')
  await page.getByLabel('New password', { exact: true }).fill('new password')
  await page.getByRole('button', { name: 'Change', exact: true }).click()

  await expect(page.getByText(replacement.ip, { exact: false })).toBeVisible()
  await expect(page.getByText('This device', { exact: true })).toHaveCount(1)
  await expect(page.getByText(current.ip, { exact: false })).toHaveCount(0)
  await expect(signOut).toHaveCount(0)
})
