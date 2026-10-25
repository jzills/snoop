---
title: Cost and context
weight: 2
---

snoop shows what each prompt cost, how full the context window is, and where
a turn's time went.

## Cost per prompt

Each prompt row in the timeline shows that turn's tokens, its estimated cost
(marked `≈`) and its duration. The Stats tab totals them for the session and
breaks them down by prompt.

{{< shot src="stats.png" alt="The Stats tab with duration, tokens, cost, context, tool mix, models and a per-prompt breakdown" caption="The Stats tab. Turns at the bottom lists each prompt's duration, tokens, cost and context." >}}

Cost per prompt is an estimate. Claude Code records only each model's total
cost for the session, so snoop splits that total across prompts by their
token use, weighted by each kind of token's price. The prompt costs add up
to the recorded total. Sessions without a recorded cost show tokens only.

## Context

The header shows how full the context window is:

```text
ctx 142k/200k ▰▰▰▰▰▰▰▱▱▱ 71%
```

The meter turns to the accent colour at 70% and red at 90%. The Stats tab
shows the current and peak context and the number of compactions, and each
compaction appears in the timeline with the context before and after it.

## Where the time went

Select a prompt row to see its turn in the detail pane: the totals, the five
slowest tool calls, and a waterfall that places each call by its start and
end. Calls that ran in parallel overlap.

{{< shot src="turn.png" alt="A prompt selected, with the turn's totals, slowest calls and a waterfall of its tool calls" caption="A turn's waterfall. Parallel calls overlap." >}}

## Across sessions

The summary in the session picker totals cost and tokens across sessions,
with a daily cost chart for the last 30 days. See
[Across sessions](../across-sessions/).
