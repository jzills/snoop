---
title: Copy and export
weight: 7
---

## Copy

Press `y` to copy the selected event to the clipboard:

| Selection | Copies |
| --- | --- |
| Bash call | the command |
| Edit or MultiEdit | the diff |
| Write | the content |
| prompt or reply | the full text |
| any other call | its result |
| a file, in the Files tab | all of its changes |

Copying uses OSC 52, so it works over SSH too. In tmux, enable it by adding
`set -g set-clipboard on` to your configuration.

## Export to Markdown

Press `e` to export the timeline as shown, including the search, filter,
thinking blocks and expanded subagents, to a Markdown file in the current
directory:

```text
./64f3ec44-20261025-1430.md
```

Prompts become headings, tool calls become collapsible `<details>` blocks,
and subagent events are quoted under their call. A subagent opened in its
own view exports as `agent-<id>-….md`.

From the shell, export a whole session, without thinking blocks:

```sh
snoop export 64f3ec44              # to stdout
snoop export 64f3ec44 -o notes.md
```
