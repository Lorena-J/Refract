import { useRef, useState } from 'react'
import { CheckCircle2, Code2, ExternalLink, Keyboard, Monitor, Moon, Sparkles, Sun, SunMoon, Upload } from 'lucide-react'
import { toast } from 'sonner'
import { api, describe, RequestError, type ChatGPTStatus } from './api'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import {
  SettingsGroup,
  SettingsNav,
  SettingsNavItem,
  SettingsPane,
  SettingsPanel,
  SettingsRow,
  SettingsSegmented,
  SettingsSwitch,
} from '@/components/ui/chatcn/settings-panel'
import { openSettings, setPref, usePrefs, useSettingsSection, type Prefs } from '@/lib/prefs'
import { shortcuts } from '@/lib/shortcuts'
import { setTheme, useTheme, type Theme } from '@/lib/theme'
import PlanBadge from './PlanBadge'

type Props = { status: ChatGPTStatus | null; onChange: (s: ChatGPTStatus) => void }

// Settings as a modal over whatever page is open (Prism-style): sections on the left, rows on the right.
export default function SettingsDialog({ status, onChange }: Props) {
  const section = useSettingsSection()
  return (
    <Dialog open={section !== null} onOpenChange={(o) => !o && openSettings(null)}>
      <DialogContent className="h-[min(640px,calc(100dvh-2rem))] gap-0 overflow-hidden rounded-2xl p-0 sm:max-w-[860px]">
        <DialogTitle className="sr-only">Settings</DialogTitle>
        <DialogDescription className="sr-only">Appearance, editor and ChatGPT settings</DialogDescription>
        <SettingsPanel value={section ?? 'general'} onValueChange={(v) => openSettings(v)} className="h-full">
          <SettingsNav title="Settings" className="bg-foreground/[0.025]">
            <SettingsNavItem value="general" icon={<SunMoon />}>
              General
            </SettingsNavItem>
            <SettingsNavItem value="editor" icon={<Code2 />}>
              Editor
            </SettingsNavItem>
            <SettingsNavItem value="assistant" icon={<Sparkles />} meta={status?.status.signed_in ? undefined : 'Off'}>
              Assistant
            </SettingsNavItem>
            <SettingsNavItem value="shortcuts" icon={<Keyboard />}>
              Shortcuts
            </SettingsNavItem>
          </SettingsNav>
          <General />
          <EditorPane />
          <AssistantPane status={status} onChange={onChange} />
          <SettingsPane value="shortcuts" title="Keyboard shortcuts">
            <SettingsGroup>
              {shortcuts.map(([k, v]) => (
                <SettingsRow key={k} label={v}>
                  <kbd className="rounded border bg-muted px-1.5 py-0.5 font-mono text-xs text-muted-foreground">{k}</kbd>
                </SettingsRow>
              ))}
            </SettingsGroup>
          </SettingsPane>
        </SettingsPanel>
      </DialogContent>
    </Dialog>
  )
}

function General() {
  const { theme } = useTheme()
  return (
    <SettingsPane value="general" title="General">
      <SettingsGroup>
        <SettingsRow label="Interface theme" description="System follows your operating system.">
          <SettingsSegmented
            value={theme}
            onValueChange={(v) => setTheme(v as Theme)}
            options={[
              { value: 'light', label: 'Light', icon: <Sun /> },
              { value: 'dark', label: 'Dark', icon: <Moon /> },
              { value: 'system', label: 'System', icon: <Monitor /> },
            ]}
          />
        </SettingsRow>
      </SettingsGroup>
    </SettingsPane>
  )
}

function EditorPane() {
  const prefs = usePrefs()
  return (
    <SettingsPane value="editor" title="Editor" description="Stored in this browser.">
      <SettingsGroup>
        <SettingsRow label="Font size" description="Text size in the editor.">
          <SettingsSegmented
            value={prefs.fontSize}
            onValueChange={(v) => setPref('fontSize', v as Prefs['fontSize'])}
            options={[
              { value: 'small', label: 'Small' },
              { value: 'normal', label: 'Normal' },
              { value: 'large', label: 'Large' },
            ]}
          />
        </SettingsRow>
        <SettingsRow label="Word wrap" description="Wrap long lines instead of scrolling sideways.">
          <SettingsSwitch checked={prefs.wrap} onCheckedChange={(v) => setPref('wrap', v)} />
        </SettingsRow>
      </SettingsGroup>
    </SettingsPane>
  )
}

function AssistantPane({ status, onChange }: Props) {
  const prefs = usePrefs()
  const [busy, setBusy] = useState(false)
  const input = useRef<HTMLInputElement>(null)

  async function run(fn: () => Promise<ChatGPTStatus>, ok: string) {
    setBusy(true)
    try {
      onChange(await fn())
      toast.success(ok)
    } catch (e) {
      toast.error(e instanceof RequestError ? describe(e.error) : String(e))
    } finally {
      setBusy(false)
    }
  }

  const s = status?.status
  return (
    <SettingsPane value="assistant" title="Assistant" description="Refract uses your ChatGPT plan for AI requests.">
      <SettingsGroup title="ChatGPT account">
        {!status ? (
          <SettingsRow label="Loading…" />
        ) : s?.signed_in ? (
          <>
            <SettingsRow
              label={
                <span className="flex items-center gap-2">
                  <CheckCircle2 className="size-4 text-success" /> {s.email}
                </span>
              }
              description={
                s.limit_reached
                  ? describe({ kind: 'usage_limit', message: '' })
                  : s.plan_granted
                    ? 'Signed in. Requests count against your ChatGPT plan.'
                    : describe({ kind: 'plan_not_granted', message: '' })
              }
            >
              {s.plan_granted && <PlanBadge manageUsage={status.manage_usage} />}
            </SettingsRow>
            <SettingsRow label="Credentials" description="Replace them after running refract login again.">
              <Button size="sm" variant="secondary" disabled={busy} onClick={() => input.current?.click()}>
                <Upload /> Replace…
              </Button>
              <Button size="sm" variant="ghost" disabled={busy} onClick={() => run(() => api('/api/chatgpt/signout', { method: 'POST' }), 'Signed out')}>
                Sign out
              </Button>
            </SettingsRow>
            <SettingsRow label="Usage" description="Limits and remaining usage are managed by ChatGPT.">
              <Button size="sm" variant="ghost" asChild>
                <a href={status.manage_usage} target="_blank" rel="noreferrer">
                  Manage usage <ExternalLink />
                </a>
              </Button>
            </SettingsRow>
          </>
        ) : (
          <SettingsRow
            stacked
            label="Not connected"
            description={
              <>
                Sign-in happens on your own computer: run <code className="rounded bg-muted px-1 py-0.5 text-foreground">refract login</code>, choose{' '}
                <em>Continue with ChatGPT</em>, then upload the credentials file it writes.
              </>
            }
          >
            <Button size="sm" disabled={busy} onClick={() => input.current?.click()}>
              <Upload /> Upload credentials…
            </Button>
          </SettingsRow>
        )}
      </SettingsGroup>
      <SettingsGroup title="Chat">
        <SettingsRow label="When collapsed" description="Keep an input bar under the editor, or tuck the chat into a bubble.">
          <SettingsSegmented
            value={prefs.chatBubble ? 'bubble' : 'bar'}
            onValueChange={(v) => setPref('chatBubble', v === 'bubble')}
            options={[
              { value: 'bar', label: 'Input bar' },
              { value: 'bubble', label: 'Bubble' },
            ]}
          />
        </SettingsRow>
      </SettingsGroup>
      <input
        ref={input}
        type="file"
        accept=".json,application/json"
        hidden
        onChange={(e) => {
          const f = e.target.files?.[0]
          if (f) run(async () => api<ChatGPTStatus>('/api/chatgpt/credentials', { method: 'POST', body: await f.text() }), 'Credentials imported')
          e.target.value = ''
        }}
      />
    </SettingsPane>
  )
}
