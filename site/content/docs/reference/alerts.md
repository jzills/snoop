---
title: Alerts and notifications
weight: 3
---

While a session receives new lines, snoop raises alerts in its header. The
live board raises the same alerts for every session on it, prefixed with the
project. Lines already in a transcript when snoop opens it never raise
alerts.

## Alerts

| Name | Shown as | Raised when |
| --- | --- | --- |
| `error` | `✗ error: Bash` | a tool call fails. The header counts errors you haven't seen |
| `stall` | `◌ waiting 30s on Bash` | a tool call has no result after `--stall`. Often a permission prompt |
| `turn` | `✓ your turn` | Claude finishes a turn |
| `compact` | `⟳ compacted` | the context is compacted |
| `ctx` | `ctx 90%` | the context window passes 90% |

Any key hides the alert. Press `!` to jump to the event that raised it.

Calls that run a subagent, through the Agent tool or a forked skill, never
raise `stall`.

## Bell and notifications

The bell and desktop notifications are both off by default. With `--bell`,
snoop rings the terminal bell, and with `--notify` it sends a desktop
notification, both for the alerts chosen with `--alerts` (by default
`error,turn`):

```sh
snoop --follow --bell
snoop --live --notify --alerts turn,stall,error
```

A notification's title names the session and its body says what happened:

```text
api · Fix the flaky test
Waiting 45s on Bash · may be a permission prompt
```

## How notifications are delivered

For each environment, snoop picks one way to deliver notifications:

| Where snoop runs | Delivered by |
| --- | --- |
| iTerm2, WezTerm, Ghostty, kitty | the terminal's own notifications. Clicking one focuses the tab |
| over SSH | the terminal's own, so they appear on your machine, not the server |
| WSL | a Windows toast, through `powershell.exe` |
| macOS, other terminals | `osascript` |
| Linux desktop | `notify-send` |
| anywhere else | the terminal's escape sequence (OSC 777) |

To override the choice, use `--notify=terminal` or `--notify=system`.

## When notifications are held back

- None are sent while snoop's window has focus, in terminals that report
  focus.
- At most one goes out per session and alert a minute.
- Several snoops watching the same session send one between them.
