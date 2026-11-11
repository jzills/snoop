---
title: Subagents
weight: 3
---

When Claude Code starts a subagent, through the Agent tool or a forked skill, the subagent writes a transcript of its own.
It appears in snoop under the call that started it.

## Expand inline

Press `x` on an Agent call to expand what its subagent did, indented under a `│` rule.
The expanded events stay live while the subagent runs.
Press `X` to expand every subagent, nested ones included, and again to collapse them all.

{{< shot src="subagent.png" alt="An Agent call expanded inline, showing the subagent's prompt and tool calls indented beneath it" caption="An Explore subagent is expanded under its Agent call." >}}

## Open in its own view

Press `a` on an Agent call to open the subagent's transcript in a view of its own, with all four tabs.
Press `esc` to return to the parent session.

## Background subagents

A subagent or skill started in the background, such as `/code-review`, returns at once and reports back later in a task notification.
Until then, its call shows a spinner instead of `✓`, and the session stays working even after its turn ends.
The timeline's last row names the newest one and what it is doing, such as `⠹ 1 in background · code-review · 3m06s  Read app.go`.
When it reports back, the call shows its full duration and Claude Code goes on with the turn.

## Search and files

Search and filters reach inside subagents.
A collapsed call whose subagent has matches stays in the list, marked `●`.
The Files tab includes files the subagents touched, labeled with the subagent that touched them.

## Waiting and stalls

A subagent can run for minutes, so snoop never reports an Agent call, or a forked skill whose subagent is writing, as waiting on you.
See [Alerts and notifications](../../reference/alerts/).
