import { useState, type DragEvent } from 'react'
import type { FileEntry } from './api'

type Node = { name: string; path: string; dir: boolean; children: Node[] }

function buildTree(files: FileEntry[]): Node[] {
  const root: Node = { name: '', path: '', dir: true, children: [] }
  const dirs = new Map<string, Node>([['', root]])
  for (const f of files) {
    const slash = f.path.lastIndexOf('/')
    const parent = dirs.get(slash < 0 ? '' : f.path.slice(0, slash)) ?? root
    const node: Node = { name: f.path.slice(slash + 1), path: f.path, dir: f.dir, children: [] }
    parent.children.push(node)
    if (f.dir) dirs.set(f.path, node)
  }
  const sort = (n: Node) => {
    n.children.sort((a, b) => Number(b.dir) - Number(a.dir) || a.name.localeCompare(b.name))
    n.children.forEach(sort)
  }
  sort(root)
  return root.children
}

type Props = {
  files: FileEntry[]
  active?: string
  dirty: (path: string) => boolean
  onOpen: (path: string) => void
  onNew: (dir: string, kind: 'file' | 'folder') => void
  onRename: (path: string) => void
  onDelete: (path: string, dir: boolean) => void
  onUpload: (dir: string, files: File[]) => void
}

export default function FileTree(p: Props) {
  const [open, setOpen] = useState<Set<string>>(new Set())
  const [selectedDir, setSelectedDir] = useState('')
  const toggle = (path: string) => {
    const next = new Set(open)
    if (next.has(path)) next.delete(path)
    else next.add(path)
    setOpen(next)
    setSelectedDir(path)
  }
  const onDrop = (e: DragEvent, dir: string) => {
    e.preventDefault()
    e.stopPropagation()
    if (e.dataTransfer.files.length) p.onUpload(dir, [...e.dataTransfer.files])
  }

  const render = (nodes: Node[], depth: number) =>
    nodes.map((n) => (
      <li key={n.path}>
        <div
          className={'tree-row' + (n.path === p.active ? ' active' : '') + (n.dir && n.path === selectedDir ? ' selected' : '')}
          style={{ paddingLeft: 8 + depth * 14 }}
          onDragOver={(e) => n.dir && e.preventDefault()}
          onDrop={(e) => n.dir && onDrop(e, n.path)}
        >
          <button className="tree-name" onClick={() => (n.dir ? toggle(n.path) : p.onOpen(n.path))} title={n.path}>
            {n.dir ? (open.has(n.path) ? '▾ ' : '▸ ') : ''}
            {n.name}
            {!n.dir && p.dirty(n.path) ? ' •' : ''}
          </button>
          <span className="tree-actions">
            <button onClick={() => p.onRename(n.path)} aria-label={`Rename or move ${n.path}`} title="Rename / move">
              ✎
            </button>
            <button onClick={() => p.onDelete(n.path, n.dir)} aria-label={`Delete ${n.path}`} title="Delete">
              ×
            </button>
          </span>
        </div>
        {n.dir && open.has(n.path) && <ul>{render(n.children, depth + 1)}</ul>}
      </li>
    ))

  return (
    <div className="tree" onDragOver={(e) => e.preventDefault()} onDrop={(e) => onDrop(e, selectedDir)}>
      <div className="tree-toolbar">
        <button onClick={() => p.onNew(selectedDir, 'file')} title="New file">+ File</button>
        <button onClick={() => p.onNew(selectedDir, 'folder')} title="New folder">+ Folder</button>
        <label className="file" title="Upload files">
          Upload
          <input type="file" multiple onChange={(e) => p.onUpload(selectedDir, [...(e.target.files ?? [])])} />
        </label>
      </div>
      <div className="hint tree-target">
        in /{selectedDir}
        {selectedDir && (
          <button className="link" onClick={() => setSelectedDir('')}>
            (root)
          </button>
        )}
      </div>
      <ul>{render(buildTree(p.files), 0)}</ul>
    </div>
  )
}
