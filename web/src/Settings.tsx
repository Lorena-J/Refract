import { useState } from 'react'
import { api, describe, RequestError, type ChatGPTStatus } from './api'
import PlanBadge from './PlanBadge'

export default function Settings({ status, onChange }: { status: ChatGPTStatus | null; onChange: (s: ChatGPTStatus) => void }) {
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function run(fn: () => Promise<ChatGPTStatus>) {
    setBusy(true)
    setError('')
    try {
      onChange(await fn())
    } catch (e) {
      setError(e instanceof RequestError ? describe(e.error) : String(e))
    } finally {
      setBusy(false)
    }
  }

  async function upload(file: File) {
    await run(async () => api<ChatGPTStatus>('/api/chatgpt/credentials', { method: 'POST', body: await file.text() }))
  }

  if (!status) return <div className="page">Loading…</div>
  const s = status.status

  return (
    <div className="page">
      <h1>Settings</h1>
      <section>
        <h2>ChatGPT</h2>
        {s.signed_in ? (
          <>
            <p>
              Signed in as <strong>{s.email}</strong>
            </p>
            {s.plan_granted ? (
              <p>
                <PlanBadge manageUsage={status.manage_usage} />
              </p>
            ) : (
              <p className="error">{describe({ kind: 'plan_not_granted', message: '' })}</p>
            )}
            {s.limit_reached && <p className="error">{describe({ kind: 'usage_limit', message: '' })}</p>}
            <button disabled={busy} onClick={() => run(() => api('/api/chatgpt/signout', { method: 'POST' }))}>
              Sign out
            </button>
          </>
        ) : (
          <>
            <p>
              Refract uses your ChatGPT plan for AI requests. Sign-in happens on your own computer:
            </p>
            <ol>
              <li>
                Run <code>refract login</code> and choose <em>Continue with ChatGPT</em> in the browser.
              </li>
              <li>Upload the credentials file it writes:</li>
            </ol>
          </>
        )}
        <p>
          <label className="file">
            {s.signed_in ? 'Replace credentials…' : 'Upload credentials…'}
            <input type="file" accept=".json,application/json" disabled={busy} onChange={(e) => e.target.files?.[0] && upload(e.target.files[0])} />
          </label>
        </p>
        {error && <p className="error">{error}</p>}
      </section>
    </div>
  )
}
