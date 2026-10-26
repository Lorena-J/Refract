import { useCallback, useEffect, useState } from 'react'
import { api, type ChatGPTStatus } from './api'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import Dev from './Dev'
import Projects from './Projects'
import SettingsDialog from './Settings'
import { openSettings } from '@/lib/prefs'
import Welcome from './Welcome'
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

  const path = window.location.pathname
  const welcome = !!status && status.status.signed_in && status.status.plan_granted && !status.welcome_seen

  return (
    // After the first tooltip opens, neighbours open instantly (skipDelayDuration).
    <TooltipProvider delayDuration={500} skipDelayDuration={400}>
      {path === '/dev' ? (
        <Dev status={status} refresh={refresh} />
      ) : path.startsWith('/p/') ? (
        <Workspace id={path.slice(3)} status={status} />
      ) : (
        <Projects status={status} />
      )}
      {status && <Welcome open={welcome} manageUsage={status.manage_usage} onClose={() => setStatus({ ...status, welcome_seen: true })} />}
      <SettingsDialog status={status} onChange={setStatus} />
      <Toaster position="bottom-right" />
    </TooltipProvider>
  )
}
