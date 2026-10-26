import { cn } from '@/lib/utils'

// A tiny sheet of paper with text lines: the project "thumbnail".
export default function PaperIcon({ className }: { className?: string }) {
  return (
    <span className={cn('flex h-9 w-7 shrink-0 flex-col gap-[3px] rounded-[2px] bg-white px-1.5 pt-2 shadow-sm ring-1 ring-black/10', className)} aria-hidden>
      <span className="h-px w-3/4 self-center bg-neutral-400" />
      <span className="mt-0.5 h-px w-full bg-neutral-300" />
      <span className="h-px w-full bg-neutral-300" />
      <span className="h-px w-5/6 bg-neutral-300" />
      <span className="h-px w-full bg-neutral-300" />
    </span>
  )
}
