import { useEffect, useState, type DragEvent } from 'react'
import { api, download, errorText, type ImportResult, type Project } from './api'

type Report = { name: string; result?: ImportResult; error?: string }

export default function Projects() {
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [reports, setReports] = useState<Report[]>([])
  const [newName, setNewName] = useState('')
  const [renaming, setRenaming] = useState<{ id: string; name: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const load = () =>
    api<Project[]>('/api/projects')
      .then(setProjects)
      .catch((e) => setError(errorText(e)))
  useEffect(() => {
    load()
  }, [])

  async function act(fn: () => Promise<unknown>) {
    setError('')
    try {
      await fn()
      await load()
    } catch (e) {
      setError(errorText(e))
    }
  }

  // Imports run one at a time so a bulk drop doesn't hold many uploads open.
  async function importZips(files: File[]) {
    const zips = files.filter((f) => f.name.toLowerCase().endsWith('.zip'))
    if (zips.length === 0) return
    setBusy(true)
    const out: Report[] = []
    for (const f of zips) {
      const name = f.name.replace(/\.zip$/i, '')
      try {
        out.push({ name, result: await api<ImportResult>(`/api/projects/import?name=${encodeURIComponent(name)}`, { method: 'POST', body: f }) })
      } catch (e) {
        out.push({ name, error: errorText(e) })
      }
    }
    setReports(out)
    setBusy(false)
    load()
  }

  async function importFolder(files: File[]) {
    if (files.length === 0) return
    const name = files[0].webkitRelativePath.split('/')[0]
    const form = new FormData()
    for (const f of files) form.append('file', f, f.webkitRelativePath)
    setBusy(true)
    try {
      const result = await api<ImportResult>(`/api/projects/import-folder?name=${encodeURIComponent(name)}`, { method: 'POST', body: form })
      setReports([{ name, result }])
    } catch (e) {
      setReports([{ name, error: errorText(e) }])
    }
    setBusy(false)
    load()
  }

  function onDrop(e: DragEvent) {
    e.preventDefault()
    importZips([...e.dataTransfer.files])
  }

  return (
    <div className="page" onDragOver={(e) => e.preventDefault()} onDrop={onDrop}>
      <header className="page-header">
        <h1>Projects</h1>
        <a href="/settings">Settings</a>
      </header>

      <div className="toolbar">
        <form
          onSubmit={(e) => {
            e.preventDefault()
            act(() => api('/api/projects', { method: 'POST', body: { name: newName } })).then(() => setNewName(''))
          }}
        >
          <input value={newName} onChange={(e) => setNewName(e.target.value)} placeholder="New project name" aria-label="New project name" />
          <button disabled={busy}>Create</button>
        </form>
        <label className="file">
          Import .zip…
          <input type="file" accept=".zip" multiple disabled={busy} onChange={(e) => importZips([...(e.target.files ?? [])])} />
        </label>
        <label className="file">
          Import folder…
          {/* @ts-expect-error webkitdirectory is non-standard but supported by all desktop browsers */}
          <input type="file" webkitdirectory="" disabled={busy} onChange={(e) => importFolder([...(e.target.files ?? [])])} />
        </label>
      </div>
      <p className="hint">{busy ? 'Importing…' : 'Or drop one or more .zip files anywhere on this page.'}</p>
      {error && <p className="error">{error}</p>}

      {reports.map((r) => (
        <ImportReport key={r.name + (r.result?.project.id ?? '')} report={r} onChange={load} />
      ))}

      {projects?.length === 0 && <p className="hint">No projects yet.</p>}
      <ul className="project-list">
        {projects?.map((p) => (
          <li key={p.id}>
            {renaming?.id === p.id ? (
              <form
                className="grow"
                onSubmit={(e) => {
                  e.preventDefault()
                  act(() => api(`/api/projects/${p.id}`, { method: 'PATCH', body: { name: renaming.name } })).then(() => setRenaming(null))
                }}
              >
                <input autoFocus value={renaming.name} onChange={(e) => setRenaming({ id: p.id, name: e.target.value })} aria-label="Project name" />
                <button>Save</button>
                <button type="button" onClick={() => setRenaming(null)}>
                  Cancel
                </button>
              </form>
            ) : (
              <div className="grow">
                <a href={`/p/${p.id}`}>{p.name}</a>
                <span className="hint"> · {p.main_file || 'main file not set'} · {new Date(p.updated_at).toLocaleString()}</span>
              </div>
            )}
            <button onClick={() => setRenaming({ id: p.id, name: p.name })}>Rename</button>
            <button onClick={() => act(() => download(`/api/projects/${p.id}/export`, `${p.name}.zip`))}>Export</button>
            <button
              onClick={() => {
                if (confirm(`Delete "${p.name}" and its whole history? This cannot be undone.`)) act(() => api(`/api/projects/${p.id}`, { method: 'DELETE' }))
              }}
            >
              Delete
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}

function ImportReport({ report, onChange }: { report: Report; onChange: () => void }) {
  const [main, setMain] = useState('')
  const [error, setError] = useState('')
  const r = report.result
  if (report.error) return <p className="error">{report.name}: {report.error}</p>
  if (!r) return null
  return (
    <section className="report">
      <strong>{r.project.name}</strong> imported.
      {r.candidates && !main && (
        <p>
          Main file not found. Choose it:{' '}
          <select
            defaultValue=""
            onChange={(e) =>
              api(`/api/projects/${r.project.id}`, { method: 'PATCH', body: { main_file: e.target.value } })
                .then(() => {
                  setMain(e.target.value)
                  onChange()
                })
                .catch((err) => setError(errorText(err)))
            }
            aria-label="Main file"
          >
            <option value="" disabled>
              Select…
            </option>
            {r.candidates.map((c) => (
              <option key={c}>{c}</option>
            ))}
          </select>
        </p>
      )}
      {error && <p className="error">{error}</p>}
      {r.skipped && r.skipped.length > 0 && <p className="hint">Skipped: {r.skipped.join(', ')}</p>}
      {r.warnings.length > 0 && (
        <ul className="warnings">
          {r.warnings.map((w, i) => (
            <li key={i}>
              <code>
                {w.file}:{w.line}
              </code>{' '}
              {w.message}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
