export const isMac = /Mac|iPhone|iPad/.test(navigator.platform)
export const mod = isMac ? '⌘' : 'Ctrl+'
export const holdKey = isMac ? '⌘' : 'Ctrl'

export const shortcutGroups: { title: string; items: [string, string][] }[] = [
  {
    title: 'General',
    items: [
      [`${mod}K`, 'Command palette'],
      [`Hold ${holdKey}`, 'Show these shortcuts'],
      ['?', 'Show these shortcuts'],
    ],
  },
  {
    title: 'Editor',
    items: [
      [`${mod}↵`, 'Compile'],
      [`${mod}J`, 'Show the cursor position in the PDF'],
      [`${mod}+click`, 'In the PDF: jump to the source'],
      [`${mod}S`, 'Save now (it also saves on its own)'],
      [`${mod}W`, 'Close the file (when installed as an app)'],
    ],
  },
  {
    title: 'Quick actions',
    items: [
      [`${mod}K`, 'With text selected: edit or explain it'],
      ['Tab', 'Accept the suggestion'],
      ['Esc', 'Reject the suggestion'],
    ],
  },
  {
    title: 'Assistant',
    items: [
      ['↵', 'Send'],
      ['⇧↵', 'New line'],
      ['Esc', 'Collapse the chat'],
    ],
  },
]

export const editorShortcuts = shortcutGroups[1].items
