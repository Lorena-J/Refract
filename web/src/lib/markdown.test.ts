// Run with: node --test web/src/lib/markdown.test.ts
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { parseInline, parseMarkdown } from './markdown.ts'

test('inline formatting', () => {
  assert.deepEqual(parseInline('a **bold** and `x` and *it*'), [
    { t: 'text', v: 'a ' },
    { t: 'strong', c: [{ t: 'text', v: 'bold' }] },
    { t: 'text', v: ' and ' },
    { t: 'code', v: 'x' },
    { t: 'text', v: ' and ' },
    { t: 'em', c: [{ t: 'text', v: 'it' }] },
  ])
})

test('links only for http(s); no HTML ever', () => {
  assert.deepEqual(parseInline('[docs](https://example.com)'), [{ t: 'link', href: 'https://example.com', c: [{ t: 'text', v: 'docs' }] }])
  assert.deepEqual(parseInline('[x](javascript:alert(1))'), [{ t: 'text', v: '[x](javascript:alert(1))' }])
  assert.deepEqual(parseInline('<img src=x onerror=alert(1)>'), [{ t: 'text', v: '<img src=x onerror=alert(1)>' }])
})

test('blocks', () => {
  const md = 'I changed:\n\n- **intro**\n- `main.tex`\n\n1. one\n2. two\n\n```latex\n\\section{A}\n```\n## Done\nLast line'
  const b = parseMarkdown(md)
  assert.deepEqual(
    b.map((x) => x.t),
    ['p', 'ul', 'ol', 'pre', 'h', 'p'],
  )
  assert.equal(b[3].t === 'pre' && b[3].v, '\\section{A}')
  assert.equal(b[1].t === 'ul' && b[1].items.length, 2)
})
