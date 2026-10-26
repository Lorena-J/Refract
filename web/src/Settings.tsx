import { useRef, useState } from 'react'
import { ArrowLeft, CheckCircle2, Upload } from 'lucide-react'
import { toast } from 'sonner'
import { api, describe, RequestError, type ChatGPTStatus } from './api'
import { Button } from '@/components/ui/button'
import AccountMenu from '@/components/AccountMenu'
import PlanBadge from './PlanBadge'

export default function Settings({ status, onChange }: { status: ChatGPTStatus | null; onChange: (s: ChatGPTStatus) => void }) {
  const [busy, setBusy] = useState(false)
  const input = useRef<HTMLInputElement>(null)

  async function run(fn: () => Promise<ChatGPTStatus>, ok: string) {
    setBusy(true)
    try {
      onChange(await fn())
      toast.success(ok)
    } catch (e) {
      toast.error(e instanceof RequestError ? describe(e.error) : String(e))
    } finally {
      setBusy(false)
    }
  }

  const s = status?.status
  return (
    <div className="flex h-full flex-col p-2">
      <main className="flex min-h-0 flex-1 flex-col overflow-auto rounded-xl border bg-card shadow-xs">
        <div className="mx-auto w-full max-w-2xl px-6 py-8">
          <a href="/" className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors duration-150 hover:text-foreground">
            <ArrowLeft className="size-4" /> All projects
          </a>
          <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>

          <section className="mt-8 rounded-xl border p-5">
            <h2 className="font-semibold">ChatGPT</h2>
            {!status ? (
              <p className="mt-2 text-sm text-muted-foreground">Loading…</p>
            ) : s?.signed_in ? (
              <div className="mt-3 space-y-3 text-sm">
                <p className="flex items-center gap-2">
                  <CheckCircle2 className="size-4 text-success" /> Signed in as <strong>{s.email}</strong>
                </p>
                {s.plan_granted ? <PlanBadge manageUsage={status.manage_usage} /> : <p className="text-destructive">{describe({ kind: 'plan_not_granted', message: '' })}</p>}
                {s.limit_reached && <p className="text-destructive">{describe({ kind: 'usage_limit', message: '' })}</p>}
              </div>
            ) : (
              <div className="mt-3 space-y-2 text-sm text-muted-foreground">
                <p>Refract uses your ChatGPT plan for AI requests. Sign-in happens on your own computer:</p>
                <ol className="list-decimal space-y-1 pl-5">
                  <li>
                    Run <code className="rounded bg-muted px-1.5 py-0.5 text-foreground">refract login</code> and choose <em>Continue with ChatGPT</em>.
                  </li>
                  <li>Upload the credentials file it writes.</li>
                </ol>
              </div>
            )}
            <div className="mt-5 flex gap-2">
              <Button variant={s?.signed_in ? 'secondary' : 'default'} disabled={busy} onClick={() => input.current?.click()}>
                <Upload /> {s?.signed_in ? 'Replace credentials…' : 'Upload credentials…'}
              </Button>
              {s?.signed_in && (
                <Button variant="ghost" disabled={busy} onClick={() => run(() => api('/api/chatgpt/signout', { method: 'POST' }), 'Signed out')}>
                  Sign out
                </Button>
              )}
              <input
                ref={input}
                type="file"
                accept=".json,application/json"
                hidden
                onChange={(e) => {
                  const f = e.target.files?.[0]
                  if (f) run(async () => api<ChatGPTStatus>('/api/chatgpt/credentials', { method: 'POST', body: await f.text() }), 'Credentials imported')
                  e.target.value = ''
                }}
              />
            </div>
          </section>
        </div>
      </main>
      <footer className="flex items-center px-1 pt-2">
        <AccountMenu status={status} />
      </footer>
    </div>
  )
}
