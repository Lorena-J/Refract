// The origin (scheme, host and port) a git remote URL connects to, as the
// server compares them: a saved access token is only ever sent to its own.
// Empty for something that isn't a URL yet.
export function remoteOrigin(raw: string): string {
  try {
    const u = new URL(raw.trim())
    if (!u.hostname || u.protocol !== 'https:') return ''
    const port = u.port || '443'
    return `${u.protocol}//${u.hostname}:${port}`
  } catch {
    return ''
  }
}

// Whether saving url would leave the saved token behind: it was saved for
// another origin, and the server refuses to send it to this one. False for
// something that isn't a URL yet: the server refuses that save anyway.
export function tokenLeftBehind(savedUrl: string, url: string): boolean {
  const next = remoteOrigin(url)
  return next !== '' && next !== remoteOrigin(savedUrl)
}

// The host the saved token belongs to, for telling the person.
export function remoteHost(raw: string): string {
  try {
    return new URL(raw.trim()).host
  } catch {
    return raw
  }
}
