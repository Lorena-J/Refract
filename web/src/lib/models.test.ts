// Run with: node --test web/src/lib/models.test.ts
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { resolveChoice } from './models.ts'

const ms = [
  { slug: 'a', display_name: 'A', description: '', default_effort: 'medium', efforts: [{ effort: 'low', description: '' }, { effort: 'medium', description: '' }] },
  { slug: 'b', display_name: 'B', description: '', default_effort: '', efforts: [] },
]

test('stored pick when offered, defaults otherwise', () => {
  assert.deepEqual(resolveChoice(ms, 'a', 'low'), { model: ms[0], effort: 'low' })
  assert.deepEqual(resolveChoice(ms, 'a', 'max'), { model: ms[0], effort: 'medium' })
  assert.deepEqual(resolveChoice(ms, 'gone', 'low'), { model: ms[0], effort: 'low' })
  assert.deepEqual(resolveChoice(ms, 'b', 'low'), { model: ms[1], effort: '' })
  assert.deepEqual(resolveChoice([], 'a', 'low'), { model: undefined, effort: '' })
})
