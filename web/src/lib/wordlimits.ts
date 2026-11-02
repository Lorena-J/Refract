import { StateEffect, StateField } from '@codemirror/state'
import { Decoration, EditorView, type DecorationSet } from '@codemirror/view'
import { defaultRules, findSections, latexWords, selectedWords, type CountRules } from './wordcount.ts'

export type WordLimit = { id: string; name: string; max: number; from: number; to: number; section?: string }
export type Limits = { rules: CountRules; ranges: WordLimit[] }
export const setLimits = StateEffect.define<Limits>()
export const wordLimits = StateField.define<Limits>({
  create: () => ({ rules: { ...defaultRules }, ranges: [] }),
  update(value, tr) {
    for (const effect of tr.effects) if (effect.is(setLimits)) return effect.value
    if (!tr.docChanged) return value
    const sections = findSections(tr.newDoc.toString())
    return {
      ...value,
      ranges: value.ranges.flatMap((range) => {
        if (range.section) {
          const mapped = tr.changes.mapPos(range.from, -1)
          const section = sections.find((s) => s.from === mapped) ?? sections.find((s) => s.id === range.section)
          return section ? [{ ...range, section: section.id, from: section.from, to: section.to }] : []
        }
        const from = tr.changes.mapPos(range.from, -1)
        const to = tr.changes.mapPos(range.to, 1)
        return [{ ...range, from, to }]
      }),
    }
  },
})

export const excessWords = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(value, tr) {
    if (!tr.docChanged && tr.startState.field(wordLimits) === tr.state.field(wordLimits)) return value
    const { ranges, rules } = tr.state.field(wordLimits)
    if (!ranges.length) return Decoration.none
    const words = latexWords(tr.newDoc.toString(), rules)
    const excess = new Map<string, { from: number; to: number }>()
    for (const range of ranges) {
      for (const word of selectedWords(words, range.from, range.to).slice(range.max)) {
        excess.set(`${word.from}:${word.to}`, {
          from: Math.max(range.from, word.from),
          to: Math.min(range.to, word.to),
        })
      }
    }
    return Decoration.set(
      [...excess.values()]
        .filter((w) => w.to > w.from)
        .map((w) => Decoration.mark({ class: 'cm-word-excess' }).range(w.from, w.to)),
      true,
    )
  },
  provide: (field) => EditorView.decorations.from(field),
})

const key = (project: string, path: string) => `refract-wordlimits:${project}:${path}`
export function saveLimits(project: string, path: string, source: string, limits: Limits) {
  try {
    localStorage.setItem(key(project, path), JSON.stringify({ source, ...limits }))
  } catch {
    /* storage unavailable */
  }
}
export function loadLimits(project: string, path: string, source: string): Limits {
  const fallback = { rules: { ...defaultRules }, ranges: [] }
  try {
    const data = JSON.parse(localStorage.getItem(key(project, path)) ?? 'null')
    if (!data || !Array.isArray(data.ranges)) return fallback
    const rules = { ...defaultRules }
    for (const k of Object.keys(rules) as (keyof CountRules)[])
      if (typeof data.rules?.[k] === 'boolean') rules[k] = data.rules[k]
    const sections = findSections(source)
    const ranges = (data.ranges as WordLimit[]).flatMap((r) => {
      if (!r || typeof r.id !== 'string' || typeof r.name !== 'string' || !Number.isSafeInteger(r.max) || r.max < 1)
        return []
      if (r.section) {
        const s = sections.find((s) => s.id === r.section)
        return s ? [{ ...r, from: s.from, to: s.to }] : []
      }
      // Don't silently apply stale offsets after changes outside this editor.
      return data.source === source &&
        Number.isInteger(r.from) &&
        Number.isInteger(r.to) &&
        r.from >= 0 &&
        r.to >= r.from &&
        r.to <= source.length
        ? [r]
        : []
    })
    return { rules, ranges }
  } catch {
    return fallback
  }
}
