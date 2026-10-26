import { useCallback, useEffect, useState } from 'react'
import { api, type ChatGPTStatus } from './api'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import Dev from './Dev'
import Projects from './Projects'
import SettingsDialog from './Settings'
import { openSettings } from '@/lib/prefs'
import Welcome from './Welcome'
import CommandPalette from './CommandPalette'
import ShortcutsOverlay from './ShortcutsOverlay'
import { getOverlay, setOverlay, useCommands } from '@/lib/commands'
import { setTheme } from '@/lib/theme'
import { FolderOpen, Keyboard, Monitor, Moon, Settings, Sun } from 'lucide-react'
import Workspace from './Workspace'

export default function App() {
  const [status, setStatus] = useState<ChatGPTStatus | null>(null)
  const refresh = useCallback(() => {
    api<ChatGPTStatus>('/api/chatgpt/status')
      .then(setStatus)
      .catch(() => {})
  }, [])
  useEffect(refresh, [refresh])
  // Old /settings links open the dialog over the project list.
  useEffect(() => {
    if (window.location.pathname === '/settings') {
      history.replaceState(null, '', '/')
      openSettings('assistant')
    }
  }, [])

  // ⌘K anywhere, also from inputs and the editor.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setOverlay(getOverlay() === 'palette' ? null : 'palette')
      }
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [])

  const path = window.location.pathname
  useCommands(() => [
    ...(path !== '/' ? [{ id: 'nav:projects', group: 'Navigate', label: 'All projects', icon: FolderOpen, run: () => (window.location.href = '/') }] : []),
    { id: 'app:settings', group: 'General', label: 'Settings', icon: Settings, keywords: 'preferences account chatgpt', run: () => openSettings() },
    { id: 'app:shortcuts', group: 'General', label: 'Keyboard shortcuts', icon: Keyboard, shortcut: '?', run: () => setOverlay('shortcuts') },
    { id: 'theme:light', group: 'General', label: 'Light theme', icon: Sun, keywords: 'appearance', run: () => setTheme('light') },
    { id: 'theme:dark', group: 'General', label: 'Dark theme', icon: Moon, keywords: 'appearance', run: () => setTheme('dark') },
    { id: 'theme:system', group: 'General', label: 'System theme', icon: Monitor, keywords: 'appearance', run: () => setTheme('system') },
  ])
  const welcome = !!status && status.status.signed_in && status.status.plan_granted && !status.welcome_seen

  return (
    // After the first tooltip opens, neighbours open instantly (within the provider's 400 ms timeout).
    <TooltipProvider delay={500}>
      {path === '/dev' ? (
        <Dev status={status} refresh={refresh} />
      ) : path.startsWith('/p/') ? (
        <Workspace id={path.slice(3)} status={status} />
      ) : (
        <Projects status={status} />
      )}
      {status && <Welcome open={welcome} manageUsage={status.manage_usage} onClose={() => setStatus({ ...status, welcome_seen: true })} />}
      <SettingsDialog status={status} onChange={setStatus} />
      <CommandPalette />
      <ShortcutsOverlay />
      <Toaster position="bottom-right" />
    </TooltipProvider>
  )
}
