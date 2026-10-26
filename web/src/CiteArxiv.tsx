import { useState } from 'react'
import { BookMarked, ExternalLink, Loader2, Quote, Search } from 'lucide-react'
import { toast } from 'sonner'
import { api, errorText } from './api'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { t } from '@/lib/i18n'

type Paper = { id: string; version: string; title: string; authors: string[]; summary: string; published: string; category: string }
export type Citation = { key: string; path: string; added: boolean; needs_setup: boolean }

type Props = {
  projectId: string
  open: boolean
  onClose: () => void
  onCited: (c: Citation) => Promise<void>
}

const authorsOf = (a: string[]) => (a.length > 3 ? `${a.slice(0, 3).join(', ')} et al.` : a.join(', '))

// Search arXiv and cite a paper: the BibTeX entry is built on the server from
// arXiv's metadata (never written by a model) and appended to the project's .bib.
export default function CiteArxiv({ projectId, open, onClose, onCited }: Props) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<Paper[] | null>(null)
  const [searching, setSearching] = useState(false)
  const [citing, setCiting] = useState('')

  async function search(e: React.FormEvent) {
    e.preventDefault()
    if (!query.trim()) return
    setSearching(true)
    try {
      setResults(await api<Paper[]>(`/api/arxiv/search?q=${encodeURIComponent(query.trim())}`))
    } catch (err) {
      toast.error(errorText(err))
    } finally {
      setSearching(false)
    }
  }

  async function cite(p: Paper) {
    setCiting(p.id)
    try {
      const c = await api<Citation>(`/api/projects/${projectId}/cite`, { method: 'POST', body: { id: p.id } })
      await onCited(c)
      onClose()
    } catch (err) {
      toast.error(errorText(err))
    } finally {
      setCiting('')
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="flex h-[min(640px,calc(100dvh-2rem))] flex-col gap-0 overflow-hidden rounded-2xl p-0 sm:max-w-2xl">
        <DialogTitle className="flex items-center gap-2 px-5 pt-5 text-base">
          <BookMarked className="size-4 text-muted-foreground" /> {t('Cite from arXiv')}
        </DialogTitle>
        <DialogDescription className="px-5 pt-1 text-sm">
          {t('The entry is added to your .bib from arXiv’s own metadata, and \\cite is inserted at the cursor.')}
        </DialogDescription>
        <form onSubmit={search} className="flex gap-2 px-5 pt-4 pb-3">
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
            <input
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t('Title words, au:surname, or an arXiv id like 1706.03762')}
              aria-label={t('Search arXiv')}
              className="h-9 w-full rounded-lg border bg-transparent pr-3 pl-9 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
            />
          </div>
          <Button type="submit" disabled={searching || !query.trim()}>
            {searching && <Loader2 className="animate-spin" />} {t('Search')}
          </Button>
        </form>
        <ol className="min-h-0 flex-1 overflow-y-auto border-t px-2 py-2">
          {results?.length === 0 && <li className="p-4 text-center text-sm text-muted-foreground">{t('No papers found.')}</li>}
          {!results && !searching && <li className="p-4 text-center text-sm text-muted-foreground">{t('Search by title, author or id.')}</li>}
          {results?.map((p) => (
            <li key={p.id} className="enter group flex gap-3 rounded-lg p-3 hover:bg-foreground/[0.03]">
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">{p.title}</p>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {authorsOf(p.authors)} · {new Date(p.published).getFullYear()} ·{' '}
                  <a className="inline-flex items-center gap-0.5 hover:text-foreground" href={`https://arxiv.org/abs/${p.id}`} target="_blank" rel="noreferrer">
                    arXiv:{p.id} <ExternalLink className="size-3" />
                  </a>{' '}
                  · {p.category}
                </p>
                <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{p.summary}</p>
              </div>
              <Button size="sm" variant="secondary" className="shrink-0 self-start" disabled={!!citing} onClick={() => cite(p)}>
                {citing === p.id ? <Loader2 className="animate-spin" /> : <Quote />} {t('Cite')}
              </Button>
            </li>
          ))}
        </ol>
      </DialogContent>
    </Dialog>
  )
}
