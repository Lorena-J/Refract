// Compact relative time like "now", "5m", "3h", "2d", "2mo", "1y".
export function ago(iso: string): string {
  const s = (Date.now() - new Date(iso).getTime()) / 1000
  if (s < 60) return 'now'
  const steps: [number, string][] = [
    [60, 'm'],
    [24, 'h'],
    [30, 'd'],
    [12, 'mo'],
  ]
  let v = s / 60
  let unit = 'm'
  for (const [size, u] of steps.slice(1)) {
    if (v < size) break
    v /= size
    unit = u
  }
  if (unit === 'mo' && v >= 12) return `${Math.floor(v / 12)}y`
  return `${Math.floor(v)}${unit}`
}
