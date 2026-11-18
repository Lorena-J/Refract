import { RangeSet, RangeValue, type Range, type Text } from '@codemirror/state'
import { EditorView, getPanel, showPanel, type Panel } from '@codemirror/view'
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
    read(view: EditorView) {
      const doc = view.state.doc
      const from = view.lineBlockAtHeight(view.scrollDOM.scrollTop - view.documentPadding.top).from
      // Leave the final line unobstructed when the editor is scrolled past the end.
      if (from === 0 || from >= doc.line(doc.lines).from) return { doc, starts: [] }
      if (indexedDoc !== doc) {
        indexedDoc = doc
        scopes = findScopes(doc)
      }
      const starts = new Set<number>()
      scopes.between(from, from, (start, end) => {
        if (start < from && from < end) starts.add(start)
      })
      return { doc, starts: [...starts].sort((a, b) => a - b).slice(-maxHeaders) }
    },
    write({ doc, starts }: { doc: Text; starts: number[] }) {
      if (doc === shownDoc && starts.length === shown.length && starts.every((start, index) => start === shown[index]))
        return
      shownDoc = doc
      shown = starts
      dom.hidden = starts.length === 0
      dom.replaceChildren(
        ...starts.map((start) => {
          const row = document.createElement('div')
          row.textContent = doc.lineAt(start).text
          return row
        }),
      )
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
      pointerEvents: 'none',
      backgroundColor: 'var(--card)',
      borderBottom: '1px solid var(--border)',
      color: 'var(--foreground)',
      fontFamily: 'var(--font-mono)',
      lineHeight: '1.65',
      paddingInline: '0.75rem',
    },
    '.cm-sticky-scroll > div': { whiteSpace: 'pre', overflow: 'hidden', textOverflow: 'ellipsis' },
  }),
]
