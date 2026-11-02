import { test } from 'node:test'
import assert from 'node:assert/strict'
import { EditorState } from '@codemirror/state'
import { defaultRules, findSections } from './wordcount.ts'
import { excessWords, loadLimits, saveLimits, setLimits, wordLimits } from './wordlimits.ts'

const limits = (state: EditorState) => state.field(wordLimits)
const create = (source = 'one two three') => EditorState.create({ doc: source, extensions: [wordLimits, excessWords] })

test('manual limits follow edits before and inside the range, including boundaries', () => {
  let state = create()
  state = state.update({
    effects: setLimits.of({ rules: defaultRules, ranges: [{ id: 'a', name: 'Test', from: 4, to: 7, max: 1 }] }),
  }).state
  state = state.update({ changes: { from: 0, insert: 'start ' } }).state
  assert.equal(limits(state).ranges[0].from, 10)
  state = state.update({ changes: { from: 13, insert: ' more' } }).state
  const range = limits(state).ranges[0]
  assert.equal(state.sliceDoc(range.from, range.to), 'two more')
  assert.equal(state.field(excessWords).size, 1)
  state = state.update({ changes: { from: range.from, to: range.to, insert: 'short' } }).state
  assert.equal(state.field(excessWords).size, 0)
})

test('automatic limits follow the section body and retain the limit when its title changes', () => {
  let state = create(String.raw`\section{Intro}one two\section{Next}three`)
  const section = findSections(state.doc.toString())[0]
  state = state.update({
    effects: setLimits.of({ rules: defaultRules, ranges: [{ ...section, section: section.id, max: 1 }] }),
  }).state
  const at = state.doc.toString().indexOf('Intro')
  state = state.update({ changes: { from: at, to: at + 5, insert: 'Introduction' } }).state
  assert.equal(limits(state).ranges.length, 1)
  const range = limits(state).ranges[0]
  assert.equal(state.sliceDoc(range.from, range.to), 'one two')
  assert.equal(range.section, 'section:Introduction:0')
})

test('rule changes update red decorations, overlapping limits do not duplicate marks', () => {
  let state = create(String.raw`one \footnote{two three}`)
  const range = { id: 'a', name: 'A', from: 0, to: state.doc.length, max: 1 }
  state = state.update({ effects: setLimits.of({ rules: defaultRules, ranges: [range, { ...range, id: 'b' }] }) }).state
  assert.equal(state.field(excessWords).size, 2)
  state = state.update({
    effects: setLimits.of({ ...limits(state), rules: { ...defaultRules, footnotes: false } }),
  }).state
  assert.equal(state.field(excessWords).size, 0)
})

test('persistence is isolated per project and file; stale manual offsets are discarded', () => {
  const values = new Map<string, string>()
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
    },
  })
  const ranges = [{ id: 'a', name: 'A', from: 0, to: 3, max: 2 }]
  saveLimits('project', 'main.tex', 'one', { rules: defaultRules, ranges })
  assert.deepEqual(loadLimits('project', 'main.tex', 'one').ranges, ranges)
  assert.equal(loadLimits('other', 'main.tex', 'one').ranges.length, 0)
  assert.equal(loadLimits('project', 'other.tex', 'one').ranges.length, 0)
  assert.equal(loadLimits('project', 'main.tex', 'changed').ranges.length, 0)
})
