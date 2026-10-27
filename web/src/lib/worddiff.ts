// Word-level diff for the quick-action preview: which parts of the selection
// go away and what comes in, in order.
export type Op = { op: 'eq' | 'del' | 'ins'; text: string }

// Words, runs of whitespace, and single punctuation characters (so "\textbf{x}"
// splits into "\", "textbf", "{", "x", "}").
const token = /\s+|[\p{L}\p{N}_]+|[^\s\p{L}\p{N}_]/gu
const tokens = (s: string) => s.match(token) ?? []

// The LCS is O(n·m): past this many cells, show a plain replace instead.
const MAX_CELLS = 2_000_000

export function wordDiff(a: string, b: string): Op[] {
  const x = tokens(a)
  const y = tokens(b)
  if (x.length * y.length > MAX_CELLS) return merge([{ op: 'del', text: a }, { op: 'ins', text: b }])
  // lcs[i][j]: length of the LCS of x[i:] and y[j:].
  const w = y.length + 1
  const lcs = new Uint32Array((x.length + 1) * w)
  for (let i = x.length - 1; i >= 0; i--) {
    for (let j = y.length - 1; j >= 0; j--) {
      lcs[i * w + j] = x[i] === y[j] ? lcs[(i + 1) * w + j + 1] + 1 : Math.max(lcs[(i + 1) * w + j], lcs[i * w + j + 1])
    }
  }
  const ops: Op[] = []
  let i = 0
  let j = 0
  while (i < x.length || j < y.length) {
    if (i < x.length && j < y.length && x[i] === y[j]) {
      ops.push({ op: 'eq', text: x[i++] })
      j++
    } else if (j < y.length && (i === x.length || lcs[i * w + j + 1] >= lcs[(i + 1) * w + j])) ops.push({ op: 'ins', text: y[j++] })
    else ops.push({ op: 'del', text: x[i++] })
  }
  return merge(absorbSpaces(merge(ops)))
}

// A lone space kept between two changes reads as noise ("~~a~~ ~~b~~" for one
// edit): fold it into the change on both sides.
function absorbSpaces(ops: Op[]): Op[] {
  const out: Op[] = []
  ops.forEach((o, k) => {
    const prev = ops[k - 1]
    const next = ops[k + 1]
    if (o.op === 'eq' && /^\s+$/.test(o.text) && prev && next && prev.op !== 'eq' && next.op !== 'eq') {
      out.push({ op: 'del', text: o.text }, { op: 'ins', text: o.text })
    } else out.push(o)
  })
  return out
}

// Join neighbours of the same kind; inside each changed run, deletions first.
function merge(ops: Op[]): Op[] {
  const out: Op[] = []
  let del = ''
  let ins = ''
  const flush = () => {
    if (del) out.push({ op: 'del', text: del })
    if (ins) out.push({ op: 'ins', text: ins })
    del = ins = ''
  }
  for (const o of ops) {
    if (o.op === 'del') del += o.text
    else if (o.op === 'ins') ins += o.text
    else {
      flush()
      const last = out[out.length - 1]
      if (last?.op === 'eq') last.text += o.text
      else out.push({ ...o })
    }
  }
  flush()
  return out.filter((o) => o.text)
}
