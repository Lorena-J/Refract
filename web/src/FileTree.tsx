import { useState, type DragEvent } from 'react'
import { ChevronRight, File, FileImage, FileText, Folder, FolderOpen, FolderPlus, MoreHorizontal, Pencil, Plus, Trash2, Upload } from 'lucide-react'
import type { FileEntry } from './api'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

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

const fileIcon = (name: string) => {
  const ext = name.slice(name.lastIndexOf('.') + 1).toLowerCase()
  if (['tex', 'bib', 'sty', 'cls', 'txt', 'md'].includes(ext)) return FileText
  if (['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'pdf', 'eps'].includes(ext)) return FileImage
  return File
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
  const [dropTarget, setDropTarget] = useState<string | null>(null)
  const toggle = (path: string) => {
    const next = new Set(open)
    if (next.has(path)) next.delete(path)
    else next.add(path)
    setOpen(next)
  }
  const drop = (e: DragEvent, dir: string) => {
    e.preventDefault()
    e.stopPropagation()
    setDropTarget(null)
    if (e.dataTransfer.files.length) p.onUpload(dir, [...e.dataTransfer.files])
  }
  const over = (e: DragEvent, dir: string) => {
    if (!e.dataTransfer.types.includes('Files')) return
    e.preventDefault()
    e.stopPropagation()
    setDropTarget(dir)
  }

  const render = (nodes: Node[], depth: number) =>
    nodes.map((n) => {
      const Icon = n.dir ? (open.has(n.path) ? FolderOpen : Folder) : fileIcon(n.name)
      const isOpen = open.has(n.path)
      return (
        <li key={n.path}>
          <div
            className={cn(
              'group/row relative flex h-8 items-center rounded-md pr-1 text-sm transition-colors duration-100',
              n.path === p.active ? 'bg-foreground/[0.07] text-foreground' : 'text-foreground/80 hover:bg-foreground/[0.04] hover:text-foreground',
              dropTarget === n.path && 'ring-1 ring-ring',
            )}
            style={{ paddingLeft: 6 + depth * 14 }}
            onDragOver={(e) => n.dir && over(e, n.path)}
            onDragLeave={() => setDropTarget(null)}
            onDrop={(e) => n.dir && drop(e, n.path)}
          >
            <button
              className="flex min-w-0 flex-1 items-center gap-1.5 self-stretch text-left outline-none focus-visible:underline"
              onClick={() => (n.dir ? toggle(n.path) : p.onOpen(n.path))}
              title={n.path}
            >
              {n.dir ? (
                <ChevronRight className={cn('size-3.5 shrink-0 text-muted-foreground transition-transform duration-150 ease-out', isOpen && 'rotate-90')} />
              ) : (
                <span className="w-3.5 shrink-0" />
              )}
              <Icon className="size-4 shrink-0 text-muted-foreground" />
              <span className="truncate">{n.name}</span>
              {!n.dir && p.dirty(n.path) && <span className="ml-1 size-1.5 shrink-0 rounded-full bg-warning" aria-label="unsaved changes" />}
            </button>
            <span className="flex items-center opacity-0 transition-opacity duration-100 group-hover/row:opacity-100 group-focus-within/row:opacity-100 has-[[data-state=open]]:opacity-100">
              {n.dir && (
                <Button variant="ghost" size="icon-xs" aria-label={`New file in ${n.path}`} onClick={() => p.onNew(n.path, 'file')}>
                  <Plus />
                </Button>
              )}
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon-xs" aria-label={`Actions for ${n.path}`}>
                    <MoreHorizontal />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" className="w-48">
                  {n.dir && (
                    <>
                      <DropdownMenuItem onSelect={() => p.onNew(n.path, 'file')}>
                        <Plus /> New file
                      </DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => p.onNew(n.path, 'folder')}>
                        <FolderPlus /> New folder
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                    </>
                  )}
                  <DropdownMenuItem onSelect={() => p.onRename(n.path)}>
                    <Pencil /> Rename or move
                  </DropdownMenuItem>
                  <DropdownMenuItem variant="destructive" onSelect={() => p.onDelete(n.path, n.dir)}>
                    <Trash2 /> Delete
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </span>
          </div>
          {n.dir && isOpen && <ul>{render(n.children, depth + 1)}</ul>}
        </li>
      )
    })

  return (
    <div
      className={cn('min-h-0 flex-1 overflow-auto px-2 pb-2', dropTarget === '' && 'rounded-md ring-1 ring-ring ring-inset')}
      onDragOver={(e) => over(e, '')}
      onDragLeave={(e) => e.currentTarget === e.target && setDropTarget(null)}
      onDrop={(e) => drop(e, '')}
    >
      <ul>{render(buildTree(p.files), 0)}</ul>
      {p.files.length === 0 && (
        <p className="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground">
          <Upload className="size-3.5" /> Drop files here or create one.
        </p>
      )}
    </div>
  )
}
