import { useEffect, useRef, useSyncExternalStore } from 'react'
import type { LucideIcon } from 'lucide-react'

// Commands for the ⌘K palette. Pages and components register providers; the
// palette asks them for commands each time the query changes, so commands can
// depend on the current state (the editor selection) and on the query itself.
export type Command = {
  id: string
  group: string
  label: string
  run: () => void
  icon?: LucideIcon
  hint?: string // muted text after the label (a folder, a time)
  shortcut?: string
  keywords?: string // extra words to match
  pinned?: boolean // shown first whatever the query (e.g. "Ask: <query>")
}
export type Provider = (query: string) => Command[]

const providers = new Set<{ current: Provider }>()

/** Registers commands while the component is mounted; the latest closure is always used. */
export function useCommands(provider: Provider) {
  const ref = useRef(provider)
  ref.current = provider
  useEffect(() => {
    providers.add(ref)
    return () => {
      providers.delete(ref)
    }
  }, [])
}

export function collect(query: string): Command[] {
  return [...providers].flatMap((p) => p.current(query))
}

// Small global UI state: palette and shortcuts overlay.
type Overlay = 'palette' | 'shortcuts' | null
let overlay: Overlay = null
const listeners = new Set<() => void>()
export function setOverlay(o: Overlay) {
  overlay = o
  listeners.forEach((l) => l())
}
export const getOverlay = () => overlay
export function useOverlay(): Overlay {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => overlay,
  )
}
