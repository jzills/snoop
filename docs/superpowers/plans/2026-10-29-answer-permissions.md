# Answering Permission Prompts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let snoop's live board and session view show a Claude Code permission request the moment it's made, and allow it once or deny it.

**Architecture:** `snoop hook` gains a `PermissionRequest` branch. When a snoop is listening (a heartbeat file), the hook writes the request to a per-session asks folder and waits briefly for an answer file, falling through to the normal prompt on timeout. The UI lists open requests in a `tea.Cmd` each tick, shows them as a new `asking` board state and an `ask` alert, and answers with `A` and `D`. Every handoff is a file renamed into place, as in the existing inbox.

**Tech Stack:** Go, Bubble Tea, bubbles/textinput, lipgloss; Claude Code hooks.

**Spec:** `docs/superpowers/specs/2026-10-29-answer-permissions-design.md`

## Global Constraints

- A timeout or error in the hook always prints nothing and exits 0, so the normal prompt appears. snoop never allows or denies on its own.
- With no snoop listening, the hook does no more than read its input and stat one file.
- Default `ask-wait` is 5m (the spike showed the terminal prompt never waits for the hook). Allowed range is 1s to 30m. The settings timeout for the `PermissionRequest` entry is `ask-wait + 10s` (310s by default).
- Claude Code kills the hook when you answer in the terminal, so a request whose hook PID (in its file name) is no longer running counts as gone.
- The hook polls for an answer every 100ms. A listening snoop rewrites the heartbeat every 2s, and the hook counts it as listening if it was written within 5s.
- The keys are `A` (allow once) and `D` (deny, with an optional reason). There is no "always allow" and no `updatedInput`.
- Output shapes are `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`, and `"behavior":"deny"` with optional `"message"`.
- No disk I/O added inside `Update`. Listing asks and writing answers or the heartbeat happen in `tea.Cmd`s.
- The Diff tab's hooks check keeps requiring only `PostToolUse`, `Stop` and `UserPromptSubmit`.
- User-facing text follows the `writing-docs` skill. Run the `check` skill before every commit. Commits use plain sentence messages with no attribution lines, like the existing log.

## Review Focus

1. **snoop answers just as the hook gives up.** The hook must not drop an answer that was claimed before its deadline, and must never print a decision after it has deleted its request. (Test in Task 2: `TestHookWaitsBrieflyForAClaimedAnswer`.)
2. **The listening file is missing, old or unparseable.** The hook treats every such case as not listening and exits at once. (Test in Task 1: `TestListening`.)
3. **A pre-existing install that lacks `PermissionRequest`.** `install` adds only that entry, and `status` names it as missing while the Diff tab still counts the hooks as installed. (Test in Task 3: `TestInstallUpgradesAnOlderInstall`.)
4. **Two requests from one session at once (parallel subagents).** `A` answers the oldest, and the detail reads `1 of 2`. (Test in Task 5: `TestAnswersTheOldestRequest`.)
5. **`A` pressed after the request was answered in the terminal or timed out.** The footer says `already answered`, and nothing is written. (Tests in Task 1: `TestRespondTwice`, and in Task 5: `TestLateAnswerSaysAlreadyAnswered`.)

---

### Task 0: Spike: does the terminal prompt wait for the hook? (done 2026-10-29; results in the spec)

Throwaway. Nothing from it is committed except the results, written into the spec.

**Files:**
- Create (scratch, not in the repo): a hook script and a settings file in a temporary project
- Modify: `docs/superpowers/specs/2026-10-29-answer-permissions-design.md` (add a "Spike results" section)

- [ ] **Step 1: Make a throwaway project.** In a temporary folder, create `.claude/settings.json` with a `PermissionRequest` hook (timeout 30) that runs a script. The script appends `date +%T` and its stdin to a log, sleeps 10s, appends `done`, and prints nothing.
- [ ] **Step 2: Have the user run `claude` there** and ask it to run `ls /tmp` (a command that prompts). Record whether the prompt appears at once or after 10s, and whether answering it during the sleep works and cancels the hook. The prompt needs a human, so if no human is available, ask the user to run this step.
- [ ] **Step 3: Repeat** with the script printing `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}` after 3s, then with the deny shape and a `message`, then with a request from a subagent. Record the input fields seen, especially `agent_id` and `agent_type`.
- [x] **Step 4: Check diff review.** Done by the user on 2026-10-29: a comment sent from the Diff tab reached the agent, which acted on it correctly.
- [ ] **Step 5: Write the results into the spec** under "Spike results". The prompt doesn't wait for the hook; with the user away and having asked for the work to go ahead, the default became 5 minutes (recorded in the spec).

### Task 1: The asks folder and the listening file

**Files:**
- Create: `internal/review/ask.go`
- Create: `internal/review/ask_test.go`
- Modify: `internal/review/inbox.go` (add `StateDir`, make `InboxDir` use it)

**Interfaces:**
- Produces:
  - `func StateDir() string`: the `snoop` folder that today's `InboxDir` lives in. `InboxDir()` becomes `filepath.Join(StateDir(), "inbox")`, with no change in behavior.
  - `func AsksDir() string` = `filepath.Join(StateDir(), "asks")`; `func ListeningFile() string` = `filepath.Join(StateDir(), "listening")`.
  - `type Request struct { ID string \`json:"-"\`; Session string \`json:"session_id"\`; Cwd string \`json:"cwd"\`; AgentID string \`json:"agent_id,omitempty"\`; AgentType string \`json:"agent_type,omitempty"\`; Tool string \`json:"tool_name"\`; Input json.RawMessage \`json:"tool_input"\`; Asked time.Time \`json:"asked"\`; Deadline time.Time \`json:"deadline"\` }`. `ID` is the file's base name, `<unix-nanos>-<pid>`.
  - `type Answer struct { Behavior string \`json:"behavior"\`; Message string \`json:"message,omitempty"\` }`
  - `var ErrAnswered = errors.New("already answered")`
  - `func Ask(dir string, r Request) (Request, error)`: writes `<dir>/<session>/<ID>.json` through a `.tmp` and a rename. It sets `ID` from `r.Asked` and the PID, and checks the session the way `sessionDir` does.
  - `func Pending(dir string, now time.Time) ([]Request, error)`: every `.json` request across sessions whose `Deadline` is after `now` and whose hook PID is still running (`alive(pid)`), oldest `Asked` first. Requests that are claimed, unreadable or past their deadline are left out, and a request whose hook has exited is deleted. It deletes any `.json`, `.claimed` or `.answer` file whose name's time is more than 40 minutes before `now`.
  - `var alive = func(pid int) bool`: on Unix, `os.FindProcess` plus `Signal(syscall.Signal(0))` (EPERM counts as alive); on Windows, whether `os.FindProcess` succeeds. It's a variable so tests can replace it. A missing `dir` returns nil, nil.
  - `func Respond(dir string, r Request, a Answer) error`: returns `ErrAnswered` if the hook PID has exited; otherwise renames `<ID>.json` to `<ID>.claimed` (returning `ErrAnswered` if the source is gone), then writes `<ID>.answer` through a `.tmp` and a rename.
  - `func Listen(path string, now time.Time) error`: writes `"<pid> <unix-nanos>\n"` through a `.tmp` and a rename.
  - `func Listening(path string, now time.Time) bool`: true when the file's time (parsed from its content) is within 5s before `now`.

- [ ] **Step 1: Write the failing tests** in `ask_test.go`, using `t.TempDir()` and the package's `t0`:
  - `TestAskAndPending`: `Ask` two requests for `s1` (asked `t0`, then `t0+1s`) and one for `s2`. `Pending(dir, t0+2s)` returns 3 in `Asked` order, each with `ID`, `Tool` and `Input` intact (compare `string(Input)` to `{"command":"ls"}`).
  - `TestPendingSkipsExpiredAndClaimed`: one request past its deadline and one claimed through `Respond`. `Pending` returns neither.
  - `TestPendingCleansLeftovers`: a `.json`, a `.claimed` and an `.answer` named 41 minutes before `now` are deleted, and one named 39 minutes before `now` is kept.
  - `TestPendingDropsDeadHooks`: with `alive` replaced to report the request's PID as exited, `Pending` leaves it out and deletes its file, and `Respond` returns `ErrAnswered`.
  - `TestAskRejectsBadSession`: sessions `""`, `".."` and `"a/b"` return an error and write nothing.
  - `TestRespondTwice`: the first `Respond` returns nil and `<ID>.answer` holds `{"behavior":"deny","message":"no"}`. The second returns `ErrAnswered`.
  - `TestListening`: false with no file, false for an empty or garbage file, true right after `Listen(p, t0)` when checked at `t0+4s`, and false at `t0+6s`.
- [ ] **Step 2: Run the tests to make sure they fail.** Run `go test ./internal/review -run 'Ask|Pending|Respond|Listening'`. Expected: FAIL, undefined names.
- [ ] **Step 3: Implement** `ask.go` and `StateDir` with the interfaces above. Reuse `sessionDir`, and follow `Send` for the `.tmp` and rename. Parse a file name's time the way `orphaned` does.
- [ ] **Step 4: Run the tests to make sure they pass.** Run `go test -race ./internal/review`. Expected: PASS, including the existing inbox tests.
- [ ] **Step 5: Commit.** Run `git add internal/review` and `git commit -m "Keep permission requests and their answers in an asks folder"`.

### Task 2: `snoop hook` answers PermissionRequest

**Files:**
- Modify: `internal/review/hook.go`
- Modify: `internal/review/hook_test.go`
- Modify: `cmd/snoop/hooks.go` (`hookCmd`)
- Modify: `cmd/snoop/main.go` (pass the args after `hook` to `hookCmd`, if the dispatch doesn't already)

**Interfaces:**
- Consumes: Task 1's `Ask`, `Listening`, `Answer`, `Request`, `AsksDir`, `ListeningFile`.
- Produces:
  - `type HookOptions struct { Inbox, Asks, Listening string; AskWait time.Duration; Now func() time.Time; Sleep func(time.Duration) }`
  - `func Hook(in io.Reader, out io.Writer, o HookOptions) error`. This replaces `Hook(in, out, dir, now)`. The comment events behave as before, with `o.Inbox` and `o.Now()`.
  - `const DefaultAskWait = 5 * time.Minute`
  - `func hookCmd(args []string)` parses `--ask-wait` (default `DefaultAskWait`).

The `PermissionRequest` branch:
1. If `!Listening(o.Listening, o.Now())`, return nil with no output.
2. Decode `tool_name`, `tool_input` (raw), `cwd`, `agent_id` and `agent_type` from the same input. Re-decode from a buffered copy, since `hookInput` reads the stream.
3. Call `Ask` with `Asked = now` and `Deadline = now + AskWait`.
4. Every 100ms (through `o.Sleep`), check for `<ID>.answer` until `o.Now()` passes the deadline.
5. At the deadline, try `os.Remove(<ID>.json)`. If it succeeds, print nothing and return. If the file is already gone (claimed), keep polling for the answer for up to 2s more, then give up silently.
6. With an answer, print the decision, delete `.answer` and `.claimed`, and return.

- [ ] **Step 1: Update the existing tests** to call `Hook` with `HookOptions{Inbox: dir, Now: func() time.Time { return t0 }}`. Then write the new failing tests. They use a fake clock: a `Now` that returns a variable which `Sleep` advances, so they run in no real time.
  - `TestHookNotListeningReturnsAtOnce`: no listening file. The output is empty, the asks folder doesn't exist, and `Sleep` was never called.
  - `TestHookAllows`: listening. A goroutine, or a `Sleep` hook on its first call, finds the request with `Pending` and calls `Respond(..., Answer{Behavior: "allow"})`. The output decodes to `hookSpecificOutput.hookEventName == "PermissionRequest"` and `decision.behavior == "allow"`, and no `message` key is present. The session's asks folder is left empty.
  - `TestHookDeniesWithMessage`: like the above, with `Answer{"deny", "use make test"}`. The output has `behavior` `deny` and `message` `use make test`.
  - `TestHookTimesOutSilently`: listening and never answered, with `AskWait` 15s. The output is empty, the `.json` is gone, and the fake clock advanced about 15s and never past 15.2s. (Also covers the claimed-answer grace: a request that isn't claimed gives up at the deadline.)
  - `TestHookWaitsBrieflyForAClaimedAnswer`: `Sleep` claims the request (renaming to `.claimed`) just before the deadline but writes the `.answer` 1s after it. The output is the allow decision.
  - `TestHookRequestFields`: input `{"session_id":"s1","hook_event_name":"PermissionRequest","cwd":"/w","agent_id":"a1","agent_type":"Explore","tool_name":"Bash","tool_input":{"command":"npm test"}}` produces a pending request with those fields and `Input` `{"command":"npm test"}`.
- [ ] **Step 2: Run the tests to make sure they fail.** Run `go test ./internal/review -run Hook`. Expected: FAIL.
- [ ] **Step 3: Implement** the branch and `HookOptions` in `hook.go`. In `hookCmd`, build the options from `InboxDir()`, `AsksDir()`, `ListeningFile()`, `time.Now` and `time.Sleep`. An unparseable `--ask-wait` falls back to the default, because the hook never fails.
- [ ] **Step 4: Run the tests to make sure they pass.** Run `go test -race ./internal/review ./cmd/...`. Expected: PASS.
- [ ] **Step 5: Commit.** Run `git add internal/review cmd/snoop` and `git commit -m "Let snoop hook wait for an answer to a permission request while snoop listens"`.

### Task 3: Installing the PermissionRequest hook

**Files:**
- Modify: `internal/review/settings.go`
- Modify: `internal/review/settings_test.go`
- Modify: `cmd/snoop/hooks.go` (`hooksCmd`)

**Interfaces:**
- Consumes: `DefaultAskWait` (Task 2).
- Produces:
  - `var CommentEvents = []string{"PostToolUse", "Stop", "UserPromptSubmit"}` and `var HookEvents = append(CommentEvents[:3:3], "PermissionRequest")`.
  - `func Installed(settings ...string) bool` now checks `CommentEvents` only.
  - `func Missing(settings ...string) []string`: the `HookEvents` that no settings file runs a runnable snoop hook for, in `HookEvents` order.
  - `func Install(settings, command string, askWait time.Duration) (bool, error)`. The `PermissionRequest` entry runs `command + " --ask-wait " + askWait.String()` with timeout `int((askWait + 10*time.Second).Seconds())`. The other entries are unchanged, at `hookTimeout`. `edit`'s change callback must build the command per event.
  - `reHookCmd` also matches an optional ` --ask-wait <duration>` before the end: `` (^|[/\\"' ])snoop(\.exe)?["']? hook( --ask-wait [0-9a-z.]+)?$ ``. `runnable` strips that suffix before finding the program.
  - `snoop hooks install --ask-wait 5m` rejects values outside 1s to 30m, with a usage error. `status` prints `Installed in X.` when nothing is missing, `Installed in X, without PermissionRequest; run snoop hooks install to add it.` when only that event is missing, and `Not installed in X.` (exit 1) otherwise.

- [ ] **Step 1: Write the failing tests:**
  - `TestInstallAddsPermissionRequest`: on an empty file, the `PermissionRequest` group's command ends in `hook --ask-wait 5m0s` with timeout 310. The other three keep timeout 10.
  - `TestInstallUpgradesAnOlderInstall`: a file with only the three comment groups (written by the old `Install`) plus an unrelated user hook. `Install` reports changed, adds only `PermissionRequest`, and leaves the other entries byte-identical. `Installed` was true before and after, and `Missing` returns `["PermissionRequest"]` before and nil after.
  - `TestInstallChangesAskWait`: `Install` with 15s and then 60s leaves one `PermissionRequest` group, with `--ask-wait 1m0s` and timeout 70.
  - `TestUninstallRemovesPermissionRequest`: everything snoop added is gone, and the user's hook stays.
  - Extend the existing `IsHookCommand` cases with `"/usr/local/bin/snoop" hook --ask-wait 15s` (true) and `snoop hook --other` (false).
- [ ] **Step 2: Run the tests to make sure they fail.** Run `go test ./internal/review -run 'Install|Uninstall|IsHook'`. Expected: FAIL.
- [ ] **Step 3: Implement** in `settings.go`, then update `hooksCmd` for `--ask-wait` and the new `status` text. The install message already prints `HookEvents`.
- [ ] **Step 4: Run the tests to make sure they pass.** Run `go test -race ./internal/review ./cmd/...`. Expected: PASS.
- [ ] **Step 5: Commit.** Run `git add internal/review cmd/snoop` and `git commit -m "Install the PermissionRequest hook with its own wait and timeout"`.

### Task 4: The UI listens and reads open requests

**Files:**
- Create: `internal/ui/ask.go`
- Create: `internal/ui/ask_test.go`
- Modify: `internal/ui/app.go` (tick handling, new messages, `Options`)
- Modify: `cmd/snoop/main.go` (fill the new `Options` fields)

**Interfaces:**
- Consumes: `review.Pending`, `review.Listen`, `review.Request`.
- Produces:
  - `Options` gains `AsksDir, ListeningFile string`. When either is empty, nothing listens, as in existing tests. `main.go` sets them from `review.AsksDir()` and `review.ListeningFile()`.
  - `type asksMsg struct{ reqs []review.Request; err error }` and `type listenedMsg struct{}`.
  - `App` gains `asks map[string][]review.Request` (by session ID, oldest first), `asksPolling bool`, `lastListen time.Time` and `seenAsks map[string]bool` (request IDs already alerted).
  - `func (a *App) listening() bool`: true when `a.board != nil || len(a.stack) > 0`.
  - On each `tickMsg`, if `a.listening()`: when 2s have passed since `lastListen`, add a `tea.Cmd` running `review.Listen`. When `!asksPolling`, add a `tea.Cmd` running `review.Pending(a.opts.AsksDir, now)`.
  - `func (a *App) onAsks(m asksMsg) []*alert`: replaces `a.asks`, and returns one `ask` alert for each request ID not in `seenAsks`, then marks them seen. Task 6 fills in the alert text and routing.
  - `func askSummary(r review.Request) string`. Bash gives `Bash: <command>`, Edit and Write give `<Tool>: <file_path>`, WebFetch gives `WebFetch: <url>`, and an `mcp__server__tool` name gives `server·tool`. Anything else gives the tool name. ` via <AgentType>` is appended when `AgentID` is set, with `subagent` used when `AgentType` is empty. The result goes through `oneLine`.

- [ ] **Step 1: Write the failing tests** in `ask_test.go`:
  - `TestAskSummary`: a table with the cases above, including `mcp__github__create_issue` giving `github·create_issue` and a subagent request giving `Bash: ls via Explore`.
  - `TestListensOnlyWithABoardOrSession`: in an `App` with temp `AsksDir` and `ListeningFile` on the picker, a tick plus running the returned cmds writes no listening file. With the board open, it writes one.
  - `TestAsksArriveOnTick`: with the board open and a request written with `review.Ask`, running the tick's cmds and feeding the `asksMsg` back fills `a.asks["s1"]`.
- [ ] **Step 2: Run the tests to make sure they fail.** Run `go test ./internal/ui -run 'AskSummary|Listens|AsksArrive'`. Expected: FAIL.
- [ ] **Step 3: Implement** in `ask.go` and `app.go`. The cmd runners in tests should reuse the helper the existing UI tests use to run a `tea.Cmd` and feed its message back, if there is one.
- [ ] **Step 4: Run the tests to make sure they pass.** Run `go test -race ./internal/ui`. Expected: PASS.
- [ ] **Step 5: Commit.** Run `git add internal/ui cmd/snoop` and `git commit -m "Listen for permission requests while the board or a session is open"`.

### Task 5: Asking on the board, and A and D

**Files:**
- Modify: `internal/ui/board.go`
- Modify: `internal/ui/ask.go` (the shared answerer)
- Modify: `internal/ui/session.go` (keys and the alert line)
- Modify: `internal/ui/app.go` (routing keys while typing a reason, `answeredMsg`)
- Test: `internal/ui/ask_test.go`, `internal/ui/board_test.go`

**Interfaces:**
- Consumes: `a.asks`, `askSummary`, `review.Respond`, `review.ErrAnswered`.
- Produces:
  - `rowAsking` becomes the first `rowStatus`, ahead of `rowWaiting`. `String()` gives `"asking"`, and `render` gives `sAccent.Bold(true)` with `"? asking"`.
  - `func (b *board) arrange(now time.Time, stall time.Duration, asks map[string][]review.Request)`. A row whose `info.ID` has requests gets `rowAsking`, with detail `askSummary(oldest)`, plus `  (1 of N)` when N > 1. The header counts asking rows in `N waiting on you`. `view` passes `asks` through, so its signature gains the same parameter.
  - On the board, with an asking row selected, the full request renders below the list. That's the Bash command and its `description` field, the Edit diff using `detail.go`'s rendering, or pretty-printed `tool_input` otherwise, plus `cwd`. Placement follows the TUI look rules: whitespace before density, and keep the colors.
  - `type answerer struct { input textinput.Model; typing bool; target review.Request }` in `ask.go`, with:
    - `func (an *answerer) allow(r review.Request, dir string) tea.Cmd`
    - `func (an *answerer) startDeny(r review.Request)`
    - `func (an *answerer) key(k tea.KeyMsg, dir string) (tea.Cmd, bool)`: `enter` sends the deny with the input as the message, `esc` cancels, and other keys go to the input. It returns whether it handled the key.
    - `func (an *answerer) view(w int) string`: the `deny reason (optional): …` line while typing.
  - `type answeredMsg struct { allow bool; err error }`. The App's handler sets the flash or footer to `allowed`, `denied`, `already answered` (for `ErrAnswered`), or `answer failed: <err>`.
  - Keys: `A` and `D` on the board act on the selected row's oldest request. In the session view, they act on the oldest request for `v.sessionID()`. They work in every tab, but never while the search, comment or deny input has focus. With no request open, the keys do nothing. The App owns a single `answerer`, and while it's typing, the App sends every key to it first.

- [ ] **Step 1: Write the failing tests:**
  - `TestRowAskingComesFirst` (board_test): two rows, one stalled (waiting) and one with a request. `arrange` puts the asking row first, with status `asking` and detail `Bash: npm test`. The header has `2 waiting on you`.
  - `TestAnswersTheOldestRequest`: a session with two requests shows `(1 of 2)`. `A` plus running the cmd leaves only the older request's `.answer`, holding `{"behavior":"allow"}`.
  - `TestDenyWithReason`: `D`, then typing `use make`, then `enter` writes `{"behavior":"deny","message":"use make"}`. Pressing `D` and then `esc` writes nothing. While typing, `q` goes into the input and doesn't quit.
  - `TestLateAnswerSaysAlreadyAnswered`: the request is removed from disk after it's listed. `A` gives a footer with `already answered`.
  - `TestAInSessionView`: in an open session view whose `sessionID()` has a request, `A` writes an allow answer. In the search input, `A` types the letter instead.
  - `TestNoAskNoEffect`: `A` and `D` with no requests write nothing, and `D` opens no input.
- [ ] **Step 2: Run the tests to make sure they fail.** Run `go test ./internal/ui -run 'Asking|Oldest|Deny|Late|AInSession|NoAsk'`. Expected: FAIL.
- [ ] **Step 3: Implement** the board state and full-request rendering, the answerer, key routing in `app.go` and `session.go`, and the footer messages. Update the board footer's `keyHelp` to show `A allow  D deny` when the selected row is asking.
- [ ] **Step 4: Run the tests to make sure they pass.** Run `go test -race ./internal/ui`. Expected: PASS, with the existing board tests updated for the new `arrange` and `view` parameter.
- [ ] **Step 5: Commit.** Run `git add internal/ui` and `git commit -m "Show permission requests on the board and answer them with A and D"`.

### Task 6: The ask alert and notifications

**Files:**
- Modify: `internal/ui/alerts.go`
- Modify: `internal/ui/ask.go` (`onAsks` builds the alerts)
- Modify: `internal/ui/app.go` (raise them on the board or the open view, and `signal` them)
- Modify: `cmd/snoop/main.go` (the `--alerts` default)
- Test: `internal/ui/ask_test.go`, `internal/ui/notify_test.go`

**Interfaces:**
- Consumes: `onAsks` (Task 4), `askSummary`, `a.signal`, `v.own`, `v.raise`, `noteTitle`.
- Produces:
  - `const alertAsk = "ask"`, added to `AlertKinds`. `render` uses `sAccent.Bold(true)`.
  - The alert has text `? ` + `askSummary(r)`, note `<Tool> wants to run <command>` for Bash and `<Tool> wants permission` otherwise, and `ev` set to the newest event in that session with `Tool.Name == r.Tool && !Tool.HasResult`, or nil if none. `key()` uses `path + "|ask|" + r.ID`.
  - Routing: if a session view for that session is on the stack, the view raises the alert and `own` marks it. Otherwise the board raises it, prefixed `<project>: `, with `path` and `title` set as in `board.poll`. Either way, the alert goes through `a.signal`.
  - The `--alerts` default becomes `"error,turn,ask"`.

- [ ] **Step 1: Write the failing tests:**
  - `TestAskAlertOnBoard`: a new request raises an alert whose text starts with `api: ? Bash: npm test`. A second tick with the same request raises nothing.
  - `TestAskAlertInSessionView`: with the session open, its header shows `? Bash: npm test`, and `!` moves the cursor to the unfinished Bash row from a JSONL fixture.
  - `TestAskNotifies` (notify_test, with the fake notifier that test file already uses): the note is `Bash wants to run npm test`, and the title is the session's.
- [ ] **Step 2: Run the tests to make sure they fail.** Run `go test ./internal/ui -run 'AskAlert|AskNotifies'`. Expected: FAIL.
- [ ] **Step 3: Implement** the alert and its routing, and the new `--alerts` default.
- [ ] **Step 4: Run the tests to make sure they pass.** Run `go test -race ./...`. Expected: PASS.
- [ ] **Step 5: Commit.** Run `git add internal/ui cmd/snoop` and `git commit -m "Alert and notify when a session asks for permission"`.

### Task 7: Docs, help and the screenshot

Load the `writing-docs` and `site` skills first.

**Files:**
- Modify: `README.md` (a section on answering permission prompts, the board state table, keys, `snoop hooks install --ask-wait`, `--alerts`)
- Modify: the matching pages under `site/content/docs/` (the `writing-docs` skill lists which copies to keep in sync)
- Modify: `internal/ui/help.go` (`A` and `D` on the board and in sessions) and its test
- Modify: `cmd/snoop/hooks.go` usage text (it mentions only comments today)
- Modify: `demo/` (a board shot with an asking row, from fictional sessions, written into a fictional `XDG_STATE_HOME`)

- [ ] **Step 1: Write the docs.** Cover what you see, `A` and `D`, the 5-minute wait and `--ask-wait`, the `listening` file and that a closed snoop never delays a prompt, the non-interactive deny caveat, and exactly what the hook writes.
- [ ] **Step 2: Update the help** and run `go test ./internal/ui -run Help`. Expected: PASS.
- [ ] **Step 3: Re-render the screenshots** as the `site` skill describes, and look at the new board shot.
- [ ] **Step 4: Run the whole `check` skill,** including the site build and the prose linter. Expected: every check is clean.
- [ ] **Step 5: Mark the spec implemented.** In its Status line, add the date and any deviations, such as the 10-minute leftover rule in place of "deadline plus a minute", and anything the spike changed.
- [ ] **Step 6: Commit.** Run `git add README.md site internal/ui cmd/snoop demo docs/superpowers` and `git commit -m "Document answering permission prompts from snoop"`.
