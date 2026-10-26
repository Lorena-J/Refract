"use client"

import type * as React from "react"
import { useConversation } from "@/components/ui/chatcn/conversation"
import { ArrowDownIcon } from "@/lib/chatcn-icons"
import { t, tn } from '@/lib/i18n'
import { cn } from "@/lib/utils"

interface ScrollToBottomProps extends Omit<React.ComponentProps<"button">, "children"> {
  /**
   * When the pill shows. `far` waits until the reader is more than one viewport up, or until a
   * new message lands below them. `away` shows as soon as they leave the bottom. Defaults to `far`.
   */
  when?: "far" | "away"
  /** Show the unread count next to the arrow. Defaults to true. */
  showUnread?: boolean
  /** Force the pill on or off, for example to keep it hidden while a dialog is open. */
  visible?: boolean
}

/**
 * Jump-to-latest pill. Floats at the bottom center of the nearest `Conversation`, fades in with
 * 4 px of travel over 150 ms, and re-engages stick-to-bottom when pressed.
 */
function ScrollToBottom({
  when = "far",
  showUnread = true,
  visible,
  className,
  onClick,
  ...props
}: ScrollToBottomProps) {
  const ctx = useConversation()
  if (!ctx) throw new Error("ScrollToBottom must be used inside Conversation.")
  const { isAtBottom, isFar, unread, scrollToBottom } = ctx
  const show = visible ?? (when === "away" ? !isAtBottom : isFar || (!isAtBottom && unread > 0))
  const count = showUnread && unread > 0 ? unread : 0
  const label = count ? tn(count, '{n} new message, scroll to latest', '{n} new messages, scroll to latest') : t('Scroll to latest')

  return (
    <button
      type="button"
      aria-label={label}
      aria-hidden={show ? undefined : true}
      tabIndex={show ? undefined : -1}
      data-slot="scroll-to-bottom"
      data-visible={show ? "" : undefined}
      onClick={(event) => {
        onClick?.(event)
        if (!event.defaultPrevented) scrollToBottom({ behavior: "smooth" })
      }}
      className={cn(
        "chatcn-focus absolute inset-x-0 bottom-4 z-10 mx-auto flex h-8 w-fit items-center justify-center gap-1.5 rounded-full",
        "bg-chatcn-card text-chatcn-fg-muted shadow-chatcn-md chatcn-text-13 font-medium tabular-nums select-none",
        "hover:text-chatcn-fg [&_svg]:size-4 [&_svg]:shrink-0",
        count ? "ps-2.5 pe-3" : "w-8",
        // Exit is 0.75x the enter. Under reduced motion only the fade remains, and it is instant.
        "ease-chatcn-out transition-[opacity,translate,color] duration-150",
        "pointer-coarse:after:absolute pointer-coarse:after:-inset-1.5 pointer-coarse:after:content-['']",
        show
          ? "translate-y-0 opacity-100"
          : "pointer-events-none translate-y-1 opacity-0 duration-110",
        "motion-reduce:translate-y-0 motion-reduce:transition-none",
        className,
      )}
      {...props}
    >
      <ArrowDownIcon aria-hidden="true" />
      {count ? <span>{t('{count} new', { count: count > 99 ? '99+' : count })}</span> : null}
    </button>
  )
}

export type { ScrollToBottomProps }
export { ScrollToBottom }
