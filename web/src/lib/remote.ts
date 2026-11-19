// The origin (scheme, host and port) a git remote URL connects to, as the
// server compares them: a saved access token is only ever sent to its own.
// Empty for something that isn't a URL yet.
export function remoteOrigin(raw: string): string {
  try {
    const u = new URL(raw.trim())
    if (!u.hostname) return ''
    const port = u.port || (u.protocol === 'https:' ? '443' : '')
    return `${u.protocol}//${u.hostname}:${port}`
  } catch {
    return ''
  }
}

// Whether saving url would leave the saved token behind: it was saved for
// another origin, and the server refuses to send it to this one.
export function tokenLeftBehind(savedUrl: string, url: string): boolean {
  return remoteOrigin(url) !== remoteOrigin(savedUrl)
}

// The host the saved token belongs to, for telling the person.
export function remoteHost(raw: string): string {
  try {
    return new URL(raw.trim()).host
  } catch {
    return raw
  }
}
