import * as React from "react"
import {
  type ChatcnScrollBehavior,
  type UseChatcnStickToBottomOptions,
  useChatcnStickToBottom,
} from "@/hooks/use-chatcn-stick-to-bottom"
import { cn } from "@/lib/utils"

/* -------------------------------------------------------------------------------------------------
 * Context
 * -----------------------------------------------------------------------------------------------*/

interface ConversationContextValue {
  scrollRef: (element: HTMLElement | null) => void
  contentRef: (element: HTMLElement | null) => void
  spacerRef: (element: HTMLElement | null) => void
  /** Within the threshold of the bottom. */
  isAtBottom: boolean
  /** Following new content. */
  isSticky: boolean
  /** More than one viewport from the bottom. */
  isFar: boolean
  /** Messages that arrived below while the reader was scrolled up. */
  unread: number
  scrollToBottom: (options?: { behavior?: ChatcnScrollBehavior }) => void
  scrollToTurn: (element: HTMLElement, options?: { behavior?: ChatcnScrollBehavior }) => void
  /** Read text to screen readers through the conversation's polite live region. */
  announce: (text: string) => void
  /** False during the first render, so restored history does not animate in. */
  ready: boolean
}

const ConversationContext = React.createContext<ConversationContextValue | null>(null)

/** Scroll state and controls of the nearest `Conversation`. Returns null outside one. */
function useConversation() {
  return React.useContext(ConversationContext)
}

/* -------------------------------------------------------------------------------------------------
 * Conversation
 * -----------------------------------------------------------------------------------------------*/

interface ConversationProps extends React.ComponentProps<"div">, UseChatcnStickToBottomOptions {}

/**
 * The thread. Owns stick-to-bottom scrolling, the reply anchor and the live region.
 * Give it a height (or put it in a flex column) and place `ConversationContent` and
 * `ScrollToBottom` inside.
 */
function Conversation({
  threshold,
  anchor,
  anchorOffset,
  initial,
  className,
  children,
  ...props
}: ConversationProps) {
  const stick = useChatcnStickToBottom({ threshold, anchor, anchorOffset, initial })
  const [message, setMessage] = React.useState("")
  const [ready, setReady] = React.useState(false)
  React.useEffect(() => setReady(true), [])

  const announce = React.useCallback((text: string) => {
    // Clearing first makes repeated identical text announce again.
    setMessage("")
    requestAnimationFrame(() => setMessage(text))
  }, [])

  const value = React.useMemo<ConversationContextValue>(
    () => ({ ...stick, announce, ready }),
    [stick, announce, ready],
  )

  return (
    <ConversationContext.Provider value={value}>
      <div
        data-slot="conversation"
        data-at-bottom={stick.isAtBottom ? "" : undefined}
        className={cn("relative flex min-h-0 flex-col overflow-hidden", className)}
        {...props}
      >
        {children}
        <div aria-live="polite" aria-atomic="true" className="sr-only">
          {message}
        </div>
      </div>
    </ConversationContext.Provider>
  )
}

/* -------------------------------------------------------------------------------------------------
 * ConversationContent
 * -----------------------------------------------------------------------------------------------*/

interface ConversationContentProps extends React.ComponentProps<"div"> {
  /** Class names for the scrolling viewport. `className` styles the message column. */
  viewportClassName?: string
  /** Accessible name of the transcript. */
  label?: string
}

/**
 * The scrolling transcript. The viewport is a focusable `log` so keyboard users can scroll it;
 * the column inside is capped at 720 px with a 24 px gap between messages.
 */
function ConversationContent({
  viewportClassName,
  label = "Conversation",
  className,
  children,
  ...props
}: ConversationContentProps) {
  const ctx = useConversation()
  if (!ctx) throw new Error("ConversationContent must be used inside Conversation.")
  return (
    <div
      ref={ctx.scrollRef}
      role="log"
      aria-label={label}
      // The log is silent: tokens are never read one by one. Finished replies are announced
      // through the conversation's live region instead.
      aria-live="off"
      // biome-ignore lint/a11y/noNoninteractiveTabindex: scrollable regions must be focusable
      tabIndex={0}
      data-slot="conversation-viewport"
      className={cn(
        "chatcn-scroll min-h-0 flex-1 overflow-y-auto outline-none [overflow-anchor:auto]",
        "focus-visible:outline-chatcn-accent focus-visible:outline-2 focus-visible:-outline-offset-2",
        viewportClassName,
      )}
    >
      <div
        ref={ctx.contentRef}
        data-slot="conversation-content"
        className={cn(
          "mx-auto flex w-full max-w-[720px] flex-col gap-6 px-4 pt-8 pb-6 sm:px-6",
          className,
        )}
        {...props}
      >
        {children}
      </div>
      <div
        ref={ctx.spacerRef}
        aria-hidden="true"
        data-slot="conversation-spacer"
        className="pointer-events-none shrink-0 [overflow-anchor:none]"
      />
    </div>
  )
}

export type { ConversationContentProps, ConversationContextValue, ConversationProps }
export { Conversation, ConversationContent, useConversation }
