"use client"

import * as React from "react"
import { ShimmerText } from "@/components/ui/chatcn/shimmer-text"
import { StreamingCaret } from "@/components/ui/chatcn/streaming-caret"
import { t } from '@/lib/i18n'
import { cn } from "@/lib/utils"

/* -------------------------------------------------------------------------------------------------
 * Timing
 * -----------------------------------------------------------------------------------------------*/

/**
 * Delayed presence for loaders: stays hidden for `delay` ms after `active` turns on, and once
 * shown stays for at least `minDuration` ms. A wait shorter than the delay shows nothing at all,
 * so the indicator can never flash.
 */
function useDelayedPresence(active: boolean, delay = 300, minDuration = 400) {
  const [shown, setShown] = React.useState(false)
  const shownAt = React.useRef(0)

  React.useEffect(() => {
    if (active && !shown) {
      const id = window.setTimeout(() => {
        shownAt.current = Date.now()
        setShown(true)
      }, delay)
      return () => window.clearTimeout(id)
    }
    if (!active && shown) {
      const left = minDuration - (Date.now() - shownAt.current)
      if (left <= 0) {
        setShown(false)
        return
      }
      const id = window.setTimeout(() => setShown(false), left)
      return () => window.clearTimeout(id)
    }
  }, [active, shown, delay, minDuration])

  return shown
}

const NBSP = "\u00a0"

function formatElapsed(ms: number) {
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}${NBSP}s`
  return `${Math.floor(s / 60)}${NBSP}m ${s % 60}${NBSP}s`
}

/** Seconds since `since`, ticking once per second while `running`. */
function useSeconds(since: number | undefined, running: boolean) {
  const [now, setNow] = React.useState(() => Date.now())
  React.useEffect(() => {
    if (!running) return
    setNow(Date.now())
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [running])
  return since === undefined ? 0 : Math.max(0, now - since)
}

/* -------------------------------------------------------------------------------------------------
 * ThinkingIndicator
 * -----------------------------------------------------------------------------------------------*/

type ThinkingIndicatorVariant = "label" | "timer" | "motif"

interface ThinkingIndicatorProps extends Omit<React.ComponentProps<"div">, "children"> {
  /** True while the model is working and nothing visible has arrived yet. Defaults to true. */
  active?: boolean
  /** `label` is the shimmer word, `timer` adds elapsed seconds, `motif` is the square alone. */
  variant?: ThinkingIndicatorVariant
  /** The status word. Defaults to "Thinking". Use a verb for the current step: "Searching". */
  label?: string
  /** Epoch ms when the wait began, for the timer. Defaults to when `active` turned on. */
  startedAt?: number
  /** Hidden for this long after `active` turns on. Defaults to 300 ms. */
  delay?: number
  /** Once shown, stays at least this long. Defaults to 400 ms. */
  minDuration?: number
}

/**
 * Pending state before the first token. Shows nothing for 300 ms (the sent message is the
 * feedback), then a shimmering "Thinking" that stays at least 400 ms so it never flashes.
 */
function ThinkingIndicator({
  active = true,
  variant = "label",
  label = t('Thinking'),
  startedAt,
  delay = 300,
  minDuration = 400,
  className,
  ...props
}: ThinkingIndicatorProps) {
  const shown = useDelayedPresence(active, delay, minDuration)
  const [since, setSince] = React.useState<number>()
  React.useEffect(() => {
    if (active) setSince((s) => s ?? Date.now())
    else setSince(undefined)
  }, [active])
  const elapsed = useSeconds(startedAt ?? since, shown && variant === "timer")

  return (
    <div
      role="status"
      data-slot="thinking-indicator"
      data-variant={variant}
      data-visible={shown ? "" : undefined}
      // Hidden, it keeps no space but stays mounted, so the live region announces reliably.
      className={cn(shown ? "chatcn-text-14 flex h-6 items-center gap-2" : "sr-only", className)}
      {...props}
    >
      {shown ? (
        <span
          className={cn(
            "inline-flex items-center gap-2",
            "ease-chatcn-out transition-opacity duration-150 starting:opacity-0",
          )}
        >
          {variant === "motif" ? (
            <>
              <StreamingCaret status="waiting" className="chatcn-text-16 ms-0" />
              <span className="sr-only">{label}</span>
            </>
          ) : (
            <>
              <ShimmerText>{label}</ShimmerText>
              {variant === "timer" && elapsed >= 1000 ? (
                <span aria-hidden="true" className="text-chatcn-fg-subtle tabular-nums">
                  {formatElapsed(elapsed)}
                </span>
              ) : null}
            </>
          )}
        </span>
      ) : null}
    </div>
  )
}

export type { ThinkingIndicatorProps, ThinkingIndicatorVariant }
export { ThinkingIndicator, useDelayedPresence }
