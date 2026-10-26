import { Check, ChevronDown } from 'lucide-react'
import type { Model } from './api'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { setPref } from '@/lib/prefs'
import { cn } from '@/lib/utils'
import { t } from '@/lib/i18n'

// Effort names; unknown ids (from the server) show as is.
const effortName: Record<string, string> = {
  none: t('None'),
  minimal: t('Minimal'),
  low: t('Low'),
  medium: t('Medium'),
  high: t('High'),
  xhigh: t('Extra high'),
  max: t('Max'),
}
const effortLabel = (e: string) => effortName[e] ?? e

// "GPT-6.1-Sol Medium ▾" in the composer, like Prism's: reasoning levels up front, models in a submenu.
export default function ModelPicker({ models, model, effort, compact }: { models: Model[]; model?: Model; effort: string; compact?: boolean }) {
  if (!model) return null
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="sm"
            className="h-7 min-w-0 gap-1 px-2 text-xs font-normal text-muted-foreground"
            aria-label={t('Model and reasoning')}
          />
        }
      >
        <span className="truncate text-foreground">{model.display_name}</span>
        {effort && !compact && <span className="truncate">{effortLabel(effort)}</span>}
        <ChevronDown className="size-3 opacity-60" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" side="top" className="w-64">
        {model.efforts.length > 0 && (
          <>
            <DropdownMenuGroup>
              <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">{t('Reasoning')}</DropdownMenuLabel>
              {model.efforts.map((e) => (
                <DropdownMenuItem key={e.effort} onClick={() => setPref('effort', e.effort)} className="items-start">
                  <span className="min-w-0 flex-1">
                    <span className="block">
                      {effortLabel(e.effort)}
                      {e.effort === model.default_effort && <span className="ml-1.5 text-xs text-muted-foreground">{t('default')}</span>}
                    </span>
                    {e.description && <span className="block text-xs text-muted-foreground">{e.description}</span>}
                  </span>
                  <Check className={cn('mt-0.5', e.effort !== effort && 'invisible')} />
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
          </>
        )}
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <span className="text-muted-foreground">{t('Model')}</span>
            <span className="ml-auto truncate pl-3">{model.display_name}</span>
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="max-h-[min(70vh,520px)] w-72">
            {models.map((m) => (
              <DropdownMenuItem key={m.slug} onClick={() => setPref('model', m.slug)} className="items-start">
                <span className="min-w-0 flex-1">
                  <span className="block">{m.display_name}</span>
                  {m.description && <span className="block text-xs text-muted-foreground">{m.description}</span>}
                </span>
                <Check className={cn('mt-0.5', m.slug !== model.slug && 'invisible')} />
              </DropdownMenuItem>
            ))}
          </DropdownMenuSubContent>
        </DropdownMenuSub>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
