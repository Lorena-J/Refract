import type * as React from "react"
import { cn } from "@/lib/utils"

/**
 * The only shimmer in chatcn: a status label such as "Thinking". Static under reduced motion.
 * Pass `active={false}` to settle it into plain subtle text.
 */
function ShimmerText({
  active = true,
  className,
  ...props
}: React.ComponentProps<"span"> & { active?: boolean }) {
  return (
    <span
      data-slot="shimmer-text"
      className={cn("font-medium", active ? "chatcn-shimmer" : "text-chatcn-fg-subtle", className)}
      {...props}
    />
  )
}

export { ShimmerText }
