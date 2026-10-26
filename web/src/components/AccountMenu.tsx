import { Check, ChevronsUpDown, ExternalLink, Monitor, Moon, Settings, Sun } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { ChatGPTStatus } from '@/api'
import { openSettings } from '@/lib/prefs'
import { setTheme, useTheme, type Theme } from '@/lib/theme'
import { cn } from '@/lib/utils'

const themes: { value: Theme; label: string; icon: typeof Sun }[] = [
  { value: 'light', label: 'Light', icon: Sun },
  { value: 'dark', label: 'Dark', icon: Moon },
  { value: 'system', label: 'System', icon: Monitor },
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
              {email ?? 'Not signed in'} <ChevronsUpDown className="size-3 shrink-0 text-muted-foreground" />
            </span>
            <span className="block truncate text-xs text-muted-foreground">
              {status?.status.plan_granted ? 'Using ChatGPT plan' : 'ChatGPT not connected'}
            </span>
          </span>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" side="top" className="w-60">
        <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">Theme</DropdownMenuLabel>
        {themes.map((t) => (
          <DropdownMenuItem key={t.value} onSelect={() => setTheme(t.value)}>
            <t.icon /> {t.label}
            <Check className={cn('ml-auto', theme !== t.value && 'invisible')} />
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => openSettings()}>
          <Settings /> Settings
        </DropdownMenuItem>
        {status && (
          <DropdownMenuItem asChild>
            <a href={status.manage_usage} target="_blank" rel="noreferrer">
              <ExternalLink /> Manage ChatGPT usage
            </a>
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
