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
import { getPrefs, openSettings, setPref, usePrefs, useSettingsSection, type Prefs } from '@/lib/prefs'
import { t } from '@/lib/i18n'
import { shortcutGroups } from '@/lib/shortcuts'
import { setTheme, useTheme, type Theme } from '@/lib/theme'
import PlanBadge from './PlanBadge'

type Props = { status: ChatGPTStatus | null; onChange: (s: ChatGPTStatus) => void }

// Settings as a modal over whatever page is open (Prism-style): sections on the left, rows on the right.
export default function SettingsDialog({ status, onChange }: Props) {
  const section = useSettingsSection()
  return (
    <Dialog open={section !== null} onOpenChange={(o) => !o && openSettings(null)}>
      <DialogContent className="h-[min(640px,calc(100dvh-2rem))] gap-0 overflow-hidden rounded-2xl p-0 sm:max-w-[860px]">
        <DialogTitle className="sr-only">{t('Settings')}</DialogTitle>
        <DialogDescription className="sr-only">{t('Appearance, editor and ChatGPT settings')}</DialogDescription>
        <SettingsPanel value={section ?? 'general'} onValueChange={(v) => openSettings(v)} className="h-full">
          <SettingsNav title={t('Settings')} className="bg-foreground/[0.025]">
            <SettingsNavItem value="general" icon={<SunMoon />}>
              {t('General')}
            </SettingsNavItem>
            <SettingsNavItem value="editor" icon={<Code2 />}>
              {t('Editor')}
            </SettingsNavItem>
            <SettingsNavItem value="assistant" icon={<Sparkles />} meta={status?.status.signed_in ? undefined : t('Off')}>
              {t('Assistant')}
            </SettingsNavItem>
            <SettingsNavItem value="shortcuts" icon={<Keyboard />}>
              {t('Shortcuts')}
            </SettingsNavItem>
          </SettingsNav>
          <General />
          <EditorPane />
          <AssistantPane status={status} onChange={onChange} />
          <SettingsPane value="shortcuts" title={t('Keyboard shortcuts')} description={t('Hold ⌘ (Ctrl) anywhere, or press ?, to see them over the page.')}>
            {shortcutGroups.map((g) => (
              <SettingsGroup key={g.title} title={g.title}>
                {g.items.map(([k, v]) => (
                  <SettingsRow key={k + v} label={v}>
                    <kbd className="rounded border bg-muted px-1.5 py-0.5 font-mono text-xs text-muted-foreground">{k}</kbd>
                  </SettingsRow>
                ))}
              </SettingsGroup>
            ))}
          </SettingsPane>
        </SettingsPanel>
      </DialogContent>
    </Dialog>
  )
}

function General() {
  const { theme } = useTheme()
  return (
    <SettingsPane value="general" title={t('General')}>
      <SettingsGroup>
        <SettingsRow label={t('Interface theme')} description={t('System follows your operating system.')}>
          <SettingsSegmented
            value={theme}
            onValueChange={(v) => setTheme(v as Theme)}
            options={[
              { value: 'light', label: t('Light'), icon: <Sun /> },
              { value: 'dark', label: t('Dark'), icon: <Moon /> },
              { value: 'system', label: t('System'), icon: <Monitor /> },
            ]}
          />
        </SettingsRow>
        <SettingsRow label={t('Language')} description={t('System follows your browser. Changing it reloads the page.')}>
          <SettingsSegmented
            value={getPrefs().language}
            onValueChange={(v) => {
              setPref('language', v as Prefs['language'])
              location.reload()
            }}
            options={[
              { value: 'system', label: t('System') },
              { value: 'en', label: 'English' }, // language names stay in their own language
              { value: 'es', label: 'Español' },
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
    <SettingsPane value="editor" title={t('Editor')} description={t('Stored in this browser.')}>
      <SettingsGroup>
        <SettingsRow label={t('Font size')} description={t('Text size in the editor.')}>
          <SettingsSegmented
            value={prefs.fontSize}
            onValueChange={(v) => setPref('fontSize', v as Prefs['fontSize'])}
            options={[
              { value: 'small', label: t('Small') },
              { value: 'normal', label: t('Normal') },
              { value: 'large', label: t('Large') },
            ]}
          />
        </SettingsRow>
        <SettingsRow label={t('Word wrap')} description={t('Wrap long lines instead of scrolling sideways.')}>
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
    <SettingsPane value="assistant" title={t('Assistant')} description={t('Refract uses your ChatGPT plan for AI requests.')}>
      <SettingsGroup title={t('ChatGPT account')}>
        {!status ? (
          <SettingsRow label={t('Loading…')} />
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
                    ? t('Signed in. Requests count against your ChatGPT plan.')
                    : describe({ kind: 'plan_not_granted', message: '' })
              }
            >
              {s.plan_granted && <PlanBadge manageUsage={status.manage_usage} />}
            </SettingsRow>
            <SettingsRow label={t('Credentials')} description={t('Replace them after running {cmd} again.', { cmd: 'refract login' })}>
              <Button size="sm" variant="secondary" disabled={busy} onClick={() => input.current?.click()}>
                <Upload /> {t('Replace…')}
              </Button>
              <Button size="sm" variant="ghost" disabled={busy} onClick={() => run(() => api('/api/chatgpt/signout', { method: 'POST' }), t('Signed out'))}>
                {t('Sign out')}
              </Button>
            </SettingsRow>
            <SettingsRow label={t('Usage')} description={t('Limits and remaining usage are managed by ChatGPT.')}>
              <Button size="sm" variant="ghost" render={<a href={status.manage_usage} target="_blank" rel="noreferrer" />} nativeButton={false}>
                {t('Manage usage')} <ExternalLink />
              </Button>
            </SettingsRow>
          </>
        ) : (
          <SettingsRow
            stacked
            label={t('Not connected')}
            description={
              <>
                {t('Sign-in happens on your own computer: run')} <code className="rounded bg-muted px-1 py-0.5 text-foreground">refract login</code>,{' '}
                {t('choose')} <em>Continue with ChatGPT</em>, {t('then upload the credentials file it writes.')}
              </>
            }
          >
            <Button size="sm" disabled={busy} onClick={() => input.current?.click()}>
              <Upload /> {t('Upload credentials…')}
            </Button>
          </SettingsRow>
        )}
      </SettingsGroup>
      <SettingsGroup title={t('Chat')}>
        <SettingsRow label={t('When collapsed')} description={t('Keep an input bar under the editor, or tuck the chat into a bubble.')}>
          <SettingsSegmented
            value={prefs.chatBubble ? 'bubble' : 'bar'}
            onValueChange={(v) => setPref('chatBubble', v === 'bubble')}
            options={[
              { value: 'bar', label: t('Input bar') },
              { value: 'bubble', label: t('Bubble') },
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
          if (f) run(async () => api<ChatGPTStatus>('/api/chatgpt/credentials', { method: 'POST', body: await f.text() }), t('Credentials imported'))
          e.target.value = ''
        }}
      />
    </SettingsPane>
  )
}
