// Run with: node --test web/src/lib/remote.test.ts
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { remoteHost, remoteOrigin, tokenLeftBehind } from './remote.ts'

// The same pairs the server is tested with (internal/projects/remote_origin_test.go).
const cases: { same: [string, string][]; different: [string, string][] } = JSON.parse(
  readFileSync(new URL('./remote-origins.json', import.meta.url), 'utf8'),
)

test('agrees with the server on which URLs share an origin', () => {
  for (const [a, b] of cases.same) {
    assert.notEqual(remoteOrigin(a), '', a)
    assert.equal(remoteOrigin(a), remoteOrigin(b), `${a} ~ ${b}`)
    assert.equal(tokenLeftBehind(a, b), false, `${a} → ${b}`)
  }
  for (const [a, b] of cases.different) {
    assert.notEqual(remoteOrigin(a), remoteOrigin(b), `${a} ≁ ${b}`)
    assert.equal(tokenLeftBehind(a, b), true, `${a} → ${b}`)
  }
})

test('no warning for something that is not an https URL yet', () => {
  const saved = 'https://git.lab.org/a/b.git'
  for (const typed of ['', 'git.lab.org/x', 'https://', 'http://evil.example/a.git', 'ssh://evil.example/a.git']) {
    assert.equal(tokenLeftBehind(saved, typed), false, typed)
  }
})

test('names the host for the person', () => {
  assert.equal(remoteHost('https://git.lab.org:8443/a.git'), 'git.lab.org:8443')
  assert.equal(remoteHost('https://github.com/a.git'), 'github.com')
})
