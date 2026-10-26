import { useCallback, useEffect, useRef, useState } from 'react'
import type { EditorState } from '@codemirror/state'
import { api, errorText, mkdir, move, readFile, removeFile, writeFile, type FileEntry, type OpenedFile, type Project } from './api'
import { Autosaver, type SaveStatus } from './autosave'
import Editor from './Editor'
import FileTree from './FileTree'

const under = (p: string, prefix: string) => p === prefix || p.startsWith(prefix + '/')
const moved = (p: string, from: string, to: string) => (under(p, from) ? to + p.slice(from.length) : p)

const statusText: Record<SaveStatus, string> = {
  saved: 'All changes saved',
  dirty: 'Unsaved changes',
  saving: 'Saving…',
  error: 'Save failed, retrying…',
}

export default function Workspace({ id }: { id: string }) {
  const [project, setProject] = useState<Project | null>(null)
  const [files, setFiles] = useState<FileEntry[]>([])
  const [tabs, setTabs] = useState<string[]>([])
  const [active, setActive] = useState<string | undefined>()
  const [statuses, setStatuses] = useState<Record<string, SaveStatus>>({})
  const [error, setError] = useState('')

  // Outside React state: content loaded from the server and live editor states per open file.
  const loaded = useRef(new Map<string, OpenedFile>())
  const states = useRef(new Map<string, EditorState>())
  const saver = useRef<Autosaver | null>(null)
  if (!saver.current) {
    saver.current = new Autosaver((p, c) => writeFile(id, p, c), {
      onStatus: (p, s) => setStatuses((prev) => ({ ...prev, [p]: s })),
    })
  }
  const autosave = saver.current

  const loadFiles = useCallback(() => api<FileEntry[]>(`/api/projects/${id}/files`).then(setFiles), [id])

  useEffect(() => {
    api<Project>(`/api/projects/${id}`)
      .then(setProject)
      .catch((e) => setError(errorText(e)))
    loadFiles().catch((e) => setError(errorText(e)))
  }, [id, loadFiles])

  // Never lose edits on reload or close: flush and ask the browser to confirm.
  useEffect(() => {
    const onUnload = (e: BeforeUnloadEvent) => {
      if (!autosave.isDirty()) return
      autosave.flushAll()
      e.preventDefault()
    }
    window.addEventListener('beforeunload', onUnload)
    return () => window.removeEventListener('beforeunload', onUnload)
  }, [autosave])

  async function run(fn: () => Promise<unknown>) {
    setError('')
    try {
      await fn()
    } catch (e) {
      setError(errorText(e))
    }
  }

  async function open(path: string) {
    if (!loaded.current.has(path)) {
      try {
        loaded.current.set(path, await readFile(id, path))
      } catch (e) {
        setError(errorText(e))
        return
      }
    }
    setTabs((t) => (t.includes(path) ? t : [...t, path]))
    setActive(path)
  }

  // Closing never cancels a pending save: the autosaver finishes it on its own.
  function forget(paths: string[]) {
    for (const p of paths) {
      loaded.current.delete(p)
      states.current.delete(p)
    }
    const next = tabs.filter((p) => !paths.includes(p))
    setTabs(next)
    if (active && paths.includes(active)) setActive(next[Math.min(tabs.indexOf(active), next.length - 1)])
  }

  function close(path: string) {
    autosave.flush(path)
    forget([path])
  }

  async function create(dir: string, kind: 'file' | 'folder') {
    const path = prompt(kind === 'file' ? 'New file path' : 'New folder path', dir ? dir + '/' : '')?.trim()
    if (!path || path.endsWith('/')) return
    await run(async () => {
      if (files.some((f) => f.path === path)) throw new Error(`${path} already exists`)
      if (kind === 'file') await writeFile(id, path, '')
      else await mkdir(id, path)
      await loadFiles()
      if (kind === 'file') await open(path)
    })
  }

  async function rename(from: string) {
    const to = prompt('Rename or move to', from)?.trim()
    if (!to || to === from) return
    await run(async () => {
      const affected = tabs.filter((p) => under(p, from))
      await Promise.all(affected.map((p) => autosave.flush(p))) // save under the old name first
      await move(id, from, to)
      for (const p of affected) {
        const q = moved(p, from, to)
        autosave.rename(p, q)
        for (const m of [loaded.current, states.current] as Map<string, unknown>[]) {
          if (m.has(p)) {
            m.set(q, m.get(p))
            m.delete(p)
          }
        }
      }
      setTabs((t) => t.map((p) => moved(p, from, to)))
      setActive((a) => (a ? moved(a, from, to) : a))
      await loadFiles()
    })
  }

  async function remove(path: string, dir: boolean) {
    if (!confirm(dir ? `Delete the folder ${path} and everything in it?` : `Delete ${path}?`)) return
    await run(async () => {
      const affected = tabs.filter((p) => under(p, path))
      affected.forEach((p) => autosave.drop(p))
      await removeFile(id, path)
      forget(affected)
      await loadFiles()
    })
  }

  async function upload(dir: string, list: File[]) {
    await run(async () => {
      for (const f of list) {
        const path = dir ? `${dir}/${f.name}` : f.name
        if (files.some((e) => e.path === path) && !confirm(`Replace ${path}?`)) continue
        autosave.drop(path)
        forget([path])
        await writeFile(id, path, f)
      }
      await loadFiles()
    })
  }

  const current = active ? loaded.current.get(active) : undefined

  return (
    <div className="layout">
      <aside className="pane files">
        <div className="pane-title">
          <a href="/">Projects</a> / <strong>{project?.name ?? '…'}</strong>
        </div>
        <FileTree
          files={files}
          active={active}
          dirty={(p) => autosave.isDirty(p)}
          onOpen={open}
          onNew={create}
          onRename={rename}
          onDelete={remove}
          onUpload={upload}
        />
      </aside>
      <main className="pane editor">
        <div className="tabs" role="tablist">
          {tabs.map((p) => (
            <div key={p} className={'tab' + (p === active ? ' active' : '')} role="tab" aria-selected={p === active}>
              <button className="tab-name" onClick={() => setActive(p)} onAuxClick={(e) => e.button === 1 && close(p)} title={p}>
                {p.split('/').pop()}
                {statuses[p] && statuses[p] !== 'saved' ? ' •' : ''}
              </button>
              <button className="tab-close" onClick={() => close(p)} aria-label={`Close ${p}`}>
                ×
              </button>
            </div>
          ))}
        </div>
        {error && <p className="error">{error}</p>}
        <div className="editor-body">
          {active && current?.text && (
            <Editor
              path={active}
              states={states.current}
              initial={(p) => {
                const f = loaded.current.get(p)
                return f?.text ? f.content : ''
              }}
              onChange={(p, c) => autosave.change(p, c)}
              onSave={() => active && autosave.flush(active)}
            />
          )}
          {active && current && !current.text && <BinaryView file={current} path={active} />}
          {!active && <p className="hint empty">Open a file from the tree.</p>}
        </div>
        {active && current?.text && <div className="status hint">{statusText[statuses[active] ?? 'saved']}</div>}
      </main>
      <section className="pane preview">PDF</section>
    </div>
  )
}

function BinaryView({ file, path }: { file: Extract<OpenedFile, { text: false }>; path: string }) {
  const [url, setUrl] = useState('')
  useEffect(() => {
    const u = URL.createObjectURL(file.blob)
    setUrl(u)
    return () => URL.revokeObjectURL(u)
  }, [file])
  if (file.type.startsWith('image/')) {
    return <div className="binary">{url && <img src={url} alt={path} />}</div>
  }
  return (
    <div className="binary hint">
      <p>{path}</p>
      <p>
        {file.type || 'binary file'} · {(file.size / 1024).toFixed(1)} KB · read-only
      </p>
    </div>
  )
}
