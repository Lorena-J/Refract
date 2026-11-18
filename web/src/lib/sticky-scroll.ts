import { syntaxTree } from '@codemirror/language'
import { RangeSet, RangeValue, type Range, type Text } from '@codemirror/state'
import { EditorView, getPanel, showPanel, type Panel } from '@codemirror/view'
import { highlightTree } from '@lezer/highlight'
import { latexStyle } from './editor'
import { headingsIn } from './outline'
import { maskForSections } from './wordcount'

class Scope extends RangeValue {}
const scope = new Scope()
const maxHeaders = 5

function findScopes(doc: Text) {
  const source = doc.toString()
  const masked = maskForSections(source, true)
  const ranges: Range<RangeValue>[] = []
  const add = (from: number, to: number) => {
    if (from < to) ranges.push(scope.range(from, to))
  }

  const documentEnd = masked.search(/\\end\s*\{document\}/)
  const end = documentEnd < 0 ? doc.length : doc.lineAt(documentEnd).from
  const sections: { from: number; level: number }[] = []
  for (const heading of headingsIn(source, '')) {
    const from = doc.line(heading.line).from
    while (sections.length && sections[sections.length - 1].level >= heading.level) {
      add(sections.pop()!.from, from)
    }
    sections.push({ from, level: heading.level })
  }
  for (const section of sections) add(section.from, end)

  const environments: { name: string; from: number }[] = []
  for (const match of masked.matchAll(/\\\\|\\(begin|end)\s*\{([^{}\s]+)\}/g)) {
    const [, command, name] = match
    if (!command) continue
    const from = doc.lineAt(match.index).from
    if (command === 'begin') {
      environments.push({ name, from })
    } else {
      let index = environments.length - 1
      while (index >= 0 && environments[index].name !== name) index--
      if (index >= 0) {
        for (const environment of environments.splice(index)) add(environment.from, from)
      }
    }
  }
  for (const environment of environments) add(environment.from, doc.length)
  return RangeSet.of(ranges, true)
}

// The header line as the editor draws it, so a line that sticks looks the same as it did in the text.
function headerRow(view: EditorView, doc: Text, start: number) {
  const row = document.createElement('div')
  const line = doc.lineAt(start)
  let pos = line.from
  highlightTree(
    syntaxTree(view.state),
    latexStyle,
    (from, to, classes) => {
      if (from > pos) row.append(doc.sliceString(pos, from))
      const span = document.createElement('span')
      span.className = classes
      span.textContent = doc.sliceString(from, to)
      row.append(span)
      pos = to
    },
    line.from,
    line.to,
  )
  row.append(doc.sliceString(pos, line.to))
  return row
}

function stickyPanel(view: EditorView): Panel {
  const dom = document.createElement('div')
  dom.className = 'cm-sticky-scroll'
  dom.hidden = true
  dom.setAttribute('aria-hidden', 'true')
  let indexedDoc: Text | undefined
  let scopes = RangeSet.empty
  let shownDoc: Text | undefined
  let shown: number[] = []
  const measure = {
    key: dom,
    read(view: EditorView) {
      const doc = view.state.doc
      if (indexedDoc !== doc) {
        indexedDoc = doc
        scopes = findScopes(doc)
      }
      const height = view.defaultLineHeight
      const scrollTop = view.scrollDOM.scrollTop - view.documentPadding.top
      // Scopes that reach the rows the headers can cover; the outermost when several start on a line.
      const candidates = new Map<number, number>()
      scopes.between(
        view.lineBlockAtHeight(scrollTop).from,
        view.lineBlockAtHeight(scrollTop + maxHeaders * height).to,
        (from, to) => {
          if (to > (candidates.get(from) ?? 0)) candidates.set(from, to)
        },
      )
      // Each header takes the next row once its own line has scrolled past that row, and keeps it
      // until its scope ends there. The end of a scope pushes its header up pixel by pixel, so the
      // next one arrives in the row exactly as this one leaves it.
      const starts: number[] = []
      let push = 0
      for (const [from, to] of [...candidates].sort((a, b) => a[0] - b[0])) {
        const row = scrollTop + starts.length * height
        if (view.lineBlockAt(from).top >= row) break
        const end = view.lineBlockAt(to).top
        if (end <= row) continue
        starts.push(from)
        push = Math.min(0, end - row - height)
        if (push < 0 || starts.length === maxHeaders) break
      }
      // Text starts where the editor's lines do: after the gutters and the line's own padding.
      return { doc, starts, push, height, left: view.contentDOM.offsetLeft + 6 }
    },
    write({
      doc,
      starts,
      push,
      height,
      left,
    }: {
      doc: Text
      starts: number[]
      push: number
      height: number
      left: number
    }) {
      if (doc !== shownDoc || starts.length !== shown.length || starts.some((start, index) => start !== shown[index])) {
        shownDoc = doc
        shown = starts
        dom.hidden = starts.length === 0
        dom.replaceChildren(...starts.map((start) => headerRow(view, doc, start)))
      }
      dom.style.setProperty('--sticky-row', `${height}px`)
      dom.style.paddingLeft = `${left}px`
      dom.style.height = `${starts.length * height + push}px`
      if (dom.lastElementChild) (dom.lastElementChild as HTMLElement).style.marginTop = `${push}px`
    },
  }
  const refresh = () => view.requestMeasure(measure)
  const workspace = view.dom.closest<HTMLElement>('.editor-workspace')
  const resize = new ResizeObserver(([entry]) => {
    workspace?.style.setProperty('--sticky-scroll-height', `${entry.borderBoxSize[0].blockSize}px`)
  })
  return {
    dom,
    top: true,
    mount() {
      if (workspace) resize.observe(dom, { box: 'border-box' })
      view.scrollDOM.addEventListener('scroll', refresh, { passive: true })
      refresh()
    },
    update(update) {
      if (update.docChanged || update.geometryChanged || update.viewportChanged) refresh()
    },
    destroy() {
      resize.disconnect()
      workspace?.style.removeProperty('--sticky-scroll-height')
      view.scrollDOM.removeEventListener('scroll', refresh)
    },
  }
}

export const stickyScroll = [
  showPanel.of(stickyPanel),
  EditorView.scrollMargins.of((view) => ({ top: getPanel(view, stickyPanel)?.dom.offsetHeight ?? 0 })),
  EditorView.theme({
    '.cm-panels-top:has(> .cm-sticky-scroll)': { borderBottom: '0', zIndex: '1' },
    '.cm-sticky-scroll': {
      position: 'absolute',
      inset: '0 0 auto',
      isolation: 'isolate',
      overflow: 'hidden',
      pointerEvents: 'none',
      backgroundColor: 'var(--card)',
      borderBottom: '1px solid var(--border)',
      color: 'var(--foreground)',
      fontFamily: 'var(--font-mono)',
      paddingRight: '0.75rem',
    },
    '.cm-sticky-scroll > div': {
      position: 'relative',
      height: 'var(--sticky-row)',
      lineHeight: 'var(--sticky-row)',
      backgroundColor: 'var(--card)',
      whiteSpace: 'pre',
      overflow: 'hidden',
      textOverflow: 'ellipsis',
    },
    // The header being pushed out slides under the one above it.
    '.cm-sticky-scroll > div:last-child': { zIndex: '-1' },
  }),
]
