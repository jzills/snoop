---
title: Files and diffs
weight: 4
---

The Files tab (`4`) lists every file the session and its subagents read,
edited or wrote.

{{< shot src="files.png" alt="The Files tab listing files with read, edit and write counts, and a file's history of diffs on the right" caption="The right pane shows each file's history, with every change as a diff." >}}

## The list

Each file shows its read, edit and write counts and the lines added and
removed. Changed files come first, then files only read, each most recent
first. Press `/` to keep only files whose path matches.

## The tree

Press `T` to show the files in a tree of folders, and press it again to go
back to the list. The choice carries over to the next session you open.
Each folder shows the totals of the files under it, and a folder that holds
only one folder shares its row, as in `internal/ui/`. Folders with changed
files start open, and folders whose files were only read start closed.

In the tree, press `space` or `enter` on a folder to open or close it, or
use `←` and `→`. Pressing `←` on a file moves to its folder. Press `E` on a
folder to open it and every folder inside it, and press it again to close
them. On a file, `E` does the same for every folder in the tree. While you search, every
folder with a match is open. Selecting a folder lists its files in the
right pane.

## A file's history

The right pane shows the selected file's history, with every change as a
diff. Press `n` and `N` to step through the changes, or `enter` to show the
file's latest change in the timeline; for a change a subagent made, that is
the Agent call.

Pressing `y` copies all of the file's changes to the clipboard.

## Comment on a change

Press `c` to comment on the change at the top of the right pane, the one `n`
and `N` move to, or `C` to comment on the whole file. The comment quotes the
change and names it, such as `the Edit at 14:03:12`, rather than giving line
numbers, which may have moved since. Press `c` on the same change again to
edit its comment. The file list counts each file's comments, and `S` sends
them along with any from the Diff tab, as described in
[Review and comment](../review/).

{{< callout type="info" >}}
Changes made through Bash, such as `sed -i` or a code generator, aren't
tracked. Only the Read, Edit, MultiEdit, Write and NotebookEdit tools are.
{{< /callout >}}
