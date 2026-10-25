---
title: Files and diffs
weight: 4
---

The Files tab (`4`) lists every file the session and its subagents read,
edited or wrote.

{{< shot src="files.png" alt="The Files tab listing files with read, edit and write counts, and a file's history of diffs on the right" caption="Each file's history, with every change as a diff." >}}

## The list

Each file shows its read, edit and write counts and the lines added and
removed. Changed files come first, then files only read, each most recent
first. `/` keeps only files whose path matches.

## A file's history

The right pane shows the selected file's history, with every change as a
diff. `n` and `N` step through the changes. `enter` shows the file's latest
change in the timeline; for a change a subagent made, it shows the Agent
call.

`y` copies all of the file's changes to the clipboard.

{{< callout type="info" >}}
Changes made through Bash, such as `sed -i` or a code generator, aren't
tracked. snoop sees only the Read, Edit, MultiEdit, Write and NotebookEdit
tools.
{{< /callout >}}
