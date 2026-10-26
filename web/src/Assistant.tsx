import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { api, describe, errorText, stream, type ApiError, type ChatGPTStatus } from './api'
import PlanBadge from './PlanBadge'

export type Turn = {
  id: string
  message: string
  reply: string
  activity: string[]
  status: 'running' | 'review' | 'done' | 'accepted' | 'rejected' | 'failed' | 'stopped'
  error?: ApiError
  files: { path: string; status: string }[]
  commits: string[]
}
type Chat = { id: string; title: string }
export type EditorContext = { open_file: string; selection: string; from_line: number; to_line: number }

type Props = {
  projectId: string
  status: ChatGPTStatus | null
  getContext: () => EditorContext
  beforeSend: () => Promise<void> // flush autosave: the turn starts from what is on disk
  onReview: (turn: Turn) => void
  reviewing?: string
  turnUpdate?: Turn // a turn changed by the review view
}

const statusLabel: Record<Turn['status'], string> = {
  running: 'Working…',
  review: 'Waiting for your review',
  done: 'No changes',
  accepted: 'Applied',
  rejected: 'Rejected',
  failed: 'Failed',
  stopped: 'Stopped',
}

export default function Assistant(p: Props) {
  const [chats, setChats] = useState<Chat[]>([])
  const [chatId, setChatId] = useState('')
  const [turns, setTurns] = useState<Turn[]>([])
  const [draft, setDraft] = useState('')
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')
  const abort = useRef<AbortController | null>(null)
  const bottom = useRef<HTMLDivElement>(null)
  const created = useRef('') // chat created by send(): its (empty) history must not overwrite the live turn

  useEffect(() => {
    api<Chat[]>(`/api/projects/${p.projectId}/chats`)
      .then((cs) => {
        setChats(cs)
        if (cs.length) setChatId(cs[0].id)
      })
      .catch((e) => setError(errorText(e)))
  }, [p.projectId])

  useEffect(() => {
    if (!chatId) return setTurns([])
    if (chatId === created.current) return
    api<Turn[]>(`/api/chats/${chatId}/turns`)
      .then(setTurns)
      .catch((e) => setError(errorText(e)))
  }, [chatId])

  useEffect(() => {
    const u = p.turnUpdate
    if (u) setTurns((ts) => ts.map((t) => (t.id === u.id ? u : t)))
  }, [p.turnUpdate])

  // Braces matter: in recent Chrome scrollIntoView returns a Promise, which
  // React would take for a cleanup function.
  useEffect(() => {
    bottom.current?.scrollIntoView({ block: 'end' })
  }, [turns])

  async function newChat() {
    const c = await api<Chat>(`/api/projects/${p.projectId}/chats`, { method: 'POST' })
    setChats((cs) => [c, ...cs])
    setChatId(c.id)
  }

  async function send() {
    const message = draft.trim()
    if (!message || running) return
    setError('')
    setRunning(true)
    try {
      await p.beforeSend()
      let id = chatId
      if (!id) {
        const c = await api<Chat>(`/api/projects/${p.projectId}/chats`, { method: 'POST' })
        created.current = c.id
        setChats((cs) => [c, ...cs])
        setChatId(c.id)
        id = c.id
      }
      const pending: Turn = { id: 'pending', message, reply: '', activity: [], status: 'running', files: [], commits: [] }
      setTurns((ts) => [...ts, pending])
      setDraft('')
      const update = (fn: (t: Turn) => Turn) => setTurns((ts) => ts.map((t) => (t.id === 'pending' ? fn(t) : t)))
      abort.current = new AbortController()
      await stream(
        `/api/chats/${id}/turns`,
        { message, context: p.getContext() },
        (ev) => {
          if (ev.type === 'text') update((t) => ({ ...t, reply: t.reply + ev.text }))
          if (ev.type === 'activity') update((t) => ({ ...t, activity: [...t.activity, ev.activity] }))
          if (ev.type === 'turn') {
            setTurns((ts) => [...ts.filter((t) => t.id !== 'pending' && t.id !== ev.turn.id), ev.turn])
            if (ev.turn.status === 'review') p.onReview(ev.turn)
          }
          if (ev.type === 'error') update((t) => ({ ...t, status: 'failed', error: ev.error }))
        },
        abort.current.signal,
      )
    } catch (e) {
      if ((e as Error).name === 'AbortError') {
        setTurns((ts) => ts.map((t) => (t.id === 'pending' ? { ...t, status: 'stopped' } : t)))
      } else setError(errorText(e))
    } finally {
      setRunning(false)
      // Titles change after the first message.
      api<Chat[]>(`/api/projects/${p.projectId}/chats`).then(setChats).catch(() => {})
    }
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      e.stopPropagation() // not the global Cmd+Enter compile
      send()
    }
  }

  const plan = p.status?.status
  return (
    <div className="assistant">
      <div className="assistant-bar">
        <select value={chatId} onChange={(e) => setChatId(e.target.value)} aria-label="Chat" disabled={running}>
          {!chatId && <option value="">New chat</option>}
          {chats.map((c) => (
            <option key={c.id} value={c.id}>
              {c.title}
            </option>
          ))}
        </select>
        <button onClick={newChat} disabled={running}>
          New chat
        </button>
      </div>

      <div className="turns">
        {turns.length === 0 && <p className="hint">Ask for a change to the project. The assistant proposes edits as a diff; nothing is applied until you accept.</p>}
        {turns.map((t) => (
          <div key={t.id} className="turn">
            <div className="turn-user">{t.message.split('\n\n[Open file:')[0]}</div>
            {t.activity.length > 0 && (
              <ul className="turn-activity">
                {t.activity.map((a, i) => (
                  <li key={i}>{a}</li>
                ))}
              </ul>
            )}
            {t.reply && <div className="turn-reply">{t.reply}</div>}
            <div className="turn-status hint">
              {statusLabel[t.status]}
              {t.status === 'review' && (
                <button className="link" onClick={() => p.onReview(t)}>
                  {p.reviewing === t.id ? 'Reviewing' : `Review ${t.files.length} file${t.files.length === 1 ? '' : 's'}`}
                </button>
              )}
            </div>
            {t.error && (
              <p className="error">
                {describe(t.error)}
                {t.error.kind === 'usage_limit' && p.status && (
                  <>
                    {' '}
                    <a href={p.status.manage_usage} target="_blank" rel="noreferrer">
                      Manage usage
                    </a>
                  </>
                )}
              </p>
            )}
          </div>
        ))}
        <div ref={bottom} />
      </div>

      {error && <p className="error">{error}</p>}
      {plan && !plan.signed_in && <p className="error">{describe({ kind: 'not_signed_in', message: '' })}</p>}
      <div className="composer">
        <textarea
          rows={3}
          value={draft}
          placeholder="Ask the assistant… (Enter to send, Shift+Enter for a new line)"
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={onKey}
          aria-label="Message"
        />
        <div className="row">
          {plan?.plan_granted && p.status && <PlanBadge manageUsage={p.status.manage_usage} />}
          <span className="spacer" />
          {running ? (
            <button onClick={() => abort.current?.abort()}>Stop</button>
          ) : (
            <button onClick={send} disabled={!draft.trim()}>
              Send
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
