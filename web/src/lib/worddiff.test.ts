// Run with: node --test web/src/lib/worddiff.test.ts
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { wordDiff, type Op } from './worddiff.ts'

const apply = (ops: Op[], side: 'a' | 'b') =>
  ops
    .filter((o) => o.op === 'eq' || o.op === (side === 'a' ? 'del' : 'ins'))
    .map((o) => o.text)
    .join('')

test('reconstructs both sides', () => {
  for (const [a, b] of [
    ['The results is good.', 'The results are good.'],
    ['\\textbf{Hola} mundo, $x^2$', '\\emph{Hola} mundo; $x^{2}$'],
    ['line one\nline two\n', 'Line one.\n\nline two\n'],
    ['', 'new'],
    ['old', ''],
  ]) {
    const ops = wordDiff(a, b)
    assert.equal(apply(ops, 'a'), a)
    assert.equal(apply(ops, 'b'), b)
  }
})

test('marks only what changed', () => {
  assert.deepEqual(wordDiff('The results is good.', 'The results are good.'), [
    { op: 'eq', text: 'The results ' },
    { op: 'del', text: 'is' },
    { op: 'ins', text: 'are' },
    { op: 'eq', text: ' good.' },
  ])
})

test('a lone space between two changes joins them', () => {
  assert.deepEqual(wordDiff('a big cat', 'a small dog'), [
    { op: 'eq', text: 'a ' },
    { op: 'del', text: 'big cat' },
    { op: 'ins', text: 'small dog' },
  ])
})
