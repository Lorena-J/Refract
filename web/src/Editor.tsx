import { useEffect, useRef } from 'react'
import { basicSetup } from 'codemirror'
import { EditorSelection, EditorState, Prec } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { StreamLanguage } from '@codemirror/language'
import { stex } from '@codemirror/legacy-modes/mode/stex'

const theme = EditorView.theme({
  '&': { height: '100%', backgroundColor: 'var(--bg)', color: 'var(--fg)' },
  '.cm-scroller': { fontFamily: 'ui-monospace, monospace', lineHeight: '1.5' },
  '.cm-gutters': { backgroundColor: 'var(--bg)', color: 'var(--muted)', borderRight: '1px solid var(--border)' },
  '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: 'var(--active-line)' },
  '&.cm-focused .cm-cursor': { borderLeftColor: 'var(--fg)' },
  '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection': { backgroundColor: 'var(--selection)' },
})

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
  jump?: { path: string; line: number; n: number } // n changes on every request
}

// One EditorView; each open file keeps its own EditorState, swapped on tab switch.
export default function Editor({ path, states, initial, onChange, onSave, onCompile, onSync, jump }: Props) {
  const host = useRef<HTMLDivElement>(null)
  const view = useRef<EditorView | null>(null)
  const current = useRef(path)
  const cb = useRef({ onChange, onSave, onCompile, onSync })
  cb.current = { onChange, onSave, onCompile, onSync }

  const extensions = useRef([
    basicSetup,
    StreamLanguage.define(stex),
    EditorView.lineWrapping,
    theme,
    // Highest precedence: basicSetup binds Mod-Enter to "insert blank line".
    Prec.highest(
      keymap.of([
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
    view.current = new EditorView({ parent: host.current!, state: stateFor(path) })
    return () => view.current!.destroy()
  }, [])

  useEffect(() => {
    const v = view.current!
    const next = stateFor(path)
    current.current = path
    if (v.state !== next) v.setState(next)
    v.focus()
  }, [path])

  useEffect(() => {
    const v = view.current!
    if (!jump || jump.path !== path) return
    const line = v.state.doc.line(Math.min(Math.max(jump.line, 1), v.state.doc.lines))
    v.dispatch({ selection: EditorSelection.cursor(line.from), effects: EditorView.scrollIntoView(line.from, { y: 'center' }) })
    v.focus()
  }, [jump, path])

  return <div className="editor-host" ref={host} />
}
