# Next feature set: design

Date: 2026-10-24. Status: draft for review.

The tool has been renamed from `ccscope` to **`snoop`**. The name isn't tied
to one agent, which leaves room for other coding agents later. Everything Claude Code-specific stays in `parse` and
`store`, so another agent could be added as a separate parser producing the
same events. No layer for multiple agents is built now.

## Intent

The tool already shows *what* happened in a session. This round answers the
questions it leaves open:

- **Live:** how full is the context, what is each prompt costing, did
  something break or stall, is it my turn?
- **After the fact:** which files changed and how, where did the time and
  money go, what did the subagents actually do, and what does my usage
  look like across sessions?
- **Sharing:** get a command, diff or whole session out of the TUI.

## Scope

1. Context and cost per prompt (plus a context meter, and compaction markers)
2. Files tab
3. Alerts while live
4. Where the time went (each prompt, and a tool waterfall)
5. Totals across sessions in the picker
6. Copy and export
7. Subagents expandable inline in the timeline

Non-goals: editing or resuming sessions, comparing two sessions, any network
calls, a model price table baked into the binary.

Anti-goals: anything that makes opening or following a session noticeably
slower, since a 75 MB transcript loads in ~150 ms today. Showing numbers that
look exact but are guesses: every estimate is marked `≈`.

## What the transcripts provide (checked against local sessions)

- `assistant` records: `message.usage` on every API request (input, output,
  cache read, cache creation). The context size of a request is
  `input + cache_read + cache_creation`.
- `system` / `subtype: compact_boundary`: `compactMetadata.trigger`
  (`manual`/`auto`), `preTokens`, `durationMs`.
- `system` / `subtype: turn_duration`: `durationMs`, `messageCount`. It marks
  the end of a turn, meaning Claude is now waiting for the person.
- `cost-state`: `totalCostUSD` plus `modelUsage[model].costUSD` and the token
  counts for each model. **There is no cost per message.**
- `system` / `local_command` sometimes carries `contextUsage.raw_max_tokens`
  and `usageReport.model_usage[m].contextWindow`, which give the context window
  size.
- `user` records: `promptId`, `permissionDecision`, `toolDenialKind`
  (for example `automode-blocked`).
- Permission prompts that are still *waiting* are not recorded. A tool waiting
  for approval looks the same as a slow tool.

## Foundation: turns and per-request usage (parse package)

Add a `Turn` to `parse.Session`: one prompt the person typed (`KindUser`),
plus everything up to the next prompt.

- `Turn.Prompt *Event`, `Start`, `End`, `Duration` (taken from
  `turn_duration` when present, otherwise the time from the prompt to the
  last event), `Done bool` (set when `turn_duration` arrives).
- `Turn.Usage` (summed over its API requests, each counted once by message
  id, as `Session.Usage` already is), `Turn.Requests`, `Turn.Tools`.
- `Turn.ContextEnd`: the context size of the turn's last request.
- `Turn.Compacted`: the turn holds a compaction.
- Every assistant `Event` gets `Ctx int`, the context size of the request it
  came from, so the meter can be drawn at any point in the timeline.

Parse `compact_boundary` into a new `KindCompact` event, shown in the
timeline as `⟳ compacted (auto) 468k → 31k`. The "after" figure is the next
request's context size. Parse `turn_duration` into the turn, not into a
visible event. Store the cost-state `modelUsage` for each model.

**Context window size** (`Session.ContextWindow()`), first match wins:
`contextUsage.raw_max_tokens` or `usageReport…contextWindow` if seen; a
model id containing `[1m]`; any request above 200k (then 1M); otherwise 200k.

**Cost per turn** (estimate, `≈`): split each model's recorded `costUSD`
across turns in proportion to the turn's weighted tokens for that model.
The weights are input 1, cache write 1.25, cache read 0.1 and output 5. These
ratios hold across Claude models, so no price table is needed, and the turn
costs add up exactly to the recorded total. When there is no `cost-state`,
show tokens only and no cost.

This all happens incrementally in `Feed`, so tailing stays cheap.

## 1. Context and cost per prompt

- **Header meter** (every session view): `ctx 142k/200k ▰▰▰▰▰▰▰▱▱▱ 71%`. Dim
  below 70%, accent from 70%, error colour from 90%. It narrows to `ctx 71%`
  on small terminals.
- **Prompt rows** in the timeline show, on the right where tool rows show
  duration, the turn's `tokens · ≈$cost · duration` (for example
  `48.2k · ≈$0.31 · 2m10s`). A turn still in progress shows a running total.
- **Stats tab**: a new "Turns" section with one line per prompt. Each line
  has the prompt's first line, a duration bar, tokens, ≈cost and the context
  at the end of the turn, with compactions marked `⟳`. A "Context" line shows
  the peak context, the number of compactions and the current context.

## 2. Files tab (tab 4)

The tab order becomes `1 Timeline  2 Tools  3 Stats  4 Files`, so the
existing numbers don't change.

- **Source:** Read, Write, Edit, MultiEdit and NotebookEdit calls, including
  calls made by subagents (their transcripts are on disk next to the session;
  the rows are marked `⤷`). Paths are relative to the session's working
  directory; paths outside it stay absolute.
- **List:** one row per file, sorted by most recent activity, with columns
  for reads, edits, writes, `+lines −lines` and errors. The `/` search filters
  by path. Files that were only read are dimmed and come last.
- **Detail pane:** every change to the file in order, as diffs (reusing
  `detail.go`'s `diff`), each with a timestamp and ✓/✗.
- `enter` jumps to that file's most recent change in the timeline;
  `n`/`N` in the detail pane move between changes.
- **Known gap**, stated at the bottom of the tab: files changed by Bash
  commands are not tracked.

## 3. Alerts while live

These run in any view that is receiving new lines, not only `--follow`.

| Alert | Trigger |
| --- | --- |
| `✗ error` | a tool result with `is_error` |
| `◌ waiting 30s` | a tool with no result for longer than `--stall` (default 30s). The text says it "may be a permission prompt" |
| `✓ your turn` | `turn_duration` arrives |
| `⟳ compacted` | a compaction |
| `ctx 90%` | the context crosses 90% |

- The most recent alert shows at the right of the header until a key is
  pressed or a newer alert arrives. Errors also keep a count of unseen errors
  (`2✗`) until you look at them.
- `!` jumps to the event that raised the most recent alert.
- `--bell` rings the terminal bell on alerts (off by default). An `--alerts`
  list chooses which alerts ring the bell, defaulting to `error,turn`.
- Alerts are never raised while the backlog loads, only for lines that arrive
  live.

## 4. Where the time went

- Selecting a **prompt row** in the timeline adds a "Turn" summary to the
  detail pane below the prompt text: duration, API requests, tool time, tokens
  and ≈cost, then the 5 slowest tool calls, then a **waterfall**. The
  waterfall has one line per tool call, and each bar is placed by its start
  and end within the turn, so parallel calls and subagents visibly overlap.
  It is coloured by tool kind, and in-progress calls extend to "now".
- The Stats tab's "Turns" section (from item 1) is the session-wide view of
  the same data.

## 5. Totals across sessions (picker)

- `s` in the picker switches between the session list and a **Summary**. The
  Summary covers the sessions currently shown, so it respects the `c`
  (this project only) toggle and the fuzzy search.
- **Contents:** session count, total ≈cost and tokens, tool calls and error
  rate; daily cost and tokens for the last 30 days (bar chart); top tools,
  skills, MCP servers and subagent types with error rates; models.
- **Data:** a full parse of every transcript, run in the background with
  bounded concurrency (as `store.List` does). A progress line reads
  `scanning 12/48`, and the summary fills in as results arrive. Results are
  cached in `os.UserCacheDir()/snoop/summaries.json`, keyed by
  path + size + mtime, so later opens are instant. Each cached entry holds the
  counts, the usage per model, the cost per model, and buckets per day (by
  message timestamp, local time).
- A failed or unreadable transcript is skipped and counted (`2 unreadable`);
  it never blocks the summary.

## 6. Copy and export

- `y` copies the selected item to the clipboard over OSC 52, which works over
  SSH and in tmux with `set-clipboard on`. What gets copied: the command for
  Bash, the diff for Edit/MultiEdit, the file content for Write, the full
  text for messages, and the result for other tools. In the Files tab it
  copies the file's combined diff. The footer confirms `copied 1.2k chars`.
- `e` exports the current view (the timeline as filtered and searched) to
  Markdown at `./<session-id-8>-<YYYYMMDD-HHMM>.md`, and the footer shows the
  path. Prompts become headings, replies become text, tool calls become
  collapsible `<details>` blocks with their input and a capped result
  (2,000 lines), and thinking is included only if it's being shown.
- CLI: `snoop export <id|path> [-o file]` writes the same Markdown for the
  whole session without opening the TUI.

## 7. Subagents inline

- On an Agent (or forked Skill) row in the timeline, `x` expands it in place:
  the subagent's events appear indented beneath it with a `│` rule in the
  agent colour. `x` again collapses it, and `X` expands or collapses all.
  `a`/`o` still open the subagent as its own full view.
- The subagent transcript loads when first expanded, and is tailed while it
  is expanded and the subagent is still running. Nested subagents can be
  expanded the same way.
- The search, the filter and `t` (thinking) apply to inline events too. A
  search match inside a collapsed subagent marks its row with `●`, but does
  not expand it.
- The detail pane, `y` and `e` work on inline events as on any other. The
  export includes expanded subagents nested under their call.

## Key and help changes

New keys: `x`/`X` (timeline), `y`, `e`, `!` (session views), `4` (Files tab),
`s` (picker). `?` help, the footer hints and the README are updated for each
feature as it lands. None of the existing bindings change.

## Build order

Each step is its own plan and ships working.

0. **Rename** to `snoop` (done 2026-10-24): module path
   `github.com/jzills/snoop`, `cmd/snoop`, the header title, error prefix and
   README. The project folder is still `idea/ccscope`. This goes first so later work doesn't need rewording.
1. Foundation (turns, per-request usage, compactions) + item 1
2. Item 4 (builds directly on turns)
3. Item 2, Files tab
4. Item 3, alerts
5. Item 7, subagents inline
6. Item 6, copy and export (comes after 7 so the export includes expanded
   subagents)
7. Item 5, totals across sessions (the largest step: background scanning and
   a cache)

## Testing

Follow the existing pattern: table tests with small JSONL fixtures in
`parse`, and model-level tests in `ui` (as `session_test.go` and
`groups_test.go` do). Each step adds fixtures for its record types
(`compact_boundary`, `turn_duration`, `cost-state` with `modelUsage`,
subagent files on disk). `cmd/probe` gains timings for building turns, to
confirm load time stays within ~10% of today's.

## Calls made by the designer (not discussed)

- Cost per turn comes from splitting the recorded cost by weighted tokens,
  not from a price table.
- Files is tab 4 rather than being placed before Stats, so the existing tab
  numbers keep working.
- Expanding subagents inline uses `x`, because `space` already means page
  down.
- The stall alert defaults to 30s and is labelled "may be a permission
  prompt", because waiting prompts aren't recorded.
- Exports go to the current directory, with a `snoop export` command for
  scripts.
- The summary cache lives under the user cache directory.

## Left to the builder

The exact layout and column widths, the waterfall's character set, how the
meter collapses on narrow terminals, and the internal type names.
