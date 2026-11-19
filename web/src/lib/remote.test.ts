// Run with: node --test web/src/lib/remote.test.ts
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { remoteHost, remoteOrigin, tokenLeftBehind } from './remote.ts'

test('origins compare scheme, host and port, not path or case', () => {
  assert.equal(remoteOrigin('https://GitHub.com/a/b.git'), 'https://github.com:443')
  assert.equal(remoteOrigin('https://github.com:443/c'), 'https://github.com:443')
  assert.equal(remoteOrigin('https://git.lab.org:8443/x.git'), 'https://git.lab.org:8443')
  assert.equal(remoteOrigin('git.lab.org/x'), '')
  assert.equal(remoteOrigin(''), '')
})

test('a token is left behind on another host or port only', () => {
  const saved = 'https://git.lab.org/a/b.git'
  assert.equal(tokenLeftBehind(saved, 'https://git.lab.org/a/c.git'), false)
  assert.equal(tokenLeftBehind(saved, 'https://GIT.LAB.ORG:443/a/b.git'), false)
  assert.equal(tokenLeftBehind(saved, 'https://git.lab.org:8443/a/b.git'), true)
  assert.equal(tokenLeftBehind(saved, 'https://git.lab.org.evil.example/a/b.git'), true)
  assert.equal(tokenLeftBehind(saved, 'https://evil.example/a/b.git'), true)
})

test('names the host for the person', () => {
  assert.equal(remoteHost('https://git.lab.org:8443/a.git'), 'git.lab.org:8443')
  assert.equal(remoteHost('https://github.com/a.git'), 'github.com')
})
