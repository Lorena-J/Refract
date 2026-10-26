export const mod = /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘' : 'Ctrl+'

export const shortcuts: [string, string][] = [
  [`${mod}↵`, 'Compile'],
  [`${mod}J`, 'Show the cursor position in the PDF'],
  [`${mod}+click`, 'In the PDF: jump to the source'],
  [`${mod}S`, 'Save now (it also saves on its own)'],
  [`${mod}K`, 'With text selected: ask the assistant to change it'],
  ['Tab', 'Accept a quick-action suggestion'],
  ['Esc', 'Reject a quick-action suggestion'],
  ['↵', 'In the assistant: send'],
  ['⇧↵', 'In the assistant: new line'],
  ['Esc', 'In the assistant: collapse the chat'],
]
