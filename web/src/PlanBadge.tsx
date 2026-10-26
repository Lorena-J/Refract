// Required by the SIWC UI guidelines near the composer / model selector.
export default function PlanBadge({ manageUsage }: { manageUsage: string }) {
  return (
    <span className="text-xs text-muted-foreground">
      Using ChatGPT plan ·{' '}
      <a className="underline-offset-4 transition-colors duration-150 hover:text-foreground hover:underline" href={manageUsage} target="_blank" rel="noreferrer">
        Manage usage
      </a>
    </span>
  )
}
