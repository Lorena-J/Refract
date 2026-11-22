import assert from 'node:assert/strict'
import { test } from 'node:test'
import { readDropped } from './dropfiles.ts'

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
