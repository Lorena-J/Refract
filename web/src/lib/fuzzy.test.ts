// Run with: node --test web/src/lib/fuzzy.test.ts
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { score } from './fuzzy.ts'

test('matches in order, rejects otherwise', () => {
  assert.ok(score('chapters/intro.tex', 'intro') > 0)
  assert.ok(score('chapters/intro.tex', 'chint') > 0)
  assert.equal(score('main.tex', 'xyz'), 0)
  assert.equal(score('main.tex', 'tnam'), 0)
})

test('ranks prefix and word starts first', () => {
  const rank = (q: string, items: string[]) => [...items].sort((a, b) => score(b, q) - score(a, q))
  assert.deepEqual(rank('com', ['Recompute', 'Compile', 'New chat']).slice(0, 2), ['Compile', 'Recompute'])
  assert.equal(rank('intro', ['sec/introduction.tex', 'main.tex', 'appendix/outro-intro.tex'])[0], 'sec/introduction.tex')
  assert.equal(rank('nf', ['Toggle sidebar', 'New file', 'Font size'])[0], 'New file')
})
