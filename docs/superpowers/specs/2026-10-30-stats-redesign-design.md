# Stats tab redesign — design

Status: implemented 2026-10-30, after a spike the user reviewed.

## Context

The user called the Stats tab an eyesore. It had sixteen equal-weight label/value pairs in two columns, full-width bars for single entries (one subagent, one model), a 100-cell tool-mix slab, and the per-prompt breakdown at the bottom. Its cost cells were also blank in practice: Claude Code writes `cost-state` once, as the last record, when a session ends, so a running session (the one you usually look at) has no recorded cost.

## Layout

Same colors, more whitespace, read top to bottom:

1. **Headline:** cost, duration (with the prompt count), tokens (with the share read from cache) and context (with peak and compactions), each bold with a dim caption.
2. **Turns:** a table of each prompt's time, tokens, cost and context, a thin bar for its share of the cost (or of the time, without a cost), the costliest prompt highlighted, and `⟳` on turns that compacted.
3. **Tools** beside **Tokens** (stacked under 100 columns): the most used tools by name, with their kind's glyph and color, a count bar, errors and average time, up to seven; then input, output, cache read and cache write with bars.
4. **Models:** one per line, most used first, with requests and cost; then the pull request, when there is one.

Bars are thin lines (`━` on a faint `─` track) rather than blocks. The Claude Code version and the skill, MCP and subagent lists are gone; skills, MCP tools and subagents appear as tool rows.

## Cost while a session runs

The original design (2026-10-24) ruled out a price table in the binary. The blank costs make that the wrong trade, so `internal/parse/prices.go` now holds Anthropic's per-model API prices (as of 2026-10-06), matched by model-id prefix:

- Each request's usage is priced as it's read: input, output, cache reads at the model's read rate (0.025× input on Fable 5.1, 0.05× on Opus 5.5, 0.1× elsewhere), and cache writes at 1.25× input, or 2× for the part Claude Code reports as written to the 1-hour cache.
- `Session.Cost()` returns the recorded cost when there is one, else the sum of the estimates, flagged as estimated. `TurnCost` and `ModelCostUSD` follow the same rule. A recorded cost always wins.
- Estimates cover the session's own requests, not its subagents', so they can be lower than the recorded cost. Models without a price get no estimate.
- The UI marks estimates `≈`, the headline caption says "estimated", and amounts under a cent read `<$0.01`.
- The live board's cost column and sort, the session picker's summary, and the timeline's prompt rows use the same cost. The summary cache version is bumped so cached sessions are re-read.

## Calls made

- Prices live in the binary and need updating when Anthropic's prices change; the date is in the file.
- Haiku 5.5's higher rate for prompts over 100K tokens isn't modelled.
- Subagent requests aren't estimated; reading every subagent transcript for this would slow opening a session.
