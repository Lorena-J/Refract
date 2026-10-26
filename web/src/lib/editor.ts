import { HighlightStyle, StreamLanguage, syntaxHighlighting } from '@codemirror/language'
import { EditorView } from '@codemirror/view'
import { stex } from '@codemirror/legacy-modes/mode/stex'
import { tags as t } from '@lezer/highlight'

// LaTeX highlighting driven by CSS variables, so it follows the light/dark theme.
const latexStyle = HighlightStyle.define([
  { tag: [t.tagName, t.keyword], color: 'var(--syn-command)' },
  { tag: t.bracket, color: 'var(--syn-brace)' },
  { tag: [t.atom, t.special(t.variableName), t.variableName], color: 'var(--syn-arg)' },
  { tag: [t.number, t.string], color: 'var(--syn-math)' },
  { tag: t.comment, color: 'var(--syn-comment)', fontStyle: 'italic' },
  { tag: t.invalid, color: 'var(--destructive)' },
])

export const latex = [StreamLanguage.define(stex), syntaxHighlighting(latexStyle), EditorView.lineWrapping]
