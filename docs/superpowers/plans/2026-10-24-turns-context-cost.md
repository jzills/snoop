# Turns, Context Meter and Cost per Prompt: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build step 1 of the design: the parse-level turn model, context
for each request, compaction events and cost estimates, shown as a header
context meter, totals on each prompt row and a Turns section in Stats.

**Architecture:** `parse.Session` builds `Turn`s incrementally in `Feed`.
A turn is a typed prompt plus everything up to the next one. Each assistant
`Event` records the context size of its request. Cost per turn is worked out
when asked for, by splitting the recorded per-model `cost-state` costs by
weighted tokens. The UI only reads these values.

**Tech Stack:** Go 1.24, Bubble Tea, lipgloss, goccy/go-json (already in use).

**Spec:** `docs/superpowers/specs/2026-10-24-next-features-design.md` (the
"Foundation" section and section 1). The rename is out of scope.

## Global Constraints

- No new dependencies. No network calls. No model price table.
- Estimated costs are always shown with `≈`. With no `cost-state`, show
  tokens only.
- Token weights: input 1, cache write 1.25, cache read 0.1, output 5.
- Context of a request = `input + cache_read + cache_creation`.
- Context window: `contextUsage.raw_max_tokens` if seen; else
  `usageReport.session.model_usage[<latest model>].contextWindow`; else a
  model id containing `[1m]` gives 1,000,000; else any request above 200,000
  gives 1,000,000; else 200,000.
- Meter colours: dim below 70%, accent from 70%, error colour from 90%.
- Load time stays within ~10% of today's (`go run ./cmd/probe`).
- The project is not a git repo, so there are no commit steps.

## Review Focus

1. A session with no prompts, or with requests before the first prompt
   (resumed sessions): no turn, no panic, and the meter still shows.
2. A message id repeated across several records (one per content block):
   usage is counted once per turn, and `Ctx` is set on every event.
3. Models seen only in `cost-state` (used by subagents): their cost is
   spread by total weight, so the turn costs add up to the recorded total.
4. A tail reset (truncated file) rebuilds the turns from scratch, with no
   stale turns left over.
5. Narrow terminals: the prompt row drops cost, then tokens, and never
   overflows the row width.

Tests for these are added to Tasks 1–3.

---

### Task 1: Turns, request context and compactions in `parse`

**Files:**
- Create: `internal/parse/turns.go`
- Modify: `internal/parse/parse.go` (Event, Session, rawRecord, Feed,
  feedUser, feedAssistant)
- Test: `internal/parse/turns_test.go`

**Interfaces (produces):**
- `const KindCompact Kind` (added after `KindSystem`)
- `type Compaction struct { Trigger string; Pre, Post int }`
- `Event.Ctx int`: context of the request (assistant events only)
- `Event.Compact *Compaction`: set on `KindCompact` events
- `type Turn struct { Prompt *Event; Start, End time.Time; Recorded time.Duration; Done bool; Usage Usage; Requests, Tools int; ContextEnd int; Compacted bool; weight map[string]float64 }`
- `func (t *Turn) Duration() time.Duration`: `Recorded` when `Done`,
  otherwise `End - Start`
- `Session.Turns []*Turn`, `func (s *Session) TurnOf(e *Event) *Turn`
  (the turn whose `Prompt` is `e`, or nil)
- `Session.Context int` (latest request), `Session.PeakContext int`,
  `Session.Compactions int`
- `func (s *Session) ContextWindow() int`

Behaviour:
- A `KindUser` event starts a new turn, unless its UUID is the same as the
  current turn's prompt UUID (several text blocks in one message).
- An assistant message is counted the first time its id is seen:
  `Requests++`, usage added, `Ctx` computed, and its weight added to
  `turn.weight[model]` and `s.weight[model]`. Every event from that message
  gets that `Ctx`, so later records with the same id reuse the stored
  context (`s.msgCtx map[string]int`).
- `KindTool` events increase `turn.Tools`. Every event moves `turn.End`
  later.
- `system`/`compact_boundary` adds a `KindCompact` event with text
  `"compacted"`, sets `Compaction{Trigger, Pre: preTokens}`, increments
  `Compactions`, and marks the current turn `Compacted`. The next new
  request sets `Post`.
- `system`/`turn_duration` sets the current turn's `Done = true` and
  `Recorded`. It adds no event.

- [ ] **Step 1: Write the failing tests** in `turns_test.go`, using one JSONL
  fixture with two prompts, a repeated message id, a compaction, a
  `turn_duration`, and `contextUsage`:
  - `TestTurnsSplitOnPrompts`: 2 turns. `Turns[0].Prompt.Text == "first"`,
    and `TurnOf(Turns[1].Prompt) == Turns[1]`.
  - `TestTurnUsageCountsMessageOnce`: turn 0's `Usage` equals one copy of
    m1's usage plus m2's, `Requests == 2`, and every event from m1 has
    `Ctx == 130`.
  - `TestTurnDurationRecorded`: turn 0 `Done`, `Duration() == 21391ms`;
    turn 1 not `Done`, `Duration() == End-Start`.
  - `TestCompaction`: one `KindCompact` event with `Pre == 468293`,
    `Trigger == "auto"`, `Post` equal to the next request's context,
    `Compactions == 1`, and turn 1 `Compacted`.
  - `TestContextWindow`: a table test. `raw_max_tokens` 1000000 → 1M; a
    `[1m]` model → 1M; a peak of 250k → 1M; nothing → 200k.
  - `TestRequestsBeforeFirstPrompt`: assistant records with no prompt give
    no turns, and `Context` is still set.
- [ ] **Step 2:** Run `go test ./internal/parse/`. Expected: FAIL, because
  `Turns`, `KindCompact` and the rest are undefined.
- [ ] **Step 3:** Implement as described above. The `rawRecord` fields to
  add: `CompactMetadata *struct{Trigger string; PreTokens int}`,
  `DurationMs int`, `ContextUsage *struct{RawMax int "raw_max_tokens"}`,
  `UsageReport *struct{Session struct{ModelUsage map[string]struct{ContextWindow int}}}`.
  `ModelUsage` for `cost-state` is used in Task 2. Handle the
  `compact_boundary` and `turn_duration` subtypes in `Feed`'s `"system"`
  case before `feedSystem`.
- [ ] **Step 4:** Run `go test ./...`. Expected: PASS, with the existing
  `TestParse` unchanged.

### Task 2: Cost estimate per turn

**Files:** Modify `internal/parse/turns.go` and `internal/parse/parse.go`
(cost-state `modelUsage`). Test: `internal/parse/turns_test.go`.

**Interfaces (produces):**
`func (s *Session) TurnCost(t *Turn) (usd float64, ok bool)`, where `ok` is
false when no `cost-state` was seen. `Session.ModelCost map[string]float64`.

Algorithm: for each model m in `ModelCost`, if `s.weight[m] > 0` take
`cost_m * t.weight[m] / s.weight[m]`. Otherwise take
`cost_m * Σt.weight / Σs.weight`, which spreads the cost of subagent-only
models by total weight. If there is no `modelUsage` but there is a
`totalCostUSD`, use the second rule on the whole total. `weight(u)` is
`in + 1.25*cacheCreate + 0.1*cacheRead + 5*out`.

- [ ] **Step 1: Tests:**
  - `TestTurnCostSumsToTotal`: a cost-state with an opus model seen in the
    transcript plus a haiku model seen only in cost-state. The sum of
    `TurnCost` over the turns is within 1e-9 of `totalCostUSD`, and the turn
    with 4× the weight costs 4× as much.
  - `TestTurnCostWithoutCostState`: `ok == false`.
- [ ] **Step 2:** Run `go test ./internal/parse/`. Expected: FAIL.
- [ ] **Step 3:** Implement it, parsing
  `rawRecord.ModelUsage map[string]struct{CostUSD float64 "costUSD"}`.
- [ ] **Step 4:** Run `go test ./...`. Expected: PASS.

### Task 3: UI (header meter, prompt row totals, compaction rows, Stats Turns)

**Files:** Modify `internal/ui/session.go` (header, renderRowName),
`internal/ui/detail.go` (KindCompact), `internal/ui/stats.go` (Turns and
Context sections), `internal/ui/theme.go` (`glyphCompact = "⟳"`),
`internal/ui/help.go` if it lists glyphs, and `README.md`. Test:
`internal/ui/session_test.go`.

**Interfaces (consumes):** everything produced by Tasks 1–2.
**Produces:** `func contextMeter(used, window, w int) string` and
`func turnSummary(s *parse.Session, t *parse.Turn, w int) string` (the right
side of a prompt row) in a new `internal/ui/turns.go`.

- `contextMeter`: `ctx 142k/200k ▰▰▰▰▰▰▰▱▱▱ 71%` when `w >= 28`, otherwise
  `ctx 71%`. Empty when `used == 0`. Shown in the header before the
  live/active marker.
- `turnSummary`: `48.2k · ≈$0.31 · 2m10s`, where the tokens are
  `Usage.Total()`. Cost is dropped when `w < 22` and tokens when `w < 12`.
  The duration is omitted when it's zero. Prompt rows place it on the right,
  as tool rows place their duration.
- Compaction row: glyph `⟳` in `sDim`, text
  `compacted (auto) 468k → 31k`, with the `→ post` part omitted until it's
  known. The detail pane shows the same text plus the trigger.
- Stats: a "Context" line in the overview (`peak 512k · 1 compaction · now 31k / 1M`),
  and a "Turns" section with one line per turn: the prompt's first line,
  then a duration bar scaled to the longest turn, tokens, ≈cost, context at
  the end, and `⟳` when compacted.

- [ ] **Step 1: Tests:**
  - `TestContextMeter`: `(142000, 200000, 40)` contains `"142k/200k"` and
    `"71%"`; `(142000, 200000, 20)` gives `"ctx 71%"`; `used == 0` gives `""`.
  - `TestTurnSummaryNarrow`: for widths 40, 20 and 10, the `lipgloss.Width`
    of the result is ≤ the width; `"≈$"` is present at 40 and absent at 20.
  - `TestPromptRowFitsWidth`: render the fixture's prompt row at widths 50,
    80 and 140, each with `lipgloss.Width == w`.
- [ ] **Step 2:** Run `go test ./internal/ui/`. Expected: FAIL.
- [ ] **Step 3:** Implement as above. Update the README "What you see"
  section with the meter, the prompt totals and the Stats Turns section.
- [ ] **Step 4:** Run `go test ./...` and `go vet ./...`. Expected: PASS and
  clean.
- [ ] **Step 5:** Run `go run ./cmd/probe` before and after. Expected: total
  parse time within ~10% of the baseline.
- [ ] **Step 6:** Render the Stats tab and a timeline for a real session
  through the view (`v.render(140, 40)` in a scratch test, or run the
  binary) and check the meter, the prompt rows and the Turns section by eye.
