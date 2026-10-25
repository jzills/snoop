---
title: Quickstart
weight: 3
---

This page takes you from installing snoop to following a session live.

{{% steps %}}

### Open the session picker

Run `snoop` in any terminal. It lists your Claude Code sessions across all
projects, newest first, with each one's project, branch, title and size.

{{< shot src="picker.png" alt="The session picker listing sessions with their age, project, branch and title" >}}

Type `/` to search titles, projects and branches, or `c` to show only the
sessions for the current directory.

### Open a session

Move to a session with `↑` `↓` (or `j` `k`) and press `enter`. The timeline
lists every prompt, reply and tool call. The right pane shows the selected
one in full: the command and output of a Bash call, the diff of an edit, the
text of a reply.

The header shows the project, branch and how full the context window is.
Press `1` to `4` to switch between the Timeline, Tools, Stats and Files tabs.

### Follow a session live

In a second terminal beside `claude`, run:

```sh
snoop --follow
```

This opens the newest session for the current directory and selects new
events as they arrive. When Claude Code starts a new session there, snoop
switches to it.

### Learn the keys that matter

| Key | Action |
| --- | --- |
| `n` `N` | next and previous prompt |
| `/` | search text, commands, paths and results |
| `x` | expand an Agent call's subagent inline |
| `esc` | clear the search, then go back |
| `?` | every key for the current screen |

{{% /steps %}}

## Next

- [Following a session](../guides/following/) explains the timeline in detail.
- [The live board](../guides/live-board/) shows every running session at once.
- [Keyboard shortcuts](../reference/keys/) lists every key.
