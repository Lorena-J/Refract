import { useCallback, useEffect, useState } from 'react'
import { api, type ChatGPTStatus } from './api'
import Dev from './Dev'
import Projects from './Projects'
import Settings from './Settings'
import Welcome from './Welcome'
import Workspace from './Workspace'

export default function App() {
  const [status, setStatus] = useState<ChatGPTStatus | null>(null)
  const refresh = useCallback(() => {
    api<ChatGPTStatus>('/api/chatgpt/status').then(setStatus).catch(() => {})
  }, [])
  useEffect(refresh, [refresh])

  const path = window.location.pathname
  const welcome = status && status.status.signed_in && status.status.plan_granted && !status.welcome_seen

  return (
    <>
      {path === '/settings' ? (
        <Settings status={status} onChange={setStatus} />
      ) : path === '/dev' ? (
        <Dev status={status} refresh={refresh} />
      ) : path.startsWith('/p/') ? (
        <Workspace id={path.slice(3)} />
      ) : (
        <Projects />
      )}
      {welcome && <Welcome manageUsage={status.manage_usage} onClose={() => setStatus({ ...status, welcome_seen: true })} />}
    </>
  )
}
