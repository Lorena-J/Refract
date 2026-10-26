// A deliberately small Markdown subset for assistant replies: paragraphs,
// headings, lists, fenced code, `code`, **bold**, *italic*, [links](https://…).
// Produces data, never HTML, so model output cannot inject markup.

export type Inline =
  | { t: 'text'; v: string }
  | { t: 'code'; v: string }
  | { t: 'strong'; c: Inline[] }
  | { t: 'em'; c: Inline[] }
  | { t: 'link'; href: string; c: Inline[] }

export type Block =
  | { t: 'p'; c: Inline[][] } // lines of a paragraph
  | { t: 'h'; c: Inline[] }
  | { t: 'ul' | 'ol'; items: Inline[][] }
  | { t: 'pre'; v: string }

const inlineRe = /(`[^`\n]+`)|(\*\*[^*\n]+\*\*)|(\*[^*\n]+\*|_[^_\n]+_)|(\[[^\]\n]+\]\((https?:\/\/[^)\s]+)\))/

export function parseInline(s: string): Inline[] {
  const out: Inline[] = []
  while (s) {
    const m = inlineRe.exec(s)
    if (!m) {
      out.push({ t: 'text', v: s })
      break
    }
    if (m.index > 0) out.push({ t: 'text', v: s.slice(0, m.index) })
    const tok = m[0]
    if (m[1]) out.push({ t: 'code', v: tok.slice(1, -1) })
    else if (m[2]) out.push({ t: 'strong', c: parseInline(tok.slice(2, -2)) })
    else if (m[3]) out.push({ t: 'em', c: parseInline(tok.slice(1, -1)) })
    else out.push({ t: 'link', href: m[5], c: parseInline(tok.slice(1, tok.indexOf(']'))) })
    s = s.slice(m.index + tok.length)
  }
  return out
}

const ulRe = /^\s*[-*•]\s+(.*)$/
const olRe = /^\s*\d+[.)]\s+(.*)$/

export function parseMarkdown(text: string): Block[] {
  const blocks: Block[] = []
  const lines = text.replace(/\r\n/g, '\n').split('\n')
  let para: Inline[][] = []
  const flush = () => {
    if (para.length) blocks.push({ t: 'p', c: para })
    para = []
  }
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    if (line.trimStart().startsWith('```')) {
      flush()
      const code: string[] = []
      for (i++; i < lines.length && !lines[i].trimStart().startsWith('```'); i++) code.push(lines[i])
      blocks.push({ t: 'pre', v: code.join('\n') })
      continue
    }
    const h = /^#{1,6}\s+(.*)$/.exec(line)
    if (h) {
      flush()
      blocks.push({ t: 'h', c: parseInline(h[1]) })
      continue
    }
    const list = ulRe.test(line) ? 'ul' : olRe.test(line) ? 'ol' : null
    if (list) {
      flush()
      const re = list === 'ul' ? ulRe : olRe
      const items: Inline[][] = []
      for (; i < lines.length && re.test(lines[i]); i++) items.push(parseInline(re.exec(lines[i])![1]))
      i--
      blocks.push({ t: list, items })
      continue
    }
    if (line.trim() === '') flush()
    else para.push(parseInline(line))
  }
  flush()
  return blocks
}
