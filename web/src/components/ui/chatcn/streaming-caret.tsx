"use client"

import * as React from "react"
import { cn } from "@/lib/utils"

type StreamingCaretStatus = "streaming" | "waiting" | "done"

interface StreamingCaretProps extends Omit<React.ComponentProps<"span">, "children"> {
  /**
   * `streaming` is solid, `waiting` blinks slowly, `done` removes the caret. Defaults to
   * `streaming`.
   */
  status?: StreamingCaretStatus
  /**
   * Pass the streamed text (or anything that changes with each token). While it keeps changing
   * the caret stays solid; once it holds still for `idleMs` the caret starts to blink.
   */
  activity?: unknown
  /** Quiet time before the caret switches to blinking. Defaults to 600 ms. */
  idleMs?: number
}

/**
 * The signature mark: a small rounded square that means "the model is writing here". It sits on
 * the text baseline at 0.55em, so it scales with whatever text it follows. Solid while tokens flow,
 * a slow 1.2 s blink only while waiting, gone when done. Static under reduced motion.
 */
function StreamingCaret({
  status = "streaming",
  activity,
  idleMs = 600,
  className,
  ...props
}: StreamingCaretProps) {
  const [idle, setIdle] = React.useState(false)
  const tracking = activity !== undefined && status === "streaming"

  React.useEffect(() => {
    if (!tracking) return
    setIdle(false)
    const id = window.setTimeout(() => setIdle(true), idleMs)
    return () => window.clearTimeout(id)
  }, [activity, tracking, idleMs])

  if (status === "done") return null
  const blinking = status === "waiting" || (tracking && idle)

  return (
    <span
      aria-hidden="true"
      data-slot="streaming-caret"
      data-caret-state={blinking ? "waiting" : "streaming"}
      className={cn(
        "bg-chatcn-accent ms-0.5 inline-block size-[0.55em] translate-y-[0.04em] rounded-[0.12em] align-baseline",
        blinking &&
          "animate-[chatcn-caret-blink_1.2s_steps(1,end)_infinite] motion-reduce:animate-none",
        className,
      )}
      {...props}
    />
  )
}

export type { StreamingCaretProps, StreamingCaretStatus }
export { StreamingCaret }
