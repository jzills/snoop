---
title: Troubleshooting
weight: 5
---

## No sessions are listed

By default, snoop reads `~/.claude/projects`, or `$CLAUDE_CONFIG_DIR/projects`
when that variable is set. If you run Claude Code with a custom `CLAUDE_CONFIG_DIR`,
set the same value for snoop.

## A session shows less than expected

The transcript format is undocumented and changes between Claude Code
versions. Rather than failing, snoop skips records it doesn't recognize, so a
newer Claude Code may show less. Please
[open an issue](https://github.com/jzills/snoop/issues) with your Claude Code
version (`claude --version`).

## Notifications don't appear

- **In tmux:** the terminal's own notifications need passthrough. Add
  `set -g allow-passthrough on` to `~/.tmux.conf`.
- **On WSL:** notifications are Windows toasts sent through
  `powershell.exe`. Check that Windows notifications are on, and that Focus
  Assist or Do Not Disturb isn't hiding them.
- **On Linux:** `notify-send` must be installed, from `libnotify-bin` or
  `libnotify`, and a desktop session must be running.
- **While snoop has focus:** none are sent while you're looking at snoop.
  Switch to another window to test.
- **Wrong method:** force one with `--notify=terminal` or `--notify=system`.

If sending fails, snoop shows the error in its footer.

## Copy doesn't reach the clipboard

Copying uses OSC 52, which most terminals support, though some need it
enabled. In tmux, add `set -g set-clipboard on` to `~/.tmux.conf`.

## A session is "waiting" but nothing is asked

The waiting state means a tool call has had no result for `--stall`. A
permission prompt is the usual cause, but a slow command, such as a long test
run, looks the same. Raise the threshold with `--stall 2m`, or turn it off
with `--stall 0`.
