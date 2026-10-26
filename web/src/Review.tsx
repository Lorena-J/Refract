import { useEffect, useRef, useState } from 'react'
import { Check, ChevronRight, FileDiff, Loader2, X } from 'lucide-react'
import { basicSetup } from 'codemirror'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { unifiedMergeView } from '@codemirror/merge'
import { api, errorText } from './api'
import type { Turn } from './Assistant'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { latex } from '@/lib/editor'
import { cn } from '@/lib/utils'

type FileDiff = { path: string; status: 'added' | 'modified' | 'deleted'; before: string; after: string; binary: boolean }

type Props = {
  turn: Turn
  onAccept: (files: string[] | null) => Promise<string[] | null> // resolves to conflicts, if any
  onReject: () => Promise<void>
  onClose: () => void
}

const statusTone: Record<FileDiff['status'], string> = {
  added: 'text-success',
  modified: 'text-muted-foreground',
  deleted: 'text-destructive',
}

// A turn's pending changes as unified diffs, file by file.
export default function Review({ turn, onAccept, onReject, onClose }: Props) {
  const [diffs, setDiffs] = useState<FileDiff[] | null>(null)
  const [busy, setBusy] = useState<string | null>(null) // what is being applied
  const [message, setMessage] = useState('')
  const [collapsed, setCollapsed] = useState<string[]>([])

  useEffect(() => {
    api<FileDiff[]>(`/api/turns/${turn.id}/diff`)
      .then(setDiffs)
      .catch((e) => setMessage(errorText(e)))
  }, [turn.id, turn.files.length])

  async function act(what: string, fn: () => Promise<string[] | null | void>) {
    setBusy(what)
    setMessage('')
    try {
      const conflicts = await fn()
      if (conflicts && conflicts.length) {
        setMessage(`Not applied: ${conflicts.join(', ')} changed since the assistant started, in the same places. Nothing was changed. Edit the file or reject these changes.`)
      }
    } catch (e) {
      setMessage(errorText(e))
    } finally {
      setBusy(null)
    }
  }

  const stats = (d: FileDiff) => {
    if (d.binary) return null
    const a = d.before ? d.before.split('\n').length : 0
    const b = d.after ? d.after.split('\n').length : 0
    return b - a
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex h-11 shrink-0 items-center gap-2 border-b px-3">
        <FileDiff className="size-4 text-muted-foreground" />
        <span className="text-sm font-medium">Review changes</span>
        <span className="text-xs text-muted-foreground">
          {turn.files.length} file{turn.files.length === 1 ? '' : 's'}
        </span>
        <span className="flex-1" />
        <Button size="sm" variant="ghost" disabled={!!busy} onClick={() => act('reject', onReject)}>
          <X /> Reject all
        </Button>
        <Button size="sm" disabled={!!busy} onClick={() => act('all', () => onAccept(null))}>
          {busy === 'all' ? <Loader2 className="animate-spin" /> : <Check />} Accept all
        </Button>
        <Button size="icon-sm" variant="ghost" onClick={onClose} aria-label="Close review">
          <X />
        </Button>
      </div>
      {message && <p className="enter border-b bg-destructive/5 px-4 py-2.5 text-sm text-destructive">{message}</p>}
      <div className="min-h-0 flex-1 overflow-auto">
        {!diffs && !message && (
          <p className="flex items-center gap-2 p-4 text-sm text-muted-foreground">
            <Loader2 className="size-4 animate-spin" /> Loading changes…
          </p>
        )}
        {diffs?.map((d) => {
          const open = !collapsed.includes(d.path)
          const delta = stats(d)
          return (
            <section key={d.path} className="enter border-b">
              <div className="sticky top-0 z-10 flex items-center gap-2 border-b bg-card/95 px-3 py-1.5 backdrop-blur">
                <button
                  className="flex min-w-0 flex-1 items-center gap-1.5 text-left"
                  onClick={() => setCollapsed(open ? [...collapsed, d.path] : collapsed.filter((p) => p !== d.path))}
                >
                  <ChevronRight className={cn('size-3.5 shrink-0 text-muted-foreground transition-transform duration-150 ease-out', open && 'rotate-90')} />
                  <code className="truncate text-sm">{d.path}</code>
                  <Badge variant="outline" className={cn('font-normal', statusTone[d.status])}>
                    {d.status}
                  </Badge>
                  {delta !== null && delta !== 0 && (
                    <span className={cn('text-xs tabular-nums', delta > 0 ? 'text-success' : 'text-destructive')}>
                      {delta > 0 ? `+${delta}` : delta} lines
                    </span>
                  )}
                </button>
                <Button size="xs" variant="secondary" disabled={!!busy} onClick={() => act(d.path, () => onAccept([d.path]))}>
                  {busy === d.path ? <Loader2 className="animate-spin" /> : <Check />} Accept file
                </Button>
              </div>
              {open && (d.binary ? <p className="px-4 py-3 text-sm text-muted-foreground">Binary file changed.</p> : <DiffView before={d.before} after={d.after} />)}
            </section>
          )
        })}
      </div>
    </div>
  )
}

function DiffView({ before, after }: { before: string; after: string }) {
  const host = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const view = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: after,
        extensions: [
          basicSetup,
          latex,
          EditorState.readOnly.of(true),
          EditorView.editable.of(false),
          unifiedMergeView({ original: before, mergeControls: false, collapseUnchanged: { margin: 3, minSize: 6 } }),
        ],
      }),
    })
    return () => view.destroy()
  }, [before, after])
  return <div ref={host} />
}
