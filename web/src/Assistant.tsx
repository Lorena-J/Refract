import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { createPortal } from 'react-dom'
import { ArrowUp, Check, ChevronDown, ChevronRight, FileDiff, Loader2, MessageSquarePlus, Square, X } from 'lucide-react'
import { api, describe, errorText, stream, type ApiError, type ChatGPTStatus } from './api'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import PlanBadge from './PlanBadge'
import Markdown from '@/components/Markdown'
import { cn } from '@/lib/utils'

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
export type Chat = { id: string; title: string; updated_at: string }
export type EditorContext = { open_file: string; selection: string; from_line: number; to_line: number }

type Props = {
  projectId: string
  status: ChatGPTStatus | null
  chats: Chat[]
  chatId: string
  onChatId: (id: string) => void
  onChatsChanged: () => void
  getContext: () => EditorContext
  beforeSend: () => Promise<void> // flush autosave: the turn starts from what is on disk
  onReview: (turn: Turn) => void
  reviewing?: string
  turnUpdate?: Turn // a turn changed by the review view
  toolbarTarget?: HTMLElement | null // render the chat controls there (shared header row)
}

const finished: Partial<Record<Turn['status'], { label: string; tone: string }>> = {
  done: { label: 'No changes', tone: 'text-muted-foreground' },
  accepted: { label: 'Changes applied', tone: 'text-success' },
  rejected: { label: 'Changes rejected', tone: 'text-muted-foreground' },
  stopped: { label: 'Stopped', tone: 'text-muted-foreground' },
}

export default function Assistant(p: Props) {
  const [turns, setTurns] = useState<Turn[]>([])
  const [draft, setDraft] = useState('')
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')
  const abort = useRef<AbortController | null>(null)
  const bottom = useRef<HTMLDivElement>(null)
  const input = useRef<HTMLTextAreaElement>(null)
  const created = useRef('') // chat created by send(): its (empty) history must not overwrite the live turn

  useEffect(() => {
    if (!p.chatId) return setTurns([])
    if (p.chatId === created.current) return
    api<Turn[]>(`/api/chats/${p.chatId}/turns`)
      .then(setTurns)
      .catch((e) => setError(errorText(e)))
  }, [p.chatId])

  useEffect(() => {
    const u = p.turnUpdate
    if (u) setTurns((ts) => ts.map((t) => (t.id === u.id ? u : t)))
  }, [p.turnUpdate])

  // Braces matter: in recent Chrome scrollIntoView returns a Promise, which
  // React would take for a cleanup function.
  useEffect(() => {
    bottom.current?.scrollIntoView({ block: 'end' })
  }, [turns])

  // Grow the composer with its content (up to a cap).
  useEffect(() => {
    const el = input.current
    if (!el || el.offsetParent === null) return // hidden tab: nothing to measure
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 200)}px`
  }, [draft])

  async function newChat() {
    const c = await api<Chat>(`/api/projects/${p.projectId}/chats`, { method: 'POST' })
    p.onChatsChanged()
    p.onChatId(c.id)
    input.current?.focus()
  }

  async function send() {
    const message = draft.trim()
    if (!message || running) return
    setError('')
    setRunning(true)
    try {
      await p.beforeSend()
      let id = p.chatId
      if (!id) {
        const c = await api<Chat>(`/api/projects/${p.projectId}/chats`, { method: 'POST' })
        created.current = c.id
        p.onChatId(c.id)
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
      p.onChatsChanged() // titles change after the first message
    }
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      e.stopPropagation() // not the global Cmd+Enter compile
      send()
    }
  }

  const plan = p.status?.status
  const title = p.chats.find((c) => c.id === p.chatId)?.title ?? 'New chat'
  const controls = (
    <>
        <DropdownMenu>
          <DropdownMenuTrigger asChild disabled={running}>
            <Button variant="ghost" size="sm" className="min-w-0 gap-1 font-normal">
              <span className="truncate">{title}</span> <ChevronDown className="opacity-60" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="max-h-80 w-72">
            <DropdownMenuItem onSelect={newChat}>
              <MessageSquarePlus /> New chat
            </DropdownMenuItem>
            {p.chats.length > 0 && <DropdownMenuSeparator />}
            {p.chats.map((c) => (
              <DropdownMenuItem key={c.id} onSelect={() => p.onChatId(c.id)}>
                <span className="truncate">{c.title}</span>
                <Check className={cn('ml-auto', c.id !== p.chatId && 'invisible')} />
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        <span className="flex-1" />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon-sm" onClick={newChat} disabled={running} aria-label="New chat">
              <MessageSquarePlus />
            </Button>
          </TooltipTrigger>
          <TooltipContent>New chat</TooltipContent>
        </Tooltip>
    </>
  )
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {p.toolbarTarget ? createPortal(controls, p.toolbarTarget) : <div className="flex h-11 shrink-0 items-center gap-1 border-b px-2">{controls}</div>}

      <div className="min-h-0 flex-1 overflow-auto px-4 py-4">
        {turns.length === 0 && (
          <div className="enter mx-auto mt-16 max-w-xs text-center">
            <p className="text-sm font-medium">Ask for a change</p>
            <p className="mt-1 text-sm text-muted-foreground">
              The assistant reads your project, edits a copy and proposes a diff. Nothing changes until you accept.
            </p>
          </div>
        )}
        {turns.map((t) => (
          <TurnView key={t.id} turn={t} status={p.status} reviewing={p.reviewing === t.id} onReview={() => p.onReview(t)} />
        ))}
        <div ref={bottom} />
      </div>

      <div className="shrink-0 p-3 pt-0">
        {(error || (plan && !plan.signed_in)) && (
          <p className="enter mb-2 text-sm text-destructive">{error || describe({ kind: 'not_signed_in', message: '' })}</p>
        )}
        <div className="rounded-2xl border bg-background shadow-xs transition-[border-color,box-shadow] duration-150 focus-within:border-ring focus-within:shadow-sm">
          <textarea
            ref={input}
            rows={2}
            value={draft}
            placeholder="Ask the assistant…"
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={onKey}
            aria-label="Message"
            className="block max-h-[200px] w-full resize-none bg-transparent px-4 pt-3 text-sm outline-none placeholder:text-muted-foreground"
          />
          <div className="flex items-center gap-2 px-3 pt-1 pb-2.5">
            {plan?.plan_granted && p.status ? <PlanBadge manageUsage={p.status.manage_usage} /> : <span className="text-xs text-muted-foreground">Enter to send · Shift+Enter for a new line</span>}
            <span className="flex-1" />
            {running ? (
              <Button size="icon-sm" variant="secondary" className="rounded-full" onClick={() => abort.current?.abort()} aria-label="Stop">
                <Square className="size-3 fill-current" />
              </Button>
            ) : (
              <Button size="icon-sm" className="rounded-full" onClick={send} disabled={!draft.trim()} aria-label="Send">
                <ArrowUp />
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

function TurnView({ turn: t, status, reviewing, onReview }: { turn: Turn; status: ChatGPTStatus | null; reviewing: boolean; onReview: () => void }) {
  const [showSteps, setShowSteps] = useState(false)
  const request = t.message.split('\n\n[Open file:')[0]
  const running = t.status === 'running'
  const done = finished[t.status]
  return (
    <div className="mb-6">
      <div className="enter ml-auto w-fit max-w-[85%] rounded-2xl rounded-br-md bg-secondary px-3.5 py-2 text-sm whitespace-pre-wrap">{request}</div>

      {t.activity.length > 0 && (
        <div className="mt-3">
          {running ? (
            <p className="flex items-center gap-2 text-xs text-muted-foreground">
              <Loader2 className="size-3.5 animate-spin" />
              <span key={t.activity.length} className="enter font-mono">
                {t.activity[t.activity.length - 1]}
              </span>
            </p>
          ) : (
            <button
              className="flex items-center gap-1 text-xs text-muted-foreground transition-colors duration-100 hover:text-foreground"
              onClick={() => setShowSteps(!showSteps)}
            >
              <ChevronRight className={cn('size-3.5 transition-transform duration-150 ease-out', showSteps && 'rotate-90')} />
              Worked · {t.activity.length} step{t.activity.length === 1 ? '' : 's'}
            </button>
          )}
          {showSteps && !running && (
            <ul className="enter mt-1.5 ml-1.5 space-y-0.5 border-l pl-3 font-mono text-xs text-muted-foreground">
              {t.activity.map((a, i) => (
                <li key={i}>{a}</li>
              ))}
            </ul>
          )}
        </div>
      )}

      {running && t.activity.length === 0 && !t.reply && (
        <p className="mt-3 flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="size-3.5 animate-spin" /> Thinking…
        </p>
      )}

      {t.reply && (
        <div className="mt-3 text-sm leading-relaxed">
          <Markdown text={t.reply} />
        </div>
      )}

      {t.status === 'review' && (
        <div className="enter mt-3 rounded-xl border bg-background p-3">
          <div className="flex items-center gap-2 text-sm font-medium">
            <FileDiff className="size-4 text-muted-foreground" /> {t.files.length} file{t.files.length === 1 ? '' : 's'} changed
          </div>
          <ul className="mt-1.5 space-y-0.5 font-mono text-xs text-muted-foreground">
            {t.files.slice(0, 5).map((f) => (
              <li key={f.path} className="truncate">
                {f.path}
              </li>
            ))}
            {t.files.length > 5 && <li>… and {t.files.length - 5} more</li>}
          </ul>
          <Button size="sm" className="mt-3 w-full" variant={reviewing ? 'secondary' : 'default'} onClick={onReview}>
            {reviewing ? 'Reviewing…' : 'Review changes'}
          </Button>
        </div>
      )}

      {done && (
        <p className={cn('mt-2 flex items-center gap-1.5 text-xs', done.tone)}>
          {t.status === 'accepted' ? <Check className="size-3.5" /> : t.status === 'rejected' ? <X className="size-3.5" /> : null}
          {done.label}
        </p>
      )}

      {t.error && (
        <div className="enter mt-3 rounded-xl border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
          {describe(t.error)}
          {t.error.kind === 'usage_limit' && status && (
            <>
              {' '}
              <a className="underline underline-offset-4" href={status.manage_usage} target="_blank" rel="noreferrer">
                Manage usage
              </a>
            </>
          )}
        </div>
      )}
    </div>
  )
}
