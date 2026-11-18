// Run with: node --test web/src/lib/device.test.ts
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { device } from './device.ts'

test('names common browsers and systems', () => {
  const cases: [string, string | null, string | null][] = [
    [
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.1 Safari/605.1.15',
      'Safari',
      'macOS',
    ],
    [
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36',
      'Chrome',
      'Windows',
    ],
    [
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36 Edg/142.0.0.0',
      'Edge',
      'Windows',
    ],
    ['Mozilla/5.0 (X11; Linux x86_64; rv:145.0) Gecko/20100101 Firefox/145.0', 'Firefox', 'Linux'],
    [
      'Mozilla/5.0 (iPhone; CPU iPhone OS 19_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/142.0 Mobile/15E148 Safari/604.1',
      'Chrome',
      'iOS',
    ],
    [
      'Mozilla/5.0 (iPad; CPU OS 19_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.1 Mobile/15E148 Safari/604.1',
      'Safari',
      'iPadOS',
    ],
    [
      'Mozilla/5.0 (Linux; Android 16; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Mobile Safari/537.36',
      'Chrome',
      'Android',
    ],
    [
      'Mozilla/5.0 (X11; CrOS x86_64 16000.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36',
      'Chrome',
      'ChromeOS',
    ],
  ]
  for (const [ua, browser, os] of cases) assert.deepEqual(device(ua), { browser, os }, ua)
})

test("says nothing it can't tell", () => {
  assert.deepEqual(device(''), { browser: null, os: null })
  assert.deepEqual(device('curl/8.10.1'), { browser: null, os: null })
})
