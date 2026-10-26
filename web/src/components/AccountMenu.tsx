import { Check, ChevronsUpDown, ExternalLink, Monitor, Moon, Settings, Sun } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { ChatGPTStatus } from '@/api'
import { openSettings } from '@/lib/prefs'
import { setTheme, useTheme, type Theme } from '@/lib/theme'
import { cn } from '@/lib/utils'
import { t } from '@/lib/i18n'

const themes: { value: Theme; label: string; icon: typeof Sun }[] = [
  { value: 'light', label: t('Light'), icon: Sun },
  { value: 'dark', label: t('Dark'), icon: Moon },
  { value: 'system', label: t('System'), icon: Monitor },
]

// Bottom-left account chip: ChatGPT identity, settings, theme.
export default function AccountMenu({ status, compact }: { status: ChatGPTStatus | null; compact?: boolean }) {
  const { theme } = useTheme()
  const email = status?.status.signed_in ? status.status.email : undefined
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex min-w-0 items-center gap-2.5 rounded-lg px-2 py-1.5 text-left outline-none transition-colors duration-150 hover:bg-foreground/[0.05] focus-visible:ring-2 focus-visible:ring-ring/50">
        <span className="flex size-7 shrink-0 items-center justify-center rounded-full border bg-card text-xs font-semibold uppercase shadow-xs">
          {email?.[0] ?? 'R'}
        </span>
        {!compact && (
          <span className="min-w-0">
            <span className="flex items-center gap-1 truncate text-sm font-medium">
              {email ?? t('Not signed in')} <ChevronsUpDown className="size-3 shrink-0 text-muted-foreground" />
            </span>
            <span className="block truncate text-xs text-muted-foreground">
              {status?.status.plan_granted ? t('Using ChatGPT plan') : t('ChatGPT not connected')}
            </span>
          </span>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" side="top" className="w-60">
        <DropdownMenuGroup>
          <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">{t('Theme')}</DropdownMenuLabel>
          {themes.map((opt) => (
            <DropdownMenuItem key={opt.value} onClick={() => setTheme(opt.value)}>
              <opt.icon /> {opt.label}
              <Check className={cn('ml-auto', theme !== opt.value && 'invisible')} />
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => openSettings()}>
          <Settings /> {t('Settings')}
        </DropdownMenuItem>
        {status && (
          <DropdownMenuItem render={<a href={status.manage_usage} target="_blank" rel="noreferrer" />}>
            <ExternalLink /> {t('Manage ChatGPT usage')}
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
