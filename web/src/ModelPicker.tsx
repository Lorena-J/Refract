import { useState } from 'react'
import { Check, ChevronDown, Search } from 'lucide-react'
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
import { score } from '@/lib/fuzzy'
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

// Lists longer than this get a search field (OpenRouter alone lists hundreds).
const SEARCH_FROM = 10

// Best matches first, by name, id and provider.
function search(models: Model[], query: string): Model[] {
  if (!query.trim()) return models
  return models
    .map((m) => [m, score(`${m.display_name} ${m.slug} ${m.provider_name ?? ''}`, query)] as const)
    .filter(([, s]) => s > 0)
    .sort((a, b) => b[1] - a[1])
    .map(([m]) => m)
}

// Models by provider, in the given order; one unnamed group when there is a single provider.
function groups(models: Model[]): [string, Model[]][] {
  const by = new Map<string, Model[]>()
  for (const m of models) by.set(m.provider_name ?? '', [...(by.get(m.provider_name ?? '') ?? []), m])
  return by.size > 1 ? [...by] : [['', models]]
}

// "GPT-6.1-Sol Medium ▾" in the composer, like Prism's: reasoning levels up front, models in a submenu.
export default function ModelPicker({ models, model, effort, compact }: { models: Model[]; model?: Model; effort: string; compact?: boolean }) {
  const [query, setQuery] = useState('')
  if (!model) return null
  const shown = search(models, query)
  return (
    <DropdownMenu onOpenChange={(open) => !open && setQuery('')}>
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
            {models.length > SEARCH_FROM && (
              // Typing stays in the field: the menu would read letters as typeahead.
              <div className="sticky -top-1 z-10 -mx-1 -mt-1 mb-1 border-b bg-popover px-3 py-2" onKeyDown={(e) => !/^(Escape|ArrowUp|ArrowDown|Enter|Tab)$/.test(e.key) && e.stopPropagation()}>
                <label className="flex items-center gap-2 text-sm">
                  <Search className="size-3.5 shrink-0 text-muted-foreground" />
                  <input
                    autoFocus
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    placeholder={t('Search models')}
                    aria-label={t('Search models')}
                    className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-muted-foreground"
                  />
                </label>
              </div>
            )}
            {shown.length === 0 && <p className="px-2 py-3 text-center text-sm text-muted-foreground">{t('No models match')}</p>}
            {groups(shown).map(([name, ms], i) => (
              <DropdownMenuGroup key={name}>
                {i > 0 && <DropdownMenuSeparator />}
                {name && <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">{name}</DropdownMenuLabel>}
                {ms.map((m) => (
                  <DropdownMenuItem key={m.slug} onClick={() => setPref('model', m.slug)} className="items-start">
                    <span className="min-w-0 flex-1">
                      <span className="block">{m.display_name}</span>
                      {m.description && <span className="block text-xs text-muted-foreground">{m.description}</span>}
                    </span>
                    <Check className={cn('mt-0.5', m.slug !== model.slug && 'invisible')} />
                  </DropdownMenuItem>
                ))}
              </DropdownMenuGroup>
            ))}
          </DropdownMenuSubContent>
        </DropdownMenuSub>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
