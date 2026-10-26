import { useEffect, useReducer, useRef, useState } from 'react'
import { basicSetup } from 'codemirror'
import { Compartment, EditorSelection, EditorState, Prec } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { latex } from '@/lib/editor'
import { getPrefs, usePrefs } from '@/lib/prefs'
import { accept, proposalField, reject } from '@/lib/proposal'
import QuickActions, { actions, type QuickHandle } from './QuickActions'
import { useCommands } from '@/lib/commands'
import { ClipboardPaste, Copy, FileSearch, MessageSquareText, Pencil, Play, Scissors, TextSelect } from 'lucide-react'
import { WithContextMenu, type MenuEntry } from '@/components/Menu'
import { mod } from '@/lib/shortcuts'
import { t } from '@/lib/i18n'

const wrapping = new Compartment()
const wrapExt = (on: boolean) => (on ? EditorView.lineWrapping : [])

type Props = {
  path: string
  // Owned by the parent (it removes entries for closed/deleted files and moves them on rename).
  // Kept outside React so switching tabs keeps undo history and selection.
  states: Map<string, EditorState>
  initial: (path: string) => string
  onChange: (path: string, content: string) => void
  onSave: () => void
  onCompile: () => void
  onSync: (path: string, line: number, col: number) => void // forward sync (Mod-J)
  onSelect?: (lines: string) => void // selected lines ("3" or "3–9"), "" when nothing is selected
  jump?: { path: string; line: number; n: number } // n changes on every request
  insert?: { path: string; text: string } // replace the selection with text, then onInserted
  onInserted?: () => void
  quick?: boolean // show the quick-actions toolbar on selections (AI available)
}

// One EditorView; each open file keeps its own EditorState, swapped on tab switch.
export default function Editor({ path, states, initial, onChange, onSave, onCompile, onSync, onSelect, jump, insert, onInserted, quick }: Props) {
  const host = useRef<HTMLDivElement>(null)
  const view = useRef<EditorView | null>(null)
  const current = useRef(path)
  const cb = useRef({ onChange, onSave, onCompile, onSync, onSelect })
  cb.current = { onChange, onSave, onCompile, onSync, onSelect }

  const [mounted, setMounted] = useState<EditorView | null>(null)
  const [dragging, setDragging] = useState(false) // no toolbar while a mouse selection is being made
  const [, bump] = useReducer((n: number) => n + 1, 0) // re-render the toolbar (it reads the view)
  const quickRef = useRef<QuickHandle>(null)

  // ⌘K with text selected: the quick actions, and the query as a free instruction.
  useCommands((query) => {
    const v = view.current
    if (!quick || !v || v.state.selection.main.empty) return []
    const q = query.trim()
    return [
      ...(q
        ? [
            {
              id: 'sel:ask',
              group: 'Selection',
              label: t('Ask: “{query}”', { query: q }),
              hint: t('edit the selection'),
              icon: MessageSquareText,
              pinned: true,
              run: () => quickRef.current?.run('custom', q),
            },
          ]
        : []),
      ...actions.map((a) => ({
        id: `sel:${a.id}`,
        group: 'Selection',
        label: a.command,
        hint: t('selection'),
        icon: a.icon,
        run: () => quickRef.current?.run(a.id),
      })),
    ]
  })

  const { wrap } = usePrefs()
  const extensions = useRef([
    basicSetup,
    latex,
    proposalField,
    wrapping.of(wrapExt(getPrefs().wrap)),
    // Highest precedence: basicSetup binds Mod-Enter to "insert blank line".
    Prec.highest(
      keymap.of([
        { key: 'Tab', run: accept },
        { key: 'Escape', run: (v) => reject(v) || (quickRef.current?.cancel() ?? false) },
        {
          key: 'Mod-s',
          run: () => {
            cb.current.onSave()
            return true
          },
        },
        {
          key: 'Mod-Enter',
          run: () => {
            cb.current.onCompile()
            return true
          },
        },
        {
          key: 'Mod-j',
          run: (v) => {
            const head = v.state.selection.main.head
            const line = v.state.doc.lineAt(head)
            cb.current.onSync(current.current, line.number, head - line.from + 1)
            return true
          },
        },
      ]),
    ),
    EditorView.updateListener.of((u) => {
      // Keep the map live, so tab switches and renames always see the latest state.
      if (states.has(current.current)) states.set(current.current, u.state)
      if (u.docChanged) cb.current.onChange(current.current, u.state.doc.toString())
      if (u.selectionSet || u.focusChanged || u.geometryChanged || u.viewportChanged || u.startState.field(proposalField) !== u.state.field(proposalField))
        bump()
      if (u.selectionSet || u.docChanged) {
        const { from, to, empty } = u.state.selection.main
        const a = u.state.doc.lineAt(from).number
        const b = u.state.doc.lineAt(to).number
        cb.current.onSelect?.(empty ? '' : a === b ? `${a}` : `${a}–${b}`)
      }
    }),
  ]).current

  const stateFor = (p: string) => {
    let s = states.get(p)
    if (!s) {
      s = EditorState.create({ doc: initial(p), extensions })
      states.set(p, s)
    }
    return s
  }

  useEffect(() => {
    const v = new EditorView({ parent: host.current!, state: stateFor(path) })
    view.current = v
    setMounted(v)
    let frame = 0
    const onScroll = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(bump)
    }
    const down = () => setDragging(true)
    const up = () => setDragging(false)
    v.scrollDOM.addEventListener('scroll', onScroll)
    v.contentDOM.addEventListener('pointerdown', down)
    window.addEventListener('pointerup', up)
    return () => {
      window.removeEventListener('pointerup', up)
      cancelAnimationFrame(frame)
      v.destroy()
    }
  }, [])

  useEffect(() => {
    const v = view.current!
    const next = stateFor(path)
    current.current = path
    if (v.state !== next) v.setState(next)
    v.focus()
  }, [path])

  // Stored states keep the setting they were created with: re-apply it to whichever is shown.
  useEffect(() => {
    view.current!.dispatch({ effects: wrapping.reconfigure(wrapExt(wrap)) })
  }, [wrap, path])

  useEffect(() => {
    const v = view.current!
    if (!jump || jump.path !== path) return
    const line = v.state.doc.line(Math.min(Math.max(jump.line, 1), v.state.doc.lines))
    v.dispatch({ selection: EditorSelection.cursor(line.from), effects: EditorView.scrollIntoView(line.from, { y: 'center' }) })
    v.focus()
  }, [jump, path])

  useEffect(() => {
    const v = view.current!
    if (!insert || insert.path !== path) return
    v.dispatch(v.state.replaceSelection(insert.text), { scrollIntoView: true, userEvent: 'input' })
    v.focus()
    onInserted?.()
  }, [insert, path])

  // Right click: clipboard, AI actions on the selection, sync and compile.
  const menu = (): MenuEntry[] => {
    const v = view.current
    if (!v) return []
    const sel = v.state.selection.main
    const text = v.state.sliceDoc(sel.from, sel.to)
    const edit = (insert: string, userEvent: string) => {
      v.dispatch(v.state.replaceSelection(insert), { userEvent, scrollIntoView: true })
      v.focus()
    }
    return [
      {
        label: t('Cut'),
        icon: Scissors,
        shortcut: `${mod}X`,
        disabled: sel.empty,
        onSelect: () => navigator.clipboard.writeText(text).then(() => edit('', 'delete.cut')),
      },
      { label: t('Copy'), icon: Copy, shortcut: `${mod}C`, disabled: sel.empty, onSelect: () => navigator.clipboard.writeText(text) },
      { label: t('Paste'), icon: ClipboardPaste, shortcut: `${mod}V`, onSelect: () => navigator.clipboard.readText().then((s) => edit(s, 'input.paste')) },
      {
        label: t('Select all'),
        icon: TextSelect,
        shortcut: `${mod}A`,
        onSelect: () => {
          v.dispatch({ selection: { anchor: 0, head: v.state.doc.length } })
          v.focus()
        },
      },
      '-',
      ...(quick && !sel.empty
        ? [
            ...actions.map((a) => ({ label: a.command, icon: a.icon, onSelect: () => quickRef.current?.run(a.id) })),
            { label: t('Ask the assistant…'), icon: Pencil, onSelect: () => quickRef.current?.ask() },
            '-' as const,
          ]
        : []),
      {
        label: t('Show in PDF'),
        icon: FileSearch,
        shortcut: `${mod}J`,
        onSelect: () => {
          const line = v.state.doc.lineAt(sel.head)
          cb.current.onSync(current.current, line.number, sel.head - line.from + 1)
        },
      },
      { label: t('Compile'), icon: Play, shortcut: `${mod}↵`, onSelect: () => cb.current.onCompile() },
    ]
  }

  return (
    <div className="relative h-full">
      <WithContextMenu items={menu} className="h-full select-text">
        <div className="h-full" ref={host} />
      </WithContextMenu>
      {quick && mounted && <QuickActions view={mounted} path={path} dragging={dragging} handle={quickRef} />}
    </div>
  )
}
