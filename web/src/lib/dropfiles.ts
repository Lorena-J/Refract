// What was dropped on the project list: files, and folders read through the (non-standard but
// universal) FileSystemEntry API. Entries must be taken from the event synchronously.

export type DroppedFile = { file: File; path: string }
export type DroppedFolder = { name: string; files: DroppedFile[] }
export type Dropped = { files: File[]; folders: DroppedFolder[] }

type Entry = {
  isFile: boolean
  isDirectory: boolean
  name: string
  file?: (ok: (f: File) => void, fail: (e: unknown) => void) => void
  createReader?: () => { readEntries: (ok: (e: Entry[]) => void, fail: (e: unknown) => void) => void }
}

type Item = { kind: string; getAsFile(): File | null; webkitGetAsEntry?: () => Entry | null }

async function readAll(dir: Entry): Promise<Entry[]> {
  const reader = dir.createReader!()
  const all: Entry[] = []
  // readEntries returns batches: keep asking until it answers an empty one.
  for (;;) {
    const batch = await new Promise<Entry[]>((ok, fail) => reader.readEntries(ok, fail))
    if (batch.length === 0) return all
    all.push(...batch)
  }
}

async function walk(entry: Entry, prefix: string, out: DroppedFile[]) {
  const path = prefix + entry.name
  if (entry.isFile) {
    const file = await new Promise<File>((ok, fail) => entry.file!(ok, fail))
    out.push({ file, path })
  } else if (entry.isDirectory) {
    for (const child of await readAll(entry)) await walk(child, path + '/', out)
  }
}

export async function readDropped(items: Item[]): Promise<Dropped> {
  // Take every entry before the first await: the event's items are gone afterwards.
  const taken = items
    .filter((i) => i.kind === 'file')
    .map((i) => ({ entry: i.webkitGetAsEntry?.() ?? null, file: i.getAsFile() }))
  const result: Dropped = { files: [], folders: [] }
  for (const { entry, file } of taken) {
    if (entry?.isDirectory) {
      const files: DroppedFile[] = []
      await walk(entry, '', files)
      result.folders.push({ name: entry.name, files })
    } else if (file) {
      result.files.push(file)
    }
  }
  return result
}
