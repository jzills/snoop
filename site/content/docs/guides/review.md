---
title: Review and comment
weight: 5
---

The Diff tab (`5`) shows what the session changed in its working tree, and
lets you comment on it. The agent receives the comments on its own, without
you typing into its terminal.

## Set up the hooks once

Comments reach the agent through Claude Code hooks. Run this once:

```sh
snoop hooks install
```

It adds three entries to `~/.claude/settings.json`, keeping everything else
in the file. With `--project`, it uses `.claude/settings.json` in the current
directory instead. Run `snoop hooks status` to check them, and
`snoop hooks uninstall` to remove them. A session that was already running
may need a restart before it uses the hooks.

## Read the diff

The tab lists every file in `git diff HEAD`, plus untracked files, shown as
added. Files this session or its subagents changed come first. Files changed
by you or another agent are dimmed, and the right pane says so. Press
`enter` or `tab` to move into a file's lines, and `J` and `K` to jump between
hunks.

The diff is read when you open the tab, again when the session finishes an
edit or a command (at most every two seconds), and when you press `r`. Git
runs in the background, so following the session doesn't slow down.

## Comment

Press `c` on a line to comment on it. To comment on several lines, press `v`,
move to the last line, and press `c`. Press `C` to comment on the whole file.
A `●` marks commented lines, and the file list counts each file's comments.
Press `e` on a commented line to edit its comment, or `x` to delete it.

## Send

Press `S` to send every comment at once. The agent sees them at the first of
three moments:

- While it's working, it sees them after its current tool call.
- When it tries to finish its turn, it gets them and keeps going.
- Otherwise, they arrive with your next prompt.

The footer shows `sent 2 comments, waiting for the agent` until a hook takes
them, then `delivered 2 comments`. If the session isn't running, the
comments wait until you resume it.

Each comment names the file and lines and quotes the code, like this:

```text
Review comments from snoop:

internal/store/store.go:40-42
    40 +	if err != nil {
    41 +		return nil
    42 +	}
Don't swallow the error; return it.
```

Without the hooks, `S` copies the comments to the clipboard instead and
keeps them, so you can paste them into the session or install the hooks and
send them again.

Leaving a session or quitting with unsent comments asks first. With
`--follow`, a new session opens only once your comments are sent or
discarded.
