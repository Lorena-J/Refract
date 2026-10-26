import type { ReactNode } from 'react'
import { parseMarkdown, type Inline } from '@/lib/markdown'

function inline(nodes: Inline[]): ReactNode[] {
  return nodes.map((n, i) => {
    switch (n.t) {
      case 'text':
        return n.v
      case 'code':
        return (
          <code key={i} className="rounded bg-muted px-1 py-0.5 font-mono text-[0.85em]">
            {n.v}
          </code>
        )
      case 'strong':
        return <strong key={i}>{inline(n.c)}</strong>
      case 'em':
        return <em key={i}>{inline(n.c)}</em>
      case 'link':
        return (
          <a key={i} href={n.href} target="_blank" rel="noreferrer" className="underline underline-offset-4">
            {inline(n.c)}
          </a>
        )
    }
  })
}

// Renders the assistant's Markdown subset (see lib/markdown.ts).
export default function Markdown({ text }: { text: string }) {
  return (
    <div className="space-y-2.5">
      {parseMarkdown(text).map((b, i) => {
        switch (b.t) {
          case 'p':
            return (
              <p key={i}>
                {b.c.map((line, j) => (
                  <span key={j}>
                    {j > 0 && <br />}
                    {inline(line)}
                  </span>
                ))}
              </p>
            )
          case 'h':
            return (
              <p key={i} className="font-semibold">
                {inline(b.c)}
              </p>
            )
          case 'ul':
          case 'ol': {
            const List = b.t
            return (
              <List key={i} className={b.t === 'ul' ? 'list-disc space-y-1 pl-5' : 'list-decimal space-y-1 pl-5'}>
                {b.items.map((it, j) => (
                  <li key={j}>{inline(it)}</li>
                ))}
              </List>
            )
          }
          case 'pre':
            return (
              <pre key={i} className="overflow-x-auto rounded-lg bg-muted p-3 font-mono text-xs leading-relaxed">
                {b.v}
              </pre>
            )
        }
      })}
    </div>
  )
}
