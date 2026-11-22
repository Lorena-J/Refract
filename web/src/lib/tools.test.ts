// Run with: node --test web/src/lib/tools.test.ts
import assert from 'node:assert/strict'
import { test } from 'node:test'

import { getMissingTools, setMissingTools } from './tools.ts'

test('only known tools are kept, in a stable order', () => {
  setMissingTools(['rg', 'bogus', 'git'])
  assert.deepEqual(getMissingTools(), ['git', 'rg'])
  setMissingTools(undefined)
  assert.deepEqual(getMissingTools(), [])
})
