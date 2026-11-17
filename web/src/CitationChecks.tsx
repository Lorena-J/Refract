import { useEffect, useEffectEvent, useRef, useState } from 'react'
import { BookMarked, Loader2 } from 'lucide-react'
import { api, errorText } from './api'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { t } from '@/lib/i18n'

type Finding = { file: string; line: number; key?: string; kind: string; fields?: string[]; identifier?: string }
type Entry = { file: string; line: number; key: string; doi?: string; identifier?: string }
type Report = { findings: Finding[]; entries: Entry[] }
export type CitationFix = { path: string; before: string; after: string }

function description(f: Finding) {
  switch (f.kind) {
    case 'missing':
      return t('Citation has no bibliography entry.')
    case 'duplicate-key':
      return t('Citation key is defined more than once.')
    case 'duplicate-work':
      return t('Another entry uses the same DOI or arXiv identifier.')
    case 'unused':
      return t('Entry is not cited in the project.')
    case 'incomplete':
      return t('Missing bibliography fields: {fields}', { fields: f.fields?.join(', ') ?? '' })
    case 'invalid-doi':
      return t('DOI format is invalid.')
    case 'unresolved-doi':
      return t('DOI does not resolve.')
    case 'doi-unavailable':
      return t('DOI could not be checked. The service may be unavailable; try again later.')
    case 'main-not-set':
      return t('Select the main file before checking citations.')
    case 'unreadable-source':
      return t('A document source or bibliography could not be read. The citation check is incomplete.')
    default:
      return t('File is too large to check (over 8 MB).')
  }
}

export default function CitationChecks({
  projectId,
  onClose,
  beforeCheck,
  onLocate,
  onProposal,
}: {
  projectId: string
  onClose: () => void
  beforeCheck: () => Promise<void>
  onLocate: (file: string, line: number) => void
  onProposal: (fix: CitationFix) => Promise<void>
}) {
  const [report, setReport] = useState<Report | null>(null)
  const [busy, setBusy] = useState(true)
  const [error, setError] = useState('')
  const [progress, setProgress] = useState('')
  const [doiCheck, setDoiCheck] = useState<{ total: number; resolved: number; checked: number } | null>(null)
  const doiResults = useRef(new Map<string, string>())
  const request = useRef<AbortController | null>(null)
  const focusEditor = useRef(false)

  async function check(remote: boolean) {
    request.current?.abort()
    const controller = new AbortController()
    request.current = controller
    setBusy(true)
    setError('')
    setReport(null)
    setDoiCheck(null)
    try {
      await beforeCheck()
      const data = await api<Report>(`/api/projects/${projectId}/citation-checks`, { signal: controller.signal })
      const dois = [...new Set(data.entries.flatMap((e) => (e.doi ? [e.doi] : [])))]
      const results = new Map(doiResults.current)
      if (remote) {
        for (const [i, doi] of dois.entries()) {
          if (controller.signal.aborted) return
          setProgress(t('Checking DOI {n} of {total}…', { n: i + 1, total: dois.length }))
          const { status } = await api<{ status: string }>(`/api/projects/${projectId}/citation-checks/doi`, {
            method: 'POST',
            body: { doi },
            signal: controller.signal,
          })
          results.set(doi, status)
        }
      }
      if (!controller.signal.aborted) {
        // Rebuild locations from the latest bibliography, keeping online
        // results only for the exact DOI identifiers already checked.
        for (const entry of data.entries) {
          const status = entry.doi && results.get(entry.doi)
          if (status && status !== 'resolved') data.findings.push({ ...entry, kind: status })
        }
        doiResults.current = results
        const checked = dois.filter((doi) => results.has(doi)).length
        setReport(data)
        setDoiCheck(
          checked > 0 || dois.length === 0
            ? { total: dois.length, checked, resolved: dois.filter((doi) => results.get(doi) === 'resolved').length }
            : null,
        )
      }
    } catch (e) {
      if (!controller.signal.aborted) setError(errorText(e))
    } finally {
      if (!controller.signal.aborted) {
        setBusy(false)
        setProgress('')
      }
    }
  }

  const initialCheck = useEffectEvent(() => {
    void check(false)
  })
  useEffect(() => {
    doiResults.current.clear()
    const lifecycle = new AbortController()
    queueMicrotask(() => {
      if (!lifecycle.signal.aborted) initialCheck()
    })
    return () => {
      lifecycle.abort()
      request.current?.abort()
    }
  }, [projectId])

  async function fix(entry: Entry) {
    const controller = new AbortController()
    request.current = controller
    setBusy(true)
    setError('')
    try {
      await beforeCheck()
      const proposal = await api<CitationFix>(`/api/projects/${projectId}/citation-checks/fix`, {
        method: 'POST',
        body: { file: entry.file, key: entry.key },
        signal: controller.signal,
      })
      if (controller.signal.aborted) return
      await onProposal(proposal)
      focusEditor.current = true
      onClose()
    } catch (e) {
      if (!controller.signal.aborted) setError(errorText(e))
    } finally {
      if (!controller.signal.aborted) setBusy(false)
    }
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent
        className="flex max-h-[85dvh] flex-col sm:max-w-2xl"
        finalFocus={() => (focusEditor.current ? document.querySelector<HTMLElement>('.cm-content') : true)}
      >
        <DialogTitle className="flex items-center gap-2">
          <BookMarked className="size-4" />
          {t('Citation checks')}
        </DialogTitle>
        <DialogDescription>
          {t(
            'Check the main document, its included files and loaded bibliographies. Metadata corrections come from doi.org or arXiv and wait for your review in the editor.',
          )}
        </DialogDescription>
        <div className="flex gap-2">
          <Button variant="secondary" disabled={busy} onClick={() => check(false)}>
            {t('Check again')}
          </Button>
          <Button variant="secondary" disabled={busy} onClick={() => check(true)}>
            {t('Check DOIs online')}
          </Button>
        </div>
        {busy && (
          <p role="status" className="flex items-center gap-2 text-sm">
            <Loader2 className="size-4 animate-spin" />
            {progress || t('Checking bibliography…')}
          </p>
        )}
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {report && (
          <div className="min-h-0 overflow-y-auto">
            <p role="status" className="mb-3 text-xs text-muted-foreground">
              {doiCheck === null
                ? t('DOIs have not been checked online.')
                : doiCheck.total === 0
                  ? t('No DOI identifiers found in the loaded bibliographies.')
                  : doiCheck.checked < doiCheck.total
                    ? t('DOIs checked online: {checked} of {total}. Check online to verify the rest.', doiCheck)
                    : t('DOI check complete: {resolved} of {total} returned metadata.', doiCheck)}
            </p>
            {report.findings.length === 0 && <p className="py-3 text-sm">{t('No citation problems found.')}</p>}
            <ul className="divide-y">
              {report.findings.map((f, i) => (
                <li key={i} className="py-3 text-sm">
                  {f.file && (
                    <button
                      className="text-primary underline underline-offset-2"
                      disabled={busy}
                      onClick={() => {
                        focusEditor.current = true
                        onLocate(f.file, f.line)
                      }}
                    >
                      {f.file}:{f.line}
                    </button>
                  )}
                  {f.key && <code className="ml-2">{f.key}</code>}
                  <p className="mt-1">{description(f)}</p>
                  {f.kind === 'incomplete' && f.identifier && (
                    <Button
                      className="mt-2"
                      size="sm"
                      variant="outline"
                      disabled={busy}
                      onClick={() => fix({ file: f.file, line: f.line, key: f.key! })}
                    >
                      {t('Review source metadata')}
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
