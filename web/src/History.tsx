import { useEffect, useState } from 'react'
import { ChevronRight, History as HistoryIcon, Loader2, RotateCcw } from 'lucide-react'
import { toast } from 'sonner'
import { api, errorText } from './api'
import { DiffView } from './Review'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { ago } from '@/lib/time'
import { cn } from '@/lib/utils'
import { t } from '@/lib/i18n'

type Commit = { hash: string; short: string; message: string; time: string }
type FileDiff = { path: string; status: 'added' | 'modified' | 'deleted'; before: string; after: string; binary: boolean }

type Props = {
  projectId: string
  open: boolean
  onClose: () => void
  beforeRestore: () => Promise<void> // save pending edits first
  onRestored: () => Promise<void> // reload what is open
}

const tone: Record<FileDiff['status'], string> = { added: 'text-success', modified: 'text-muted-foreground', deleted: 'text-destructive' }
const statusLabel: Record<FileDiff['status'], string> = { added: t('added'), modified: t('modified'), deleted: t('deleted') }

// The project's commits, what each changed, and restoring a file or the whole
// project as a new commit (history is never rewritten).
export default function History({ projectId, open, onClose, beforeRestore, onRestored }: Props) {
  const [commits, setCommits] = useState<Commit[] | null>(null)
  const [selected, setSelected] = useState<string | null>(null)
  const [diffs, setDiffs] = useState<FileDiff[] | null>(null)
  const [collapsed, setCollapsed] = useState<string[]>([])
  const [busy, setBusy] = useState('')
  const [confirming, setConfirming] = useState(false)

  const load = () =>
    api<Commit[]>(`/api/projects/${projectId}/history`)
      .then((cs) => {
        setCommits(cs)
        setSelected((s) => s ?? cs[0]?.hash ?? null)
      })
      .catch((e) => toast.error(errorText(e)))

  useEffect(() => {
    if (open) load()
    else {
      setCommits(null)
      setSelected(null)
    }
  }, [open, projectId])

  useEffect(() => {
    setDiffs(null)
    setConfirming(false)
    setCollapsed([])
    if (!selected) return
    api<FileDiff[]>(`/api/projects/${projectId}/history/${selected}`)
      .then(setDiffs)
      .catch((e) => toast.error(errorText(e)))
  }, [selected, projectId])

  async function restore(what: string, body: { path?: string; before?: boolean }) {
    if (!selected) return
    setBusy(what)
    try {
      await beforeRestore()
      const res = await api<{ commit: string }>(`/api/projects/${projectId}/history/${selected}/restore`, { method: 'POST', body })
      if (!res.commit) toast(t('Already identical: nothing to restore.'))
      else {
        toast.success(body.path ? t('Restored {path}', { path: body.path }) : t('Restored the project'))
        await onRestored()
        setSelected(null)
        await load()
      }
    } catch (e) {
      toast.error(errorText(e))
    } finally {
      setBusy('')
      setConfirming(false)
    }
  }

  const current = commits?.find((c) => c.hash === selected)
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="h-[min(720px,calc(100dvh-2rem))] gap-0 overflow-hidden rounded-2xl p-0 sm:max-w-[1100px]">
        <DialogTitle className="sr-only">{t('Project history')}</DialogTitle>
        <DialogDescription className="sr-only">{t('Every saved version of the project, what changed, and restoring older versions.')}</DialogDescription>
        <div className="flex h-full min-h-0">
          <aside className="flex w-72 shrink-0 flex-col border-r bg-foreground/[0.025]">
            <h2 className="flex items-center gap-2 px-4 pt-4 pb-2 text-sm font-medium">
              <HistoryIcon className="size-4 text-muted-foreground" /> {t('History')}
            </h2>
            <ol className="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
              {!commits && (
                <li className="flex items-center gap-2 p-2 text-sm text-muted-foreground">
                  <Loader2 className="size-4 animate-spin" /> {t('Loading…')}
                </li>
              )}
              {commits?.map((c) => (
                <li key={c.hash}>
                  <button
                    onClick={() => setSelected(c.hash)}
                    className={cn(
                      'w-full rounded-lg px-2.5 py-2 text-left transition-colors duration-100',
                      c.hash === selected ? 'bg-foreground/[0.07]' : 'hover:bg-foreground/[0.04]',
                    )}
                  >
                    <span className="line-clamp-2 text-sm">{c.message}</span>
                    <span className="mt-0.5 flex gap-2 text-xs text-muted-foreground">
                      <span title={new Date(c.time).toLocaleString()}>{ago(c.time)}</span>
                      <code>{c.short}</code>
                    </span>
                  </button>
                </li>
              ))}
            </ol>
          </aside>

          <section className="flex min-w-0 flex-1 flex-col">
            <header className="flex min-h-14 shrink-0 items-center gap-3 border-b py-2 pr-12 pl-4">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{current?.message ?? ' '}</p>
                {current && <p className="text-xs text-muted-foreground">{new Date(current.time).toLocaleString()}</p>}
              </div>
              {current &&
                (confirming ? (
                  <div className="enter flex items-center gap-2 text-xs">
                    <span className="text-muted-foreground">{t('Replace every file with this version? The current one stays in history.')}</span>
                    <Button size="sm" variant="ghost" onClick={() => setConfirming(false)}>
                      {t('Cancel')}
                    </Button>
                    <Button size="sm" disabled={!!busy} onClick={() => restore('project', {})}>
                      {busy === 'project' && <Loader2 className="animate-spin" />} {t('Restore')}
                    </Button>
                  </div>
                ) : (
                  <Button size="sm" variant="secondary" onClick={() => setConfirming(true)}>
                    <RotateCcw /> {t('Restore project to this version')}
                  </Button>
                ))}
            </header>
            <div className="min-h-0 flex-1 overflow-auto">
              {selected && !diffs && (
                <p className="flex items-center gap-2 p-4 text-sm text-muted-foreground">
                  <Loader2 className="size-4 animate-spin" /> {t('Loading changes…')}
                </p>
              )}
              {diffs?.length === 0 && <p className="p-4 text-sm text-muted-foreground">{t('No file changes in this version.')}</p>}
              {diffs?.map((d) => {
                const isOpen = !collapsed.includes(d.path)
                const restoreBefore = d.status === 'deleted'
                return (
                  <section key={d.path} className="border-b">
                    <div className="sticky top-0 z-10 flex items-center gap-2 border-b bg-popover/95 px-3 py-1.5 backdrop-blur">
                      <button
                        className="flex min-w-0 flex-1 items-center gap-1.5 text-left"
                        onClick={() => setCollapsed(isOpen ? [...collapsed, d.path] : collapsed.filter((p) => p !== d.path))}
                      >
                        <ChevronRight
                          className={cn('size-3.5 shrink-0 text-muted-foreground transition-transform duration-150 ease-out', isOpen && 'rotate-90')}
                        />
                        <code className="truncate text-sm">{d.path}</code>
                        <span className={cn('text-xs', tone[d.status])}>{statusLabel[d.status]}</span>
                      </button>
                      <Button size="xs" variant="ghost" disabled={!!busy} onClick={() => restore(d.path, { path: d.path, before: restoreBefore })}>
                        {busy === d.path ? <Loader2 className="animate-spin" /> : <RotateCcw />}
                        {restoreBefore ? t('Restore deleted file') : t('Restore this version')}
                      </Button>
                    </div>
                    {isOpen &&
                      (d.binary ? (
                        <p className="px-4 py-3 text-sm text-muted-foreground">{t('Binary file.')}</p>
                      ) : (
                        <DiffView before={d.before} after={d.after} />
                      ))}
                  </section>
                )
              })}
            </div>
          </section>
        </div>
      </DialogContent>
    </Dialog>
  )
}
