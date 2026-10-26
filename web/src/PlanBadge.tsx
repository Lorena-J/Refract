// Required by the SIWC UI guidelines near the composer / model selector.
export default function PlanBadge({ manageUsage }: { manageUsage: string }) {
  return (
    <span className="plan-badge">
      Using ChatGPT plan ·{' '}
      <a href={manageUsage} target="_blank" rel="noreferrer">
        Manage usage
      </a>
    </span>
  )
}
