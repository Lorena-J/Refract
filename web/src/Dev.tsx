import { useEffect, useRef, useState } from 'react'
import { api, describe, stream, type ChatGPTStatus } from './api'
import PlanBadge from './PlanBadge'

type Model = { slug: string; display_name: string }

// Dev-only SIWC spike page (needs REFRACT_DEV=1 on the server).
export default function Dev({ status, refresh }: { status: ChatGPTStatus | null; refresh: () => void }) {
  const [models, setModels] = useState<Model[]>([])
  const [model, setModel] = useState('')
  const [prompt, setPrompt] = useState('What time is it on the server? Use the tool.')
  const [tools, setTools] = useState(true)
  const [log, setLog] = useState<string[]>([])
  const [text, setText] = useState('')
  const [running, setRunning] = useState(false)
  const abort = useRef<AbortController | null>(null)

  useEffect(() => {
    if (!status?.status.signed_in) return
    api<Model[]>('/api/chatgpt/models')
      .then((ms) => {
        setModels(ms ?? [])
        setModel((m) => m || status.default_model || ms?.[0]?.slug || '')
      })
      .catch((e) => setLog((l) => [...l, 'models: ' + describe(e.error ?? { kind: 'other', message: String(e) })]))
  }, [status?.status.signed_in, status?.default_model])

  async function send() {
    setRunning(true)
    setText('')
    setLog([])
    abort.current = new AbortController()
    try {
      await stream(
        '/api/dev/chat',
        { model, prompt, tools },
        (ev) => {
          if (ev.type === 'text') setText((t) => t + ev.text)
          if (ev.type === 'tool') setLog((l) => [...l, `tool ${ev.name}(${ev.arguments}) → ${ev.result}`])
          if (ev.type === 'done') setLog((l) => [...l, `done · ${ev.usage?.total_tokens ?? '?'} tokens`])
          if (ev.type === 'error') setLog((l) => [...l, 'error: ' + describe(ev.error)])
        },
        abort.current.signal,
      )
    } catch (e) {
      if ((e as Error).name !== 'AbortError') setLog((l) => [...l, String(e)])
    } finally {
      setRunning(false)
      refresh()
    }
  }

  async function expire() {
    await api('/api/dev/expire', { method: 'POST' }).catch((e) => setLog((l) => [...l, describe(e.error)]))
    setLog((l) => [...l, 'access token marked expired: next call must refresh'])
  }

  return (
    <div className="page">
      <h1>SIWC spike</h1>
      <p>
        {status?.status.signed_in ? `Signed in as ${status.status.email}` : 'Not signed in.'} <a href="/settings">Settings</a>
      </p>
      <div className="composer">
        <textarea rows={3} value={prompt} onChange={(e) => setPrompt(e.target.value)} />
        <div className="row">
          <select value={model} onChange={(e) => setModel(e.target.value)} aria-label="Model">
            {models.map((m) => (
              <option key={m.slug} value={m.slug}>
                {m.display_name || m.slug}
              </option>
            ))}
            {model && !models.some((m) => m.slug === model) && <option value={model}>{model}</option>}
          </select>
          <label>
            <input type="checkbox" checked={tools} onChange={(e) => setTools(e.target.checked)} /> function tool
          </label>
          {status?.status.plan_granted && <PlanBadge manageUsage={status.manage_usage} />}
          <span className="spacer" />
          <button onClick={expire} disabled={running}>
            Expire token
          </button>
          {running ? (
            <button onClick={() => abort.current?.abort()}>Stop</button>
          ) : (
            <button onClick={send} disabled={!prompt || !model}>
              Send
            </button>
          )}
        </div>
      </div>
      {text && <pre className="output">{text}</pre>}
      {log.length > 0 && <pre className="log">{log.join('\n')}</pre>}
    </div>
  )
}
