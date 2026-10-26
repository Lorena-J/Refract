import { useEffect, useRef, useState } from 'react'
import { basicSetup } from 'codemirror'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { StreamLanguage } from '@codemirror/language'
import { stex } from '@codemirror/legacy-modes/mode/stex'
import { unifiedMergeView } from '@codemirror/merge'
import { api, errorText } from './api'
import type { Turn } from './Assistant'

type FileDiff = { path: string; status: 'added' | 'modified' | 'deleted'; before: string; after: string; binary: boolean }

type Props = {
  turn: Turn
  onAccept: (files: string[] | null) => Promise<string[] | null> // resolves to conflicts, if any
  onReject: () => Promise<void>
  onClose: () => void
}

// Shows a turn's pending changes as unified diffs, file by file.
export default function Review({ turn, onAccept, onReject, onClose }: Props) {
  const [diffs, setDiffs] = useState<FileDiff[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')

  useEffect(() => {
    api<FileDiff[]>(`/api/turns/${turn.id}/diff`)
      .then(setDiffs)
      .catch((e) => setMessage(errorText(e)))
  }, [turn.id, turn.files.length])

  async function act(fn: () => Promise<string[] | null | void>) {
    setBusy(true)
    setMessage('')
    try {
      const conflicts = await fn()
      if (conflicts && conflicts.length) {
        setMessage(
          `Not applied: ${conflicts.join(', ')} changed since the assistant started, in the same places. Nothing was changed. Edit the file or reject these changes.`,
        )
      }
    } catch (e) {
      setMessage(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="review">
      <div className="review-bar">
        <strong>Review changes</strong>
        <span className="hint">{turn.files.length} file{turn.files.length === 1 ? '' : 's'}</span>
        <span className="spacer" />
        <button disabled={busy} onClick={() => act(() => onAccept(null))}>
          Accept all
        </button>
        <button disabled={busy} onClick={() => act(onReject)}>
          Reject all
        </button>
        <button className="link" onClick={onClose}>
          Close
        </button>
      </div>
      {message && <p className="error review-message">{message}</p>}
      <div className="review-files">
        {diffs?.map((d) => (
          <section key={d.path} className="review-file">
            <div className="review-file-bar">
              <code>{d.path}</code>
              <span className="hint">{d.status}</span>
              <span className="spacer" />
              <button disabled={busy} onClick={() => act(() => onAccept([d.path]))}>
                Accept file
              </button>
            </div>
            {d.binary ? <p className="hint">Binary file changed.</p> : <DiffView before={d.before} after={d.after} />}
          </section>
        ))}
      </div>
    </div>
  )
}

function DiffView({ before, after }: { before: string; after: string }) {
  const host = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const view = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: after,
        extensions: [
          basicSetup,
          StreamLanguage.define(stex),
          EditorView.lineWrapping,
          EditorState.readOnly.of(true),
          EditorView.editable.of(false),
          unifiedMergeView({ original: before, mergeControls: false, collapseUnchanged: { margin: 3, minSize: 6 } }),
        ],
      }),
    })
    return () => view.destroy()
  }, [before, after])
  return <div ref={host} className="diff-view" />
}
