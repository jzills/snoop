---
title: The live board
weight: 5
---

The live board lists every session written in the last 30 minutes, across
projects, most urgent first. Use it when you run several sessions at once
and want to know which one needs you.

```sh
snoop --live
```

Or press `w` in the session picker.

{{< shot src="live.png" width="960" height="280" alt="The live board with one session waiting on a tool call, one whose turn ended and one working" caption="One session waiting on you, one whose turn has ended, one still working." >}}

## States

| State | Meaning |
| --- | --- |
| `◌ waiting` | a tool call has had no result for 30 seconds, usually a permission prompt |
| `✓ your turn` | the turn ended, or you interrupted it |
| `● working` | replying, or running a tool or subagent |
| `· idle` | no prompts yet, or nothing written for 10 minutes |

The header counts the sessions waiting on you. Each row shows the project,
the session, how full its context is, its cost, and when it last wrote.
Sessions leave the board after 30 minutes without writes.

Press `enter` to open a session and `esc` to come back to the board. From
the board, `esc` or `w` returns to the session picker.

The waiting threshold is `--stall`, 30 seconds by default. Permission
prompts aren't recorded in the transcript until you answer them, so a tool
call with no result is the closest sign of one.

## Notifications

Add `--notify` to get a desktop notification when a session needs you:

```sh
snoop --live --notify
```

Each notification names the session and what happened:

```text
api · Fix the flaky test
Your turn · finished after 2m10s
```

By default snoop notifies when a turn ends and when a tool call fails. Choose
others with `--alerts`, for example `--alerts turn,stall,error` to include
sessions waiting on a tool call.

Nothing is sent while snoop's window has focus. At most one notification goes
out per session and alert a minute, and several snoops watching the same
session send one between them.

[Alerts and notifications](../../reference/alerts/) lists every alert and how
notifications are delivered on each platform.

## Cost of watching

Only sessions on the board are read, and after the first read only what they
add. An idle board checks each file's size once a second.
