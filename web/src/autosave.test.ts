// Run with: node --test web/src/autosave.test.ts (Node ≥ 22.18 strips the types).
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { Autosaver } from './autosave.ts'

// A fake server whose saves complete in a controllable order.
function fakeDisk() {
  const disk = new Map<string, string>()
  const queue: { path: string; content: string; resolve: () => void; reject: (e: Error) => void }[] = []
  const save = (path: string, content: string) =>
    new Promise<void>((resolve, reject) => {
      queue.push({
        path,
        content,
        resolve: () => {
          disk.set(path, content)
          resolve()
        },
        reject,
      })
    })
  // Completes queued saves (newest first when asked to, to provoke reordering).
  const settle = async (order: 'fifo' | 'lifo' = 'fifo') => {
    while (queue.length) {
      const s = order === 'fifo' ? queue.shift()! : queue.pop()!
      s.resolve()
      await new Promise((r) => setImmediate(r))
    }
  }
  return { disk, queue, save, settle }
}

const tick = () => new Promise((r) => setImmediate(r))

test('fast tab switching loses nothing and never saves stale content last', async () => {
  const d = fakeDisk()
  const a = new Autosaver(d.save, { delay: 1_000_000 })
  // Typing in a.tex, switch to b.tex (flush a), type, switch back and keep typing.
  a.change('a.tex', 'a1')
  const f1 = a.flush('a.tex')
  a.change('b.tex', 'b1')
  const f2 = a.flush('b.tex')
  a.change('a.tex', 'a2') // while a1 is still in flight
  const f3 = a.flush('a.tex')
  a.change('a.tex', 'a3')
  const f4 = a.flushAll()
  await tick()
  // Only one save per file may be in flight: a2 waits for a1.
  assert.equal(d.queue.filter((s) => s.path === 'a.tex').length, 1)
  await d.settle('lifo')
  await Promise.all([f1, f2, f3, f4])
  await d.settle()
  assert.equal(d.disk.get('a.tex'), 'a3')
  assert.equal(d.disk.get('b.tex'), 'b1')
  assert.equal(a.isDirty(), false)
  assert.equal(a.status('a.tex'), 'saved')
})

test('debounce saves only the latest content', async () => {
  const d = fakeDisk()
  const a = new Autosaver(d.save, { delay: 5 })
  for (const c of ['x', 'xy', 'xyz']) a.change('m.tex', c)
  assert.equal(a.status('m.tex'), 'dirty')
  await new Promise((r) => setTimeout(r, 20))
  assert.deepEqual(
    d.queue.map((s) => s.content),
    ['xyz'],
  )
  await d.settle()
  assert.equal(d.disk.get('m.tex'), 'xyz')
})

test('a failed save keeps the content and retries', async () => {
  const d = fakeDisk()
  const statuses: string[] = []
  const a = new Autosaver(d.save, { delay: 1_000_000, retryDelay: 5, onStatus: (_, s) => statuses.push(s) })
  a.change('m.tex', 'keep me')
  const f = a.flush('m.tex')
  await tick()
  d.queue.shift()!.reject(new Error('network'))
  await f
  assert.equal(a.status('m.tex'), 'error')
  assert.equal(a.isDirty('m.tex'), true)
  await new Promise((r) => setTimeout(r, 20)) // retry fires
  await d.settle()
  assert.equal(d.disk.get('m.tex'), 'keep me')
  assert.equal(a.status('m.tex'), 'saved')
  assert.deepEqual(statuses, ['dirty', 'saving', 'error', 'saving', 'saved'])
})

test('rename carries pending content to the new path; drop discards it', async () => {
  const d = fakeDisk()
  const a = new Autosaver(d.save, { delay: 1_000_000 })
  a.change('old.tex', 'content')
  a.rename('old.tex', 'new.tex')
  a.change('gone.tex', 'x')
  a.drop('gone.tex')
  const f = a.flushAll()
  await tick()
  await d.settle()
  await f
  assert.equal(d.disk.get('new.tex'), 'content')
  assert.equal(d.disk.has('old.tex'), false)
  assert.equal(d.disk.has('gone.tex'), false)
})
