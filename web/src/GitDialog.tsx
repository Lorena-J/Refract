import { useEffect, useState } from 'react'
import { ArrowDownToLine, ArrowUpFromLine, GitBranch, Loader2, Lock, Plus } from 'lucide-react'
import { toast } from 'sonner'
import { api, errorText } from './api'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { SettingsSwitch } from '@/components/ui/chatcn/settings-panel'
import { openSettings } from '@/lib/prefs'
import { t, tn } from '@/lib/i18n'
import { ago } from '@/lib/time'
import { cn } from '@/lib/utils'

type SyncStatus = { op: 'push' | 'pull'; at: string; auto: boolean; error?: string }
type RemoteInfo = { url: string; branch: string; auto_push: boolean; github: boolean; has_token: boolean; last?: SyncStatus }
type GitHub = { configured: boolean; connected: boolean; login?: string }
type Repo = { full_name: string; clone_url: string; private: boolean; empty: boolean }
export type PullResult = { changed: string[]; conflicts: string[]; empty: boolean }

type Props = {
  projectId: string
  projectName: string
  open: boolean
  onClose: () => void
  beforeSync: () => Promise<void> // save pending edits first
  onPulled: (r: PullResult) => Promise<void>
}

const slug = (s: string) =>
  s
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/[^A-Za-z0-9._-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 100) || 'refract-project'

// A project's git remote: a GitHub repository of the connected account, or any
// https remote with a token. Push and pull by hand, or push after every commit.
export default function GitDialog({ projectId, projectName, open, onClose, beforeSync, onPulled }: Props) {
  const [info, setInfo] = useState<RemoteInfo | null>(null)
  const [gh, setGh] = useState<GitHub | null>(null)
  const [repos, setRepos] = useState<Repo[] | null>(null)
  const [mode, setMode] = useState<'github' | 'url'>('url')
  const [url, setUrl] = useState('')
  const [token, setToken] = useState('')
  const [branch, setBranch] = useState('main')
  const [autoPush, setAutoPush] = useState(false)
  const [newRepo, setNewRepo] = useState('')
  const [busy, setBusy] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    setToken('')
    Promise.all([api<RemoteInfo>(`/api/projects/${projectId}/remote`), api<GitHub>('/api/github')])
      .then(([r, g]) => {
        setInfo(r)
        setGh(g)
        setUrl(r.url)
        setBranch(r.branch || 'main')
        setAutoPush(r.auto_push)
        setMode(g.connected && (r.github || !r.url) ? 'github' : 'url')
        setNewRepo(slug(projectName))
        if (g.connected) api<Repo[]>('/api/github/repos').then(setRepos, (e) => toast.error(errorText(e)))
      })
      .catch((e) => toast.error(errorText(e)))
  }, [open, projectId, projectName])

  async function run<T>(what: string, fn: () => Promise<T>): Promise<T | undefined> {
    setBusy(what)
    try {
      return await fn()
    } catch (e) {
      toast.error(errorText(e))
    } finally {
      setBusy(null)
    }
  }

  const save = (remote: { url: string; github: boolean }) =>
    run('save', async () => {
      const r = await api<RemoteInfo>(`/api/projects/${projectId}/remote`, {
        method: 'PUT',
        body: { url: remote.url, github: remote.github, token: remote.github ? '' : token, branch, auto_push: autoPush },
      })
      setInfo(r)
      setUrl(r.url)
      setToken('')
      toast.success(r.url ? t('Remote saved') : t('Remote removed'))
      return r
    })

  const create = () =>
    run('create', async () => {
      const repo = await api<Repo>('/api/github/repos', { method: 'POST', body: { name: newRepo, private: true } })
      setRepos((rs) => [repo, ...(rs ?? [])])
      setUrl(repo.clone_url)
      toast.success(t('Created {repo} on GitHub', { repo: repo.full_name }))
    })

  const push = () =>
    run('push', async () => {
      await beforeSync()
      const st = await api<SyncStatus>(`/api/projects/${projectId}/remote/push`, { method: 'POST' })
      setInfo((i) => (i ? { ...i, last: st } : i))
      toast.success(t('Pushed to {branch}', { branch }))
    })

  const pull = () =>
    run('pull', async () => {
      await beforeSync()
      const res = await api<PullResult>(`/api/projects/${projectId}/remote/pull`, { method: 'POST' })
      await onPulled(res)
    })

  const saved = !!info?.url
  const dirty =
    !!info && (url !== info.url || branch !== (info.branch || 'main') || autoPush !== info.auto_push || !!token || (mode === 'github') !== info.github)
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="flex max-h-[min(720px,calc(100dvh-2rem))] flex-col gap-0 overflow-hidden rounded-2xl p-0 sm:max-w-xl">
        <DialogTitle className="flex items-center gap-2 px-5 pt-5 text-base">
          <GitBranch className="size-4 text-muted-foreground" /> {t('Git remote')}
        </DialogTitle>
        <DialogDescription className="px-5 pt-1 text-sm">
          {t('Every saved version of the project is a git commit. Keep a copy on GitHub or any git server, and bring in changes made elsewhere.')}
        </DialogDescription>

        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-5 py-4">
          {!info && (
            <p className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" /> {t('Loading…')}
            </p>
          )}
          {info && (
            <>
              <div className="grid grid-cols-2 gap-1 rounded-lg bg-foreground/[0.06] p-0.5 text-xs" role="tablist">
                {(['github', 'url'] as const).map((m) => (
                  <button
                    key={m}
                    role="tab"
                    aria-selected={mode === m}
                    onClick={() => setMode(m)}
                    className={cn(
                      'rounded-md px-3 py-1.5 font-medium transition-colors duration-150',
                      mode === m ? 'bg-background shadow-xs' : 'text-muted-foreground hover:text-foreground',
                    )}
                  >
                    {m === 'github' ? t('GitHub repository') : t('Other remote (URL and token)')}
                  </button>
                ))}
              </div>

              {mode === 'github' && !gh?.connected && (
                <div className="rounded-lg border p-3 text-sm text-muted-foreground">
                  {gh?.configured ? t('Connect your GitHub account first.') : t('GitHub sign-in is not set up on this server (REFRACT_GITHUB_CLIENT_ID).')}{' '}
                  {gh?.configured && (
                    <Button size="xs" variant="secondary" onClick={() => openSettings('integrations')}>
                      {t('Open settings')}
                    </Button>
                  )}
                </div>
              )}

              {mode === 'github' && gh?.connected && (
                <div className="space-y-2">
                  <p className="text-xs text-muted-foreground">{t('Signed in as @{login}. Choose a repository or create one.', { login: gh.login ?? '' })}</p>
                  <div className="max-h-48 overflow-y-auto rounded-lg border">
                    {!repos && (
                      <p className="flex items-center gap-2 p-3 text-sm text-muted-foreground">
                        <Loader2 className="size-4 animate-spin" /> {t('Loading…')}
                      </p>
                    )}
                    {repos?.map((r) => (
                      <button
                        key={r.full_name}
                        onClick={() => setUrl(r.clone_url)}
                        className={cn(
                          'flex w-full items-center gap-2 border-b px-3 py-2 text-left text-sm last:border-b-0 transition-colors duration-100',
                          url === r.clone_url ? 'bg-foreground/[0.07]' : 'hover:bg-foreground/[0.04]',
                        )}
                      >
                        <span className="min-w-0 flex-1 truncate">{r.full_name}</span>
                        {r.empty && <span className="text-xs text-muted-foreground">{t('empty')}</span>}
                        {r.private && <Lock className="size-3.5 text-muted-foreground" aria-label={t('private')} />}
                      </button>
                    ))}
                  </div>
                  <div className="flex gap-2">
                    <input
                      value={newRepo}
                      onChange={(e) => setNewRepo(e.target.value)}
                      aria-label={t('New repository name')}
                      className="h-8 min-w-0 flex-1 rounded-lg border bg-transparent px-3 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                    />
                    <Button size="sm" variant="secondary" disabled={!!busy || !newRepo} onClick={create}>
                      {busy === 'create' ? <Loader2 className="animate-spin" /> : <Plus />} {t('Create private repository')}
                    </Button>
                  </div>
                </div>
              )}

              {mode === 'url' && (
                <div className="space-y-3">
                  <label className="block space-y-1">
                    <span className="text-xs font-medium">{t('Remote URL')}</span>
                    <input
                      value={url}
                      onChange={(e) => setUrl(e.target.value)}
                      placeholder="https://github.com/you/thesis.git"
                      className="h-8 w-full rounded-lg border bg-transparent px-3 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                    />
                  </label>
                  <label className="block space-y-1">
                    <span className="text-xs font-medium">{t('Access token')}</span>
                    <input
                      type="password"
                      autoComplete="off"
                      value={token}
                      onChange={(e) => setToken(e.target.value)}
                      placeholder={
                        info.has_token && !info.github
                          ? t('Saved. Type a new one to replace it.')
                          : t('A personal access token with write access to the repository')
                      }
                      className="h-8 w-full rounded-lg border bg-transparent px-3 text-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                    />
                    <span className="block text-xs text-muted-foreground">{t('Stored encrypted on the server and never shown again.')}</span>
                  </label>
                </div>
              )}

              <div className="flex items-center gap-3">
                <label className="flex items-center gap-2 text-xs font-medium">
                  {t('Branch')}
                  <input
                    value={branch}
                    onChange={(e) => setBranch(e.target.value)}
                    className="h-7 w-32 rounded-md border bg-transparent px-2 font-mono text-xs outline-none focus-visible:border-ring"
                  />
                </label>
                <span className="flex-1" />
                <label className="flex items-center gap-2 text-xs">
                  {t('Push after every saved version')}
                  <SettingsSwitch checked={autoPush} onCheckedChange={setAutoPush} aria-label={t('Push after every saved version')} />
                </label>
              </div>

              {info.last && (
                <p className={cn('text-xs', info.last.error ? 'text-destructive' : 'text-muted-foreground')}>
                  {info.last.op === 'push' ? t('Last push') : t('Last pull')} {info.last.auto && `(${t('automatic')})`} · {ago(info.last.at)}
                  {info.last.error && `: ${info.last.error}`}
                </p>
              )}
            </>
          )}
        </div>

        <div className="flex items-center gap-2 border-t px-5 py-3">
          {saved && (
            <Button size="sm" variant="ghost" className="text-destructive" disabled={!!busy} onClick={() => save({ url: '', github: false })}>
              {t('Remove remote')}
            </Button>
          )}
          <span className="flex-1" />
          <Button size="sm" variant="secondary" disabled={!!busy || !saved || dirty} onClick={pull}>
            {busy === 'pull' ? <Loader2 className="animate-spin" /> : <ArrowDownToLine />} {t('Pull')}
          </Button>
          <Button size="sm" variant="secondary" disabled={!!busy || !saved || dirty} onClick={push}>
            {busy === 'push' ? <Loader2 className="animate-spin" /> : <ArrowUpFromLine />} {t('Push')}
          </Button>
          <Button size="sm" disabled={!!busy || !url || !dirty} onClick={() => save({ url, github: mode === 'github' })}>
            {busy === 'save' && <Loader2 className="animate-spin" />} {t('Save')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}

export const pulledMessage = (r: PullResult) =>
  r.empty
    ? t('The remote branch is empty: push to start it.')
    : r.conflicts.length
      ? t('Not merged: {files} changed in the same places here and on the remote.', { files: r.conflicts.join(', ') })
      : r.changed.length
        ? tn(r.changed.length, 'Pulled {n} changed file', 'Pulled {n} changed files')
        : t('Already up to date.')
