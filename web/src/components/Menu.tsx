import type { ReactElement } from 'react'
import type { LucideIcon } from 'lucide-react'
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuSeparator, ContextMenuShortcut, ContextMenuTrigger } from '@/components/ui/context-menu'
import { DropdownMenuItem, DropdownMenuSeparator, DropdownMenuShortcut } from '@/components/ui/dropdown-menu'

// One description of a menu, rendered as a dropdown ("⋯") or a right-click menu.
export type MenuEntry = { label: string; icon?: LucideIcon; onSelect: () => void; shortcut?: string; destructive?: boolean; disabled?: boolean } | '-'

const clean = (items: MenuEntry[]) => items.filter((e, i, all) => e !== '-' || (i > 0 && i < all.length - 1 && all[i - 1] !== '-'))

export function DropdownItems({ items }: { items: MenuEntry[] }) {
  return clean(items).map((e, i) =>
    e === '-' ? (
      <DropdownMenuSeparator key={i} />
    ) : (
      <DropdownMenuItem key={i} onClick={e.onSelect} disabled={e.disabled} variant={e.destructive ? 'destructive' : 'default'}>
        {e.icon && <e.icon />} {e.label}
        {e.shortcut && <DropdownMenuShortcut>{e.shortcut}</DropdownMenuShortcut>}
      </DropdownMenuItem>
    ),
  )
}

/** Wraps children so a right click opens `items` (a function: built when the menu opens). */
export function WithContextMenu({ items, children, className }: { items: () => MenuEntry[]; children: ReactElement | ReactElement[]; className?: string }) {
  return (
    <ContextMenu>
      <ContextMenuTrigger className={className}>{children}</ContextMenuTrigger>
      <ContextMenuContent className="w-max max-w-80 min-w-56">
        <ContextItems items={items} />
      </ContextMenuContent>
    </ContextMenu>
  )
}

// Rendered only while the menu is open, so items reflect the state at the right click.
function ContextItems({ items }: { items: () => MenuEntry[] }) {
  return clean(items()).map((e, i) =>
    e === '-' ? (
      <ContextMenuSeparator key={i} />
    ) : (
      <ContextMenuItem key={i} onClick={e.onSelect} disabled={e.disabled} variant={e.destructive ? 'destructive' : 'default'}>
        {e.icon && <e.icon />} {e.label}
        {e.shortcut && <ContextMenuShortcut>{e.shortcut}</ContextMenuShortcut>}
      </ContextMenuItem>
    ),
  )
}
