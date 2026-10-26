// Every /api call carries X-Refract (the server rejects requests without it: CSRF).
const headers = { 'X-Refract': '1' }

export type ApiError = { kind: string; message: string; code?: string; param?: string }

export class RequestError extends Error {
  constructor(public error: ApiError) {
    super(error.message)
  }
}

export async function api<T>(path: string, init: { method?: string; body?: unknown } = {}): Promise<T> {
  const res = await fetch(path, {
    method: init.method ?? 'GET',
    headers: init.body === undefined ? headers : { ...headers, 'Content-Type': 'application/json' },
    body: init.body === undefined ? undefined : typeof init.body === 'string' ? init.body : JSON.stringify(init.body),
  })
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => ({ error: { kind: 'other', message: `HTTP ${res.status}` } }))
  if (!res.ok) throw new RequestError(data.error)
  return data as T
}

// Reads a server-sent event stream from a POST (EventSource can't send headers or a body).
export async function stream(path: string, body: unknown, onEvent: (ev: any) => void, signal?: AbortSignal) {
  const res = await fetch(path, {
    method: 'POST',
    headers: { ...headers, 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  })
  if (!res.ok || !res.body) {
    const data = await res.json().catch(() => null)
    onEvent({ type: 'error', error: data?.error ?? { kind: 'other', message: `HTTP ${res.status}` } })
    return
  }
  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader()
  let buf = ''
  for (;;) {
    const { value, done } = await reader.read()
    if (done) return
    buf += value
    let i
    while ((i = buf.indexOf('\n\n')) >= 0) {
      const chunk = buf.slice(0, i)
      buf = buf.slice(i + 2)
      if (chunk.startsWith('data: ')) onEvent(JSON.parse(chunk.slice(6)))
    }
  }
}

export type ChatGPTStatus = {
  status: { provider: string; signed_in: boolean; plan_granted: boolean; email?: string; limit_reached: boolean }
  welcome_seen: boolean
  manage_usage: string
  default_model: string
}

// User-facing text per error kind (PLAN.md §4.4).
export function describe(e: ApiError): string {
  switch (e.kind) {
    case 'not_signed_in':
      return 'Not signed in to ChatGPT. Run `refract login` and upload the credentials in Settings.'
    case 'plan_not_granted':
      return 'ChatGPT plan usage was not granted. Run `refract login` again and allow plan usage.'
    case 'usage_limit':
      return 'You have reached a usage limit on your ChatGPT plan or for this app. Review it in ChatGPT settings → Usage.'
    case 'not_eligible':
      return `ChatGPT plan usage is not available for this account: ${e.message}`
    case 'model_unavailable':
      return 'This model is not available on your ChatGPT plan. Pick another one.'
    case 'unsupported':
      return `Request not supported on plan usage (${e.param ?? 'unknown'}): ${e.message}`
    case 'unavailable':
      return `ChatGPT is temporarily unavailable. Try again shortly. (${e.message})`
    default:
      return e.message
  }
}
