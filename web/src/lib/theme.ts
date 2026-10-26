import { useSyncExternalStore } from 'react'

export type Theme = 'light' | 'dark' | 'system'

const KEY = 'refract-theme'
const listeners = new Set<() => void>()
const media = window.matchMedia('(prefers-color-scheme: dark)')

function stored(): Theme {
  try {
    const v = localStorage.getItem(KEY)
    return v === 'light' || v === 'dark' ? v : 'system'
  } catch {
    return 'system'
  }
}

let theme = stored()

function apply() {
  const dark = theme === 'dark' || (theme === 'system' && media.matches)
  document.documentElement.classList.toggle('dark', dark)
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
}
apply()
media.addEventListener('change', () => {
  apply()
  listeners.forEach((l) => l())
})

export function setTheme(t: Theme) {
  theme = t
  try {
    localStorage.setItem(KEY, t)
  } catch {
    /* private mode: per-session only */
  }
  apply()
  listeners.forEach((l) => l())
}

/** The chosen theme and whether dark is in effect. */
export function useTheme(): { theme: Theme; dark: boolean } {
  const t = useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => theme,
  )
  const dark = useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => document.documentElement.classList.contains('dark'),
  )
  return { theme: t, dark }
}
