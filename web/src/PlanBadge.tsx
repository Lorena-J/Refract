import { t } from '@/lib/i18n'

// Required by the SIWC UI guidelines near the composer / model selector.
export default function PlanBadge({ manageUsage }: { manageUsage: string }) {
  return (
    <span className="text-xs text-muted-foreground">
      {t('Using ChatGPT plan')} ·{' '}
      <a
        className="underline-offset-4 transition-colors duration-150 hover:text-foreground hover:underline"
        href={manageUsage}
        target="_blank"
        rel="noreferrer"
      >
        {t('Manage usage')}
      </a>
    </span>
  )
}
