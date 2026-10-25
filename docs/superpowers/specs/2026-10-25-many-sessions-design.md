# Many sessions at once — design

## Context

snoop follows one session at a time. Heavy Claude Code users run several sessions in parallel, often across projects, and the first thing they need is to know which one wants them.

This set makes snoop the place to see all of them:
- a **live board** of every active session;
- **desktop notifications**, so "waiting on you" reaches you from any session;
- **search across sessions** ("which session touched X?").

The user asked for the whole set now and will validate snoop as a whole before choosing a release date.

Anti-goal, carried over from the spec: opening and following a single session must not get slower. Idle sessions are never fully parsed by the board.

After approval, following the last feature set's process, I'll write:
- the design to `docs/superpowers/specs/2026-10-25-many-sessions-design.md`;
- the plan to `docs/superpowers/plans/2026-10-25-many-sessions.md`.

Then I'll build it in three steps, one commit each, with tests written first.

## Step 1: shared plumbing (refactors, no behaviour change)

**`store.Follower`** (new, `internal/store/follow.go`): `{Path, Sess *parse.Session, Off int64}`.
- `Track(path) (*Follower, error)` loads the file with `Load`.
- `(*Follower).Poll() (changed, reset bool)` stats the file. If the file shrank, it reloads it. If the file grew, it seeks to `Off` and calls `Sess.Read`.
- This is the logic in `subTranscript` / `view.sub` (`internal/ui/subagents.go`) lifted out, and `view.sub` is rewritten to use it.

**Alert watch**, extracted from `internal/ui/alerts.go` into `type watch struct{ pending map[*parse.Event]bool; unseenErrs int }`. It has three methods:
- `snap(s) liveState`
- `alerts(s, before) []*alert`
- `stalls(now, stall) []*alert`

Error counts come from a cheap `errorCount(s)` loop, not `stats.Compute`. `view` embeds `watch`, so existing behaviour and tests are unchanged.

**`parse.ToolCall.FilePath() string`** returns `file_path` or `notebook_path` for Read, Write, Edit, MultiEdit and NotebookEdit. `fileCall` in `internal/ui/files.go` then uses it for its path.

## Step 2: live board and notifications

**Entry points:** `snoop --live`, and the key `w` in the picker. `esc` or `w` goes back to the picker, and `enter` opens a session. `esc` from that session comes back to the board, which uses the existing view stack in `App`.

**Discovery** (`internal/ui/board.go`, plus `store.Recent(root, since) []string`, which globs and stats only):
- Every 5 s, any transcript written in the last 30 min becomes a row.
- A new row is loaded in a `tea.Cmd` goroutine (`store.Info` + `store.Track`), so a large transcript never blocks the UI.
- Each 1 s tick polls only the tracked rows' Followers, inside `Update`.
- A row's last write is the newest mtime among its transcript and its `subagents/*.jsonl`, checked at discovery. That way a session whose subagent is busy stays live.
- Rows drop out after 30 min without writes.

**State, in display order**, from `boardState(row, now, stall)`, which is a pure function and tested:
1. **waiting on you:** a non-Agent tool call with no result for longer than `--stall`. The detail reads "waiting 45s on Bash · may be a permission prompt".
2. **your turn:** the last turn is `Done` (from turn_duration). The detail is the time since it ended.
3. **working:** the detail is the running tool and its summary, `⤷ <type>` for a pending Agent call, or "replying".
4. **idle:** no prompts yet, or not done and no writes for 10 min (for example, interrupted).

Within each state, rows are newest first.

**Row** (responsive like the picker, reusing `fit`, `ago`, `contextMeter`, `costText` and the theme styles): state glyph and label, project base name, branch, session name, `ctx N%`, cost, age, detail. The header reads `snoop  Live · 4 sessions · 1 waiting on you`, followed by the latest alert.

**Alerts on the board:**
- `watch.alerts` runs around each Poll.
- `watch.stalls` runs each tick.
- Alert text is prefixed with the project name.
- A session that is also open in a view on the stack doesn't raise board alerts, so nothing is notified twice.

**Notifications** (`internal/ui/notify.go`):
- New flag `--notify`. It fires for the same alert kinds as `--bell` (`--alerts`, whose help text becomes "alerts that ring the bell or notify"), in both the board and single-session views. `App.bell` becomes `App.signal(as)`.
- The escape sequence is picked from the environment:
  - OSC 99 in kitty (`KITTY_WINDOW_ID`);
  - OSC 9 in iTerm2, WezTerm and Ghostty (`TERM_PROGRAM`);
  - OSC 777 otherwise.
- Inside tmux it's wrapped for passthrough (needs `allow-passthrough on`; documented), and inside screen with DCS.
- Control characters and `;` are stripped from the text.
- The title is "snoop" and the body is `<project>: your turn` or `<project>: <alert text>`.

## Step 3: search across sessions

**Index in the summary cache** (`internal/summary`): `Session` gains `Find *Index` (`json:"find,omitempty"`), holding:
- `Prompts`: the first line of each prompt, up to 200 runes;
- `Files`: every unique path, via `FilePath()`;
- `Commands`: unique Bash commands, first line, up to 200 runes, capped at 300.

`scanOne` also loads the session's subagent transcripts (`store.SubagentIDs`) into Files and Commands. `cacheVersion` becomes 2, so old caches are rebuilt.

**`summary.Find(sums map[string]Session, query string) []Hit`**, where `Hit{Path, Kind, Text, N}`. All the lowercased words of the query must match within one entry, the same rule as the timeline search. It returns the first matching entry and the match count for each session, newest first. It's a pure function and tested.

**Picker key `f`** opens a find input with the placeholder "find a file, command or prompt in any session":
- It starts the scan if one hasn't run, and results fill in as the scan progresses (`scanning n/m`).
- It respects `c` (this project only).
- Each row is the usual session row, with the name replaced by the hit: `▸ internal/ui/files.go`, `$ go test ./...` or `› fix the flaky…`, plus `+N more` if there are other matches.
- `enter` opens the session with the timeline search already set to the query, so the matching events, subagents included, are what's shown. `esc` clears the search as it does today.

## Docs and help

- Help sections in `internal/ui/help.go`: Live board, picker `w` and `f`, `--notify`.
- README:
  - a "Many sessions" section;
  - flags in the Use block;
  - a note on tmux passthrough.
- Site: a seventh card, "Every session at a glance", alongside the existing six.
- `RELEASING.md` is unchanged.

## Files

- **New:**
  - `internal/store/follow.go`
  - `internal/ui/board.go`
  - `internal/ui/notify.go`
  - `internal/ui/find.go`
  - each with a `_test.go`
- **Modified:**
  - `internal/ui/{alerts,subagents,files,app,picker,help,session}.go`
  - `internal/parse/parse.go`
  - `internal/summary/summary.go`
  - `cmd/snoop/main.go`
  - `README.md`
  - `site/content/_index.md`

## Verification

- **Unit tests:**
  - **Follower:** grow, a partial last line, and truncate → reset.
  - **`watch`:** existing alerts tests moved over unchanged.
  - **`boardState`:** each state, with Agent calls excluded from waiting, and stale → idle.
  - **`store.Recent` window:**
    - picks up subagent mtimes;
    - rows drop out after the window.
  - **Notify:** the bytes for each terminal and for tmux and screen wrapping; sanitising.
  - **`summary.Find`:** all words within one entry, kind priority, the `c` filter, and the N count.
  - **Index:** built from a fixture including a subagent edit; a cache with version 1 is discarded.
- **pty smoke test** (the existing Python driver), with a temporary `CLAUDE_CONFIG_DIR` and three seeded live sessions:
  - `snoop --live` shows three rows;
  - appending a turn_duration record to one moves it to "your turn" and emits the notify bytes;
  - a pending Bash older than `--stall 2s` shows "waiting on you";
  - picker `f` "files.go" lists the session that edited it;
  - `enter` opens with that search applied.
- **Regression:**
  - `go test -race ./...`, gofmt, vet;
  - `cmd/probe` open timings unchanged;
  - opening a single session is checked against the current numbers (about 590 ms for 264 MB).
- **Demo:** extend `demo/seed.py` so the board has something to show, and record a shot for the user to review, without replacing `demo.gif` unless they approve.
