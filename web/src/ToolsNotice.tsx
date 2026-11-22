import { useEffect } from 'react'
import { toast } from 'sonner'
import { t } from '@/lib/i18n'
import { getMissingTools, type ToolName } from '@/lib/tools'
import { usePath } from '@/lib/router'

export const effects: Record<ToolName, () => string> = {
  git: () =>
    t("Git is missing, so projects can't keep their history. Reinstall Refract, or install Git, then restart it."),
  tectonic: () => t("Tectonic is missing, so documents can't be compiled."),
  synctex: () => t('SyncTeX is missing, so jumping between the source and the PDF is unavailable.'),
  rg: () => t("ripgrep is missing, so the assistant can't search the project."),
}

let shown = false

/**
 * One notice, shown once per page load on the project list when the server started
 * without some of its tools, instead of failing on first use. Inside a project the
 * features that need a missing tool say so when used (see Workspace).
 */
export default function ToolsNotice() {
  const path = usePath()
  useEffect(() => {
    const missing = getMissingTools()
    if (shown || path !== '/' || missing.length === 0) return
    shown = true
    toast.warning(t('Some tools are missing'), {
      id: 'missing-tools',
      duration: 15000,
      closeButton: true,
      description: (
        <ul className="list-disc pl-4">
          {missing.map((n) => (
            <li key={n}>{effects[n]()}</li>
          ))}
        </ul>
      ),
    })
  }, [path])
  return null
}
