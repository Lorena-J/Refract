import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import {
  ArrowUp,
  Check,
  ChevronDown,
  ChevronRight,
  ChevronUp,
  Copy,
  FileDiff,
  FileText,
  MessageSquarePlus,
  Minimize2,
  PanelBottom,
  Sparkles,
  Square,
  X,
} from 'lucide-react'
import { api, describe, errorText, stream, type ApiError, type ChatGPTStatus } from './api'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { Conversation, ConversationContent } from '@/components/ui/chatcn/conversation'
import { ScrollToBottom } from '@/components/ui/chatcn/scroll-to-bottom'
import { ShimmerText } from '@/components/ui/chatcn/shimmer-text'
import { ThinkingIndicator } from '@/components/ui/chatcn/thinking-indicator'
import PlanBadge from './PlanBadge'
import Markdown from '@/components/Markdown'
import { openSettings, setPref, usePrefs } from '@/lib/prefs'
import { ago } from '@/lib/time'
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
  contextLabel?: string // what getContext will send, shown as a chip: "main.tex · L3–9"
  beforeSend: () => Promise<void> // flush autosave: the turn starts from what is on disk
  onReview: (turn: Turn) => void
  reviewing?: string
  turnUpdate?: Turn // a turn changed by the review view
  open: boolean // the chat sheet over the editor; collapsed it is an input bar or a bubble
  onOpen: (open: boolean) => void
  focus?: number // bump to focus the input
}

const finished: Partial<Record<Turn['status'], { label: string; tone: string }>> = {
  done: { label: 'No changes', tone: 'text-muted-foreground' },
  accepted: { label: 'Changes applied', tone: 'text-success' },
  rejected: { label: 'Changes rejected', tone: 'text-muted-foreground' },
  stopped: { label: 'Stopped', tone: 'text-muted-foreground' },
}

const suggestions = ['Fix the compile errors', 'Check the grammar of the open file', 'Make the abstract more concise']
const MIN_HEIGHT = 260

// The assistant floats over the editor (Prism-style): an input bar, a bubble, or a resizable sheet.
export default function Assistant(p: Props) {
  const prefs = usePrefs()
  const [turns, setTurns] = useState<Turn[]>([])
  const [draft, setDraft] = useState('')
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')
  const [height, setHeight] = useState<number | null>(null) // while dragging; prefs keep the last one
  const abort = useRef<AbortController | null>(null)
  const input = useRef<HTMLTextAreaElement>(null)
  const overlay = useRef<HTMLDivElement>(null)
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

  useEffect(() => {
    if (p.focus) input.current?.focus()
  }, [p.focus])

  const bubble = !p.open && prefs.chatBubble

  // Grow the input with its content (up to a cap).
  useEffect(() => {
    const el = input.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 200)}px`
  }, [draft, p.open, bubble])

  async function newChat() {
    if (running) return
    p.onChatId('')
    setError('')
    p.onOpen(true)
    input.current?.focus()
  }

  async function send(text = draft) {
    const message = text.trim()
    if (!message || running) return
    setError('')
    setRunning(true)
    p.onOpen(true)
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

  // Esc collapses the sheet; menus portalled out of it handle their own Esc.
  function onCardKey(e: KeyboardEvent) {
    if (e.key === 'Escape' && p.open && e.currentTarget.contains(e.target as Node)) {
      e.preventDefault()
      p.onOpen(false)
    }
  }

  // Drag the top edge to resize the sheet. Pointer capture keeps the drag when leaving the handle.
  function startResize(e: PointerEvent) {
    e.preventDefault()
    e.currentTarget.setPointerCapture(e.pointerId)
  }
  function resize(e: PointerEvent) {
    if (!e.currentTarget.hasPointerCapture(e.pointerId) || !overlay.current) return
    const r = overlay.current.getBoundingClientRect()
    setHeight(Math.max(MIN_HEIGHT, Math.min(r.bottom - 12 - e.clientY, r.height - 24)))
  }
  function endResize() {
    if (height !== null) setPref('chatHeight', Math.round(height))
    setHeight(null)
  }

  const plan = p.status?.status
  const notice = error || (plan && !plan.signed_in ? describe({ kind: 'not_signed_in', message: '' }) : '')
  const title = p.chats.find((c) => c.id === p.chatId)?.title ?? 'New chat'
  const last = turns[turns.length - 1]
  const pendingReview = last?.status === 'review' ? last : undefined
  const sheetHeight = height ?? prefs.chatHeight

  if (bubble) {
    return (
      <div className="pointer-events-none absolute inset-0 z-20 flex items-end justify-end p-4">
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              onClick={() => p.onOpen(true)}
              aria-label="Open assistant"
              className="pointer-events-auto relative flex size-11 origin-bottom-right items-center justify-center rounded-full bg-primary text-primary-foreground shadow-lg transition-[scale,opacity] duration-200 ease-out starting:scale-90 starting:opacity-0 hover:scale-105 active:scale-95"
            >
              <Sparkles className="size-5" />
              {(running || pendingReview) && (
                <span className={cn('absolute top-0 right-0 size-3 rounded-full border-2 border-card', running ? 'animate-pulse bg-muted-foreground' : 'bg-warning')} />
              )}
            </button>
          </TooltipTrigger>
          <TooltipContent side="left">{running ? 'Assistant is working…' : pendingReview ? 'Changes ready to review' : 'Assistant'}</TooltipContent>
        </Tooltip>
      </div>
    )
  }

  return (
    <div ref={overlay} className="pointer-events-none absolute inset-0 z-20 flex flex-col justify-end p-3">
      <div
        onKeyDown={onCardKey}
        style={p.open ? { height: sheetHeight ? sheetHeight : '55%' } : undefined}
        className={cn(
          'pointer-events-auto relative mx-auto flex max-h-full w-full flex-col rounded-2xl border bg-card/95 shadow-lg backdrop-blur-md',
          // The bar appears from the bubble's corner; the sheet just grows (its content fades in).
          'origin-bottom-right transition-[scale,opacity] duration-200 ease-out starting:scale-95 starting:opacity-0',
          p.open ? 'min-h-[260px]' : 'max-w-3xl',
        )}
      >
        {p.open && (
          <>
            <div
              role="separator"
              aria-orientation="horizontal"
              aria-label="Resize assistant"
              onPointerDown={startResize}
              onPointerMove={resize}
              onPointerUp={endResize}
              onPointerCancel={endResize}
              onDoubleClick={() => setPref('chatHeight', 0)}
              className="group/handle absolute inset-x-0 -top-1.5 z-10 flex h-4 cursor-ns-resize touch-none justify-center pt-2"
            >
              <span className="h-1 w-10 rounded-full bg-border transition-[background-color,width] duration-150 group-hover/handle:w-14 group-hover/handle:bg-muted-foreground/50" />
            </div>
            <div className="enter flex h-11 shrink-0 items-center gap-1 px-2 pt-1">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="icon-sm" className="text-muted-foreground" onClick={() => p.onOpen(false)} aria-label="Collapse assistant">
                    <ChevronDown />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  Collapse <kbd className="ml-1 opacity-70">Esc</kbd>
                </TooltipContent>
              </Tooltip>
              <DropdownMenu>
                <DropdownMenuTrigger asChild disabled={running}>
                  <Button variant="ghost" size="sm" className="min-w-0 gap-1 font-medium">
                    <span className="truncate">{title}</span> <ChevronDown className="opacity-50" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" className="max-h-80 w-72">
                  <DropdownMenuItem onSelect={newChat}>
                    <MessageSquarePlus /> New chat
                  </DropdownMenuItem>
                  {p.chats.length > 0 && (
                    <>
                      <DropdownMenuSeparator />
                      <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">Recent chats</DropdownMenuLabel>
                    </>
                  )}
                  {p.chats.map((c) => (
                    <DropdownMenuItem key={c.id} onSelect={() => p.onChatId(c.id)}>
                      <span className="truncate">{c.title}</span>
                      <span className="ml-auto shrink-0 text-xs text-muted-foreground">{ago(c.updated_at)}</span>
                      <Check className={cn(c.id !== p.chatId && 'hidden')} />
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
              <span className="flex-1" />
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="icon-sm" className="text-muted-foreground" onClick={newChat} disabled={running} aria-label="New chat">
                    <MessageSquarePlus />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>New chat</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="text-muted-foreground"
                    onClick={() => {
                      setPref('chatBubble', !prefs.chatBubble)
                      p.onOpen(false)
                    }}
                    aria-label={prefs.chatBubble ? 'Collapse to input bar' : 'Minimize to bubble'}
                  >
                    {prefs.chatBubble ? <PanelBottom /> : <Minimize2 />}
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{prefs.chatBubble ? 'Collapse to input bar' : 'Minimize to bubble'}</TooltipContent>
              </Tooltip>
            </div>

            <Conversation className="enter min-h-0 flex-1">
              <ConversationContent className="max-w-3xl gap-6 px-5 pt-3 pb-4 sm:px-6">
                {turns.length === 0 && (
                  <div className="mx-auto mt-6 max-w-sm text-center">
                    <p className="text-sm font-medium">Ask for a change</p>
                    <p className="mt-1 text-sm text-muted-foreground">
                      The assistant reads your project, edits a copy and proposes a diff. Nothing changes until you accept.
                    </p>
                    <div className="mt-4 flex flex-wrap justify-center gap-1.5">
                      {suggestions.map((s) => (
                        <button
                          key={s}
                          className="rounded-full border px-3 py-1 text-xs text-muted-foreground transition-[color,background-color,scale] duration-150 hover:bg-accent hover:text-foreground active:scale-[0.97]"
                          onClick={() => {
                            setDraft(s)
                            input.current?.focus()
                          }}
                        >
                          {s}
                        </button>
                      ))}
                    </div>
                  </div>
                )}
                {turns.map((t) => (
                  <TurnView key={t.id} turn={t} status={p.status} reviewing={p.reviewing === t.id} onReview={() => p.onReview(t)} />
                ))}
              </ConversationContent>
              <ScrollToBottom />
            </Conversation>
          </>
        )}

        {!p.open && (running || pendingReview) && (
          <div className="enter flex items-center gap-2 border-b px-4 py-2 text-xs">
            {running ? (
              <ShimmerText className="min-w-0 truncate">{last?.activity[last.activity.length - 1] ?? 'Thinking'}</ShimmerText>
            ) : (
              <span className="flex items-center gap-1.5 text-muted-foreground">
                <FileDiff className="size-3.5" /> {pendingReview!.files.length} file{pendingReview!.files.length === 1 ? '' : 's'} changed
              </span>
            )}
            <span className="flex-1" />
            {pendingReview && !running && p.reviewing !== pendingReview.id && (
              <Button size="xs" variant="secondary" onClick={() => p.onReview(pendingReview)}>
                Review
              </Button>
            )}
            <Button size="xs" variant="ghost" onClick={() => p.onOpen(true)}>
              Open chat
            </Button>
          </div>
        )}

        {notice && (
          <p className="enter flex items-center gap-2 px-4 pt-2 text-xs text-destructive">
            <span className="min-w-0 flex-1">{notice}</span>
            {!error && (
              <Button size="xs" variant="ghost" onClick={() => openSettings('assistant')}>
                Open settings
              </Button>
            )}
          </p>
        )}

        <div className={cn('shrink-0', p.open && 'p-2 pt-0')}>
          <div
            className={cn(
              'flex transition-[border-color,box-shadow] duration-150',
              p.open ? 'flex-col rounded-xl border bg-background focus-within:border-ring' : 'items-end',
            )}
          >
            <textarea
              ref={input}
              rows={1}
              value={draft}
              placeholder={p.open ? 'Ask for a change, or ask about the document…' : 'Ask anything'}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={onKey}
              aria-label="Message the assistant"
              className={cn(
                'block max-h-[200px] min-w-0 flex-1 resize-none bg-transparent text-sm outline-none placeholder:text-muted-foreground',
                p.open ? 'px-3.5 pt-3 pb-1' : 'px-4 py-3',
              )}
            />
            <div className={cn('flex shrink-0 items-center gap-1', p.open ? 'px-2 pb-2' : 'py-2 pr-2')}>
              {p.open && (
                <>
                  {p.contextLabel && (
                    <span className="flex min-w-0 items-center gap-1 rounded-md bg-foreground/[0.05] px-1.5 py-0.5 text-xs text-muted-foreground" title="Sent with your message">
                      <FileText className="size-3 shrink-0" />
                      <span className="truncate">{p.contextLabel}</span>
                    </span>
                  )}
                  <span className="flex-1" />
                  {plan?.plan_granted && p.status && <PlanBadge manageUsage={p.status.manage_usage} />}
                </>
              )}
              {!p.open && turns.length > 0 && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button variant="ghost" size="icon-sm" className="text-muted-foreground" onClick={() => p.onOpen(true)} aria-label="Show chat">
                      <ChevronUp />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>Show chat</TooltipContent>
                </Tooltip>
              )}
              {!p.open && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button variant="ghost" size="icon-sm" className="text-muted-foreground" onClick={() => setPref('chatBubble', true)} aria-label="Minimize to bubble">
                      <Minimize2 />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>Minimize to bubble</TooltipContent>
                </Tooltip>
              )}
              {running ? (
                <Button size="icon-sm" variant="secondary" className="rounded-full" onClick={() => abort.current?.abort()} aria-label="Stop">
                  <Square className="size-3 fill-current" />
                </Button>
              ) : (
                <Button size="icon-sm" className="rounded-full" onClick={() => send()} disabled={!draft.trim()} aria-label="Send">
                  <ArrowUp />
                </Button>
              )}
            </div>
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
    <div className="flex flex-col gap-3">
      <div data-slot="message" data-role="user" className="enter ml-auto w-fit max-w-[85%] rounded-2xl rounded-br-md bg-secondary px-3.5 py-2 text-sm whitespace-pre-wrap">
        {request}
      </div>

      {t.activity.length > 0 &&
        (running ? (
          <p className="flex min-w-0 items-center gap-2 text-xs">
            <ShimmerText key={t.activity.length} className="enter truncate font-mono font-normal">
              {t.activity[t.activity.length - 1]}
            </ShimmerText>
          </p>
        ) : (
          <div>
            <button
              className="flex items-center gap-1 text-xs text-muted-foreground transition-colors duration-100 hover:text-foreground"
              onClick={() => setShowSteps(!showSteps)}
              aria-expanded={showSteps}
            >
              <ChevronRight className={cn('size-3.5 transition-transform duration-150 ease-out', showSteps && 'rotate-90')} />
              Worked · {t.activity.length} step{t.activity.length === 1 ? '' : 's'}
            </button>
            {showSteps && (
              <ul className="enter mt-1.5 ml-1.5 space-y-0.5 border-l pl-3 font-mono text-xs text-muted-foreground">
                {t.activity.map((a, i) => (
                  <li key={i}>{a}</li>
                ))}
              </ul>
            )}
          </div>
        ))}

      <ThinkingIndicator active={running && t.activity.length === 0 && !t.reply} />

      {t.reply && (
        <div className="group/reply">
          <div className="text-sm leading-relaxed">
            <Markdown text={t.reply} />
          </div>
          {!running && <CopyButton text={t.reply} />}
        </div>
      )}

      {t.status === 'review' && (
        <div className="enter rounded-xl border bg-background p-3">
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
        <p className={cn('flex items-center gap-1.5 text-xs', done.tone)}>
          {t.status === 'accepted' ? <Check className="size-3.5" /> : t.status === 'rejected' ? <X className="size-3.5" /> : null}
          {done.label}
        </p>
      )}

      {t.error && (
        <div className="enter rounded-xl border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
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

// Copy → check swap, blurred crossfade so the two icons read as one morph.
function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    if (!copied) return
    const t = setTimeout(() => setCopied(false), 1500)
    return () => clearTimeout(t)
  }, [copied])
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon-xs"
          className="mt-1 text-muted-foreground opacity-60 transition-opacity duration-150 group-hover/reply:opacity-100 focus-visible:opacity-100"
          aria-label="Copy reply"
          onClick={() => navigator.clipboard.writeText(text).then(() => setCopied(true))}
        >
          <span key={String(copied)} className="grid animate-[chatcn-swap_150ms_var(--ease-out)] place-items-center">
            {copied ? <Check /> : <Copy />}
          </span>
        </Button>
      </TooltipTrigger>
      <TooltipContent>{copied ? 'Copied' : 'Copy'}</TooltipContent>
    </Tooltip>
  )
}
