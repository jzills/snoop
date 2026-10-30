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

You can also press `w` in the session picker.

{{< shot src="live.png" width="960" height="280" alt="The live board with one session waiting on a tool call, one whose turn ended and one working" caption="One session is waiting on you, one has finished its turn, and one is still working." >}}

## States

| State | Meaning |
| --- | --- |
| `? asking` | the session asks for permission to use a tool, which you can answer here |
| `◌ waiting` | a tool call has had no result for 30 seconds, usually a permission prompt |
| `✓ your turn` | the turn ended, or you interrupted it |
| `● working` | replying, or running a tool or subagent |
| `· idle` | no prompts yet, or nothing written for 10 minutes |

The header counts the sessions waiting on you. Each row shows the project,
the session, how full its context is, its cost, and when it last wrote.
Sessions leave the board after 30 minutes without writes.

Press `enter` to open a session and `esc` to come back to the board. From
the board, `esc` or `w` returns to the session picker.

To sort by another column, press `o`, which moves to the next column, and
`O` to reverse the order. The sorted column's heading is marked `▾`, or `▴`
when reversed.

The waiting threshold is `--stall`, 30 seconds by default. Permission
prompts aren't recorded in the transcript until you answer them, so a tool
call with no result is the closest sign of one.

## Answer permission prompts

When a session asks for permission to use a tool, its row shows `? asking`
and what it wants, such as `Bash: npm test`. Selecting the row shows the
request in full under the list: the command and its description, the
edit's diff, or the tool's input, with the folder it runs in.

{{< shot src="ask.png" width="960" height="460" alt="The live board with a session asking to run an npm command, and the request in full under the list" caption="The selected session asks to run its e2e tests, and the request is shown in full under the list." >}}

Press `A` to allow it once, or `D` to deny it. After `D`, you can type a
reason, which Claude sees, and press `enter`, or press `esc` to cancel. The
same keys work in the session's own view, whose footer shows the request.
When a session has several requests, such as from parallel subagents, the
keys answer the oldest.

This needs snoop's hooks, which you add once:

```sh
snoop hooks install
```

Claude Code's own prompt still appears in the session's terminal, and
whichever answer comes first wins. The hook waits for snoop only while a
live board or session view is open, for at most 5 minutes; to change that,
pass `--ask-wait`, such as `snoop hooks install --ask-wait 10m`. With no
snoop open, the hook returns at once. There is no "always allow" here, so
permission rules stay in Claude Code's own prompt.

{{< callout type="info" >}}
Sessions that can't show a prompt, such as background subagents in
non-interactive mode, deny a request no hook answers. That's Claude Code's
behavior with or without snoop.
{{< /callout >}}

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

By default snoop notifies when a turn ends, when a tool call fails, and when
a session asks for permission (`Bash wants to run npm test`). Choose
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
