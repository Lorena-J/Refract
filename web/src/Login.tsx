import { useEffect, useState } from 'react'
import { Loader2 } from 'lucide-react'
import { api, errorText } from './api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { t } from '@/lib/i18n'

// Why an OIDC sign-in came back refused (?login_error=…).
const oidcErrors: Record<string, string> = {
  not_allowed: t('There is no Refract account for that email. Ask whoever runs this server to add you.'),
  expired: t('The sign-in took too long or started in another browser. Try again.'),
  failed: t('Signing in with the provider failed. Try again, or use your password.'),
}

// Shown instead of the app when the server asks for a session (REFRACT_AUTH=password).
export default function Login() {
  const [oidc, setOidc] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState(() => {
    const code = new URLSearchParams(location.search).get('login_error')
    return code ? (oidcErrors[code] ?? oidcErrors.failed) : ''
  })

  useEffect(() => {
    api<{ oidc: string }>('/api/auth/config').then((c) => setOidc(c.oidc), () => {})
    if (location.search.includes('login_error')) history.replaceState(null, '', location.pathname)
  }, [])

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api('/api/auth/login', { method: 'POST', body: { email, password } })
      location.reload()
    } catch (err) {
      setError(errorText(err))
      setBusy(false)
    }
  }

  return (
    <main className="flex min-h-dvh items-center justify-center bg-background p-4">
      <div className="w-full max-w-sm space-y-6">
        <div className="flex flex-col items-center gap-3 text-center">
          <img src="/logo.svg" alt="" className="size-10" />
          <h1 className="text-xl font-semibold">{t('Sign in to Refract')}</h1>
        </div>
        <form onSubmit={submit} className="space-y-3 rounded-2xl border bg-card p-5 shadow-xs">
          <Input type="email" autoComplete="username" autoFocus required value={email} onChange={(e) => setEmail(e.target.value)} placeholder={t('Email')} aria-label={t('Email')} />
          <Input
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder={t('Password')}
            aria-label={t('Password')}
          />
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <Button type="submit" className="w-full" disabled={busy}>
            {busy && <Loader2 className="animate-spin" />} {t('Sign in')}
          </Button>
          {oidc && (
            <>
              <div className="flex items-center gap-3 text-xs text-muted-foreground">
                <span className="h-px flex-1 bg-border" /> {t('or')} <span className="h-px flex-1 bg-border" />
              </div>
              <Button type="button" variant="secondary" className="w-full" nativeButton={false} render={<a href="/api/auth/oidc/start" />}>
                {t('Continue with {name}', { name: oidc })}
              </Button>
            </>
          )}
        </form>
      </div>
    </main>
  )
}
