// Match score for the command palette: 0 = no match, higher = better.
// Every query character must appear in order; contiguous runs, word starts and
// an early first match score higher.
export function score(text: string, query: string): number {
  const t = text.toLowerCase()
  const q = query.toLowerCase().trim()
  if (!q) return 1
  const exact = t.indexOf(q)
  if (exact >= 0) return 1000 - exact + (exact === 0 || /[\s/._-]/.test(t[exact - 1]) ? 500 : 0)
  let s = 0
  let i = 0
  let prev = -2
  for (const c of q) {
    if (c === ' ') continue
    const j = t.indexOf(c, i)
    if (j < 0) return 0
    s += j === prev + 1 ? 10 : 1
    if (j === 0 || /[\s/._-]/.test(t[j - 1])) s += 8
    prev = j
    i = j + 1
  }
  return s
}
