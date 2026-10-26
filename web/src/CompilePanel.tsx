import { useEffect, useState } from 'react'
import { api, download, errorText, type Project, type Warning } from './api'

export type Problem = { severity: 'error' | 'warning'; kind: string; file?: string; line?: number; message: string }
export type CompileResult = {
  ok: boolean
  problems: Problem[]
  output: string
  timed_out: boolean
  superseded: boolean
  duration_ms: number
  pdf?: string
  checkpoint: boolean
}

type Props = {
  project: Project
  texFiles: string[]
  compiling: boolean
  result: CompileResult | null
  error: string
  onCompile: () => void
  onMainFile: (path: string) => void
  onJump: (file: string, line: number) => void
}

// Noisy kinds, collapsed by default.
const groups = [
  { kinds: ['overfull', 'underfull'], label: (n: number) => `${n} overfull/underfull box warning${n > 1 ? 's' : ''}` },
  { kinds: ['absolute-path'], label: (n: number) => `${n} file${n > 1 ? 's' : ''} read from outside the project (e.g. system fonts): the build depends on this machine` },
]

export default function CompilePanel(p: Props) {
  const [checks, setChecks] = useState<Warning[]>([])
  const [expanded, setExpanded] = useState<number[]>([])
  const [log, setLog] = useState<string | null>(null)

  // Portability checks follow the sources: refresh after every compile.
  useEffect(() => {
    api<Warning[]>(`/api/projects/${p.project.id}/checks`)
      .then(setChecks)
      .catch(() => {})
    setLog(null)
  }, [p.project.id, p.result])

  const problems = p.result?.problems ?? []
  const main = problems.filter((x) => !groups.some((g) => g.kinds.includes(x.kind)))

  async function toggleLog() {
    if (log !== null) return setLog(null)
    try {
      const res = await fetch(`/api/projects/${p.project.id}/log`, { headers: { 'X-Refract': '1' } })
      setLog(res.ok ? await res.text() : 'No log yet: compile first.')
    } catch (e) {
      setLog(errorText(e))
    }
  }

  async function openPDF() {
    const res = await fetch(`/api/projects/${p.project.id}/pdf`, { headers: { 'X-Refract': '1' } })
    if (res.ok) window.open(URL.createObjectURL(await res.blob()), '_blank')
  }

  const errors = problems.filter((x) => x.severity === 'error').length
  const secs = p.result ? (p.result.duration_ms / 1000).toFixed(1) : ''
  const status = p.compiling
    ? 'Compiling…'
    : !p.result
      ? ''
      : p.result.ok
        ? `Compiled in ${secs} s${p.result.checkpoint ? ' · checkpoint saved' : ''}`
        : p.result.superseded
          ? 'Cancelled'
          : p.result.pdf
            ? `Compiled with ${errors} error${errors === 1 ? '' : 's'} in ${secs} s`
            : 'Failed'
  const hasPDF = !!p.result?.pdf

  return (
    <div className="compile">
      <div className="compile-bar">
        <button onClick={p.onCompile} disabled={p.compiling || !p.project.main_file} title="Compile (Cmd/Ctrl+Enter)">
          Compile
        </button>
        <select value={p.project.main_file} onChange={(e) => p.onMainFile(e.target.value)} aria-label="Main file">
          {!p.project.main_file && <option value="">Choose main file…</option>}
          {p.texFiles.map((f) => (
            <option key={f}>{f}</option>
          ))}
        </select>
        <span className={'hint' + (p.result && !p.result.ok && !p.compiling ? ' error' : '')}>{status}</span>
      </div>
      {p.error && <p className="error">{p.error}</p>}

      <ul className="problems">
        {main.map((x, i) => (
          <ProblemRow key={i} problem={x} onJump={p.onJump} />
        ))}
        {groups.map((g, gi) => {
          const items = problems.filter((x) => g.kinds.includes(x.kind))
          if (items.length === 0) return null
          const isOpen = expanded.includes(gi)
          return [
            <li key={'g' + gi} className="problem">
              <button className="link" onClick={() => setExpanded(isOpen ? expanded.filter((x) => x !== gi) : [...expanded, gi])}>
                {isOpen ? '▾' : '▸'} {g.label(items.length)}
              </button>
            </li>,
            ...(isOpen ? items.map((x, i) => <ProblemRow key={`g${gi}-${i}`} problem={x} onJump={p.onJump} />) : []),
          ]
        })}
        {checks.map((w, i) => (
          <ProblemRow key={'c' + i} problem={{ severity: 'warning', kind: 'portability', file: w.file, line: w.line, message: w.message }} onJump={p.onJump} />
        ))}
      </ul>

      <div className="compile-links">
        {hasPDF && (
          <button className="link" onClick={() => openPDF()}>
            Open PDF
          </button>
        )}
        <button className="link" onClick={toggleLog}>
          {log === null ? 'Show full log' : 'Hide log'}
        </button>
        {hasPDF && (
          <button className="link" onClick={() => download(`/api/projects/${p.project.id}/pdf`, `${p.project.name}.pdf`).catch(() => {})}>
            Download PDF
          </button>
        )}
      </div>
      {log !== null && <pre className="log raw-log">{log}</pre>}
    </div>
  )
}

function ProblemRow({ problem: x, onJump }: { problem: Problem; onJump: (file: string, line: number) => void }) {
  const where = x.file ? `${x.file}${x.line ? ':' + x.line : ''}` : ''
  return (
    <li className={'problem ' + x.severity}>
      <span className="problem-kind">{x.severity === 'error' ? 'Error' : x.kind === 'portability' ? 'Portability' : 'Warning'}</span>
      {where ? (
        <button className="link problem-where" onClick={() => onJump(x.file!, x.line ?? 1)}>
          {where}
        </button>
      ) : null}
      <span className="problem-msg">{x.message}</span>
    </li>
  )
}
