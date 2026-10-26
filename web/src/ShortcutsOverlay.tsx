import { useEffect } from 'react'
import { getOverlay, setOverlay, useOverlay } from '@/lib/commands'
import { isMac, shortcutGroups } from '@/lib/shortcuts'

const HOLD_MS = 700

const typing = (t: EventTarget | null) => t instanceof HTMLElement && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))

// Keyboard shortcuts over the page: shown while ⌘ (Ctrl) is held alone, or toggled with "?".
// It never takes clicks, so holding ⌘ to ⌘-click the PDF still works; moving the
// pointer while holding cancels it (that is aiming, not asking for help).
export default function ShortcutsOverlay() {
  const shown = useOverlay() === 'shortcuts'

  useEffect(() => {
    const modKey = isMac ? 'Meta' : 'Control'
    let timer = 0
    let held = false
    const hide = () => {
      clearTimeout(timer)
      if (getOverlay() === 'shortcuts') setOverlay(null)
      held = false
    }
    const down = (e: KeyboardEvent) => {
      if (e.key === modKey) {
        if (e.repeat) return
        clearTimeout(timer)
        timer = window.setTimeout(() => {
          if (getOverlay()) return
          held = true
          setOverlay('shortcuts')
        }, HOLD_MS)
        return
      }
      clearTimeout(timer)
      if (getOverlay() === 'shortcuts') {
        // Any other key closes it; a held-⌘ shortcut still runs.
        if (e.key === 'Escape' || e.key === '?') e.preventDefault()
        hide()
        return
      }
      if (e.key === '?' && !e.metaKey && !e.ctrlKey && !typing(e.target) && !getOverlay()) {
        e.preventDefault()
        setOverlay('shortcuts')
      }
    }
    const up = (e: KeyboardEvent) => {
      if (e.key !== modKey) return
      clearTimeout(timer)
      if (held) hide()
    }
    const cancelHold = () => {
      clearTimeout(timer)
      if (held) hide()
    }
    const pointer = () => {
      clearTimeout(timer)
      if (getOverlay() === 'shortcuts') hide()
    }
    window.addEventListener('keydown', down, true)
    window.addEventListener('keyup', up, true)
    window.addEventListener('pointermove', cancelHold, true)
    window.addEventListener('pointerdown', pointer, true)
    window.addEventListener('blur', hide)
    document.addEventListener('visibilitychange', hide)
    return () => {
      clearTimeout(timer)
      window.removeEventListener('keydown', down, true)
      window.removeEventListener('keyup', up, true)
      window.removeEventListener('pointermove', cancelHold, true)
      window.removeEventListener('pointerdown', pointer, true)
      window.removeEventListener('blur', hide)
      document.removeEventListener('visibilitychange', hide)
    }
  }, [])

  if (!shown) return null
  return (
    <div
      className="pointer-events-none fixed inset-0 z-50 flex items-center justify-center bg-black/20 p-4 transition-opacity duration-150 ease-out starting:opacity-0"
      role="dialog"
      aria-label="Keyboard shortcuts"
    >
      <div className="w-full max-w-2xl rounded-2xl border bg-popover/95 p-6 text-popover-foreground shadow-2xl backdrop-blur-md transition-[opacity,scale] duration-150 ease-out starting:scale-[0.98] starting:opacity-0">
        <div className="mb-4 flex items-baseline justify-between">
          <h2 className="text-sm font-semibold">Keyboard shortcuts</h2>
          <span className="text-xs text-muted-foreground">Release to close</span>
        </div>
        <div className="grid gap-x-10 gap-y-5 sm:grid-cols-2">
          {shortcutGroups.map((g) => (
            <section key={g.title}>
              <h3 className="mb-2 text-xs font-medium text-muted-foreground">{g.title}</h3>
              <dl className="space-y-1.5">
                {g.items.map(([k, v]) => (
                  <div key={k + v} className="flex items-center justify-between gap-4 text-sm">
                    <dt className="text-foreground/90">{v}</dt>
                    <dd>
                      <kbd className="rounded-md border bg-muted px-1.5 py-0.5 font-mono text-xs text-muted-foreground">{k}</kbd>
                    </dd>
                  </div>
                ))}
              </dl>
            </section>
          ))}
        </div>
      </div>
    </div>
  )
}
