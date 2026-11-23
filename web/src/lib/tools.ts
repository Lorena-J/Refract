// The external programs the server didn't find at startup (REFRACT_*_BIN, the
// bundled folder, the PATH), from /api/me. Set once before the app renders
// (main.tsx); unknown names are ignored so a newer server can't break this one.
const known = ['git', 'tectonic', 'synctex', 'rg'] as const
export type ToolName = (typeof known)[number]

let missing: ToolName[] = []

export function setMissingTools(names: unknown) {
  missing = Array.isArray(names) ? known.filter((k) => names.includes(k)) : []
}

export function getMissingTools(): ToolName[] {
  return missing
}
