---
title: Subagents
weight: 3
---

When Claude Code starts a subagent, through the Agent tool or a forked
skill, the subagent writes a transcript of its own. snoop shows it under the
call that started it.

## Expand inline

Press `x` on an Agent call to expand what its subagent did, indented under a
`│` rule. The expanded events stay live while the subagent runs. Press `X`
to expand every subagent, nested ones included, and again to collapse them
all.

{{< shot src="subagent.png" alt="An Agent call expanded inline, showing the subagent's prompt and tool calls indented beneath it" caption="An Explore subagent expanded under its Agent call." >}}

## Open in its own view

Press `a` on an Agent call to open the subagent's transcript in a view of its
own, with all four tabs. Press `esc` to return to the parent session.

## Search and files

Search and filters reach inside subagents. A collapsed call whose subagent
has matches stays in the list, marked `●`. The Files tab includes files the
subagents touched, labelled with the subagent that touched them.

## Waiting and stalls

A subagent can run for minutes. snoop never reports an Agent call, or a
forked skill whose subagent is writing, as waiting on you. See
[Alerts and notifications](../../reference/alerts/).
