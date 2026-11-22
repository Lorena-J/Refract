import assert from 'node:assert/strict'
import { test } from 'node:test'
import { maxEntries, readDropped } from './dropfiles.ts'

const file = (name: string) => ({
  isFile: true,
  isDirectory: false,
  name,
  file: (ok: (f: File) => void) => ok(new File(['x'], name)),
})
const dir = (name: string, kids: unknown[]) => {
  let given = false
  return {
    isFile: false,
    isDirectory: true,
    name,
    // two calls: the children, then an empty batch
    createReader: () => ({
      readEntries: (ok: (e: unknown[]) => void) => {
        ok(given ? [] : kids)
        given = true
      },
    }),
  }
}

test('folders are walked with relative paths, files pass through', async () => {
  const zip = new File(['z'], 'a.zip')
  const tree = dir('thesis', [file('main.tex'), dir('figs', [file('a.png')])])
  const got = await readDropped([
    { kind: 'file', getAsFile: () => null, webkitGetAsEntry: () => tree as never },
    { kind: 'file', getAsFile: () => zip, webkitGetAsEntry: () => file('a.zip') as never },
    { kind: 'string', getAsFile: () => null },
  ])
  assert.deepEqual(got.files, [zip])
  assert.equal(got.folders[0].name, 'thesis')
  assert.deepEqual(
    got.folders[0].files.map((f) => f.path),
    ['thesis/main.tex', 'thesis/figs/a.png'],
  )
})

test('directories are read until an empty batch, in batches of 100', async () => {
  const kids = Array.from({ length: 250 }, (_, i) => file(`f${i}.tex`))
  let at = 0
  const big = {
    isFile: false,
    isDirectory: true,
    name: 'big',
    createReader: () => ({
      readEntries: (ok: (e: unknown[]) => void) => {
        ok(kids.slice(at, at + 100))
        at += 100
      },
    }),
  }
  const got = await readDropped([{ kind: 'file', getAsFile: () => null, webkitGetAsEntry: () => big as never }])
  assert.equal(got.folders[0].files.length, 250)
})

test('a folder that contains itself stops at the guard instead of looping', async () => {
  const loop: Record<string, unknown> = { isFile: false, isDirectory: true, name: 'loop' }
  loop.createReader = () => {
    let given = false
    return {
      readEntries: (ok: (e: unknown[]) => void) => {
        ok(given ? [] : [loop])
        given = true
      },
    }
  }
  const got = await readDropped([{ kind: 'file', getAsFile: () => null, webkitGetAsEntry: () => loop as never }])
  assert.deepEqual(got.folders, [{ name: 'loop', files: [], tooBig: true }])
  assert.ok(maxEntries > 5000)
})

test('a read error rejects', async () => {
  const bad = {
    isFile: false,
    isDirectory: true,
    name: 'bad',
    createReader: () => ({ readEntries: (_ok: unknown, fail: (e: unknown) => void) => fail(new Error('denied')) }),
  }
  await assert.rejects(
    readDropped([{ kind: 'file', getAsFile: () => null, webkitGetAsEntry: () => bad as never }]),
    /denied/,
  )
})
