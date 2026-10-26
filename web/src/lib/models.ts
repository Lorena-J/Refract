import type { Model } from '../api'

/** The model and effort a turn will use: the stored pick when the plan still offers it, else the defaults. */
export function resolveChoice(models: Model[], model: string, effort: string): { model: Model | undefined; effort: string } {
  const m = models.find((x) => x.slug === model) ?? models[0]
  if (!m) return { model: undefined, effort: '' }
  return { model: m, effort: m.efforts.some((e) => e.effort === effort) ? effort : m.default_effort }
}
