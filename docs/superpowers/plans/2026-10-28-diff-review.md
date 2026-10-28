# Diff review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Diff tab in the session view that shows the git diff of the session's directory, takes comments on lines, ranges and files, and sends them to the running agent through Claude Code hooks.

**Architecture:** A new `internal/review` package holds everything that isn't UI: running git and parsing diffs, the comment model and its text, the inbox, and editing Claude Code's settings. `cmd/snoop` adds `snoop hook` and `snoop hooks install|uninstall|status`. `internal/ui/diff.go` adds the tab, and all git work runs in a `tea.Cmd`.

**Tech Stack:** Go 1.26, Bubble Tea, lipgloss, `git` on `PATH`.

**Spec:** `docs/superpowers/specs/2026-10-27-diff-review-design.md`

## Global Constraints

- Opening and following a session must not get slower: `git` runs only for the Diff tab, never inside `Update`.
- snoop writes nothing until comments are sent, and touches Claude Code settings only through `snoop hooks install|uninstall`.
- `snoop hook` always exits 0 and prints nothing when there are no comments. It reads only its stdin and the inbox.
- A comment is never lost silently and never delivered twice.
- User-facing text follows the `writing-docs` skill. Commits have no attribution lines.

## Deviations from the spec (decided while planning)

- **The inbox is a directory per session with one file per send**, `<state>/snoop/inbox/<session-id>/<unix-nanos>.txt`, written to a temporary name and then renamed. A single appended JSONL file can't be claimed atomically while snoop is appending to it. The hook claims each file by renaming it to `<name>.claimed`, prints it, then deletes it. A `.claimed` file older than one minute is treated as orphaned and delivered again, because checking whether a PID is alive isn't portable.
- **Without hooks, `S` copies the comments to the clipboard right away and keeps them pending**, rather than offering `y`. This saves a keypress, and the comments are still there if you install the hooks and press `S` again.
- **Untracked files are read directly** (shown as all added, or as binary if the first 8000 bytes contain a NUL) instead of running one `git diff --no-index` per file. At most 200 untracked files are shown.

## Review Focus

1. **A repository with no commits yet:** `git diff HEAD` fails. Diff against the empty tree instead, so new files still show. (Task 1 test `TestLoadNoCommits`.)
2. **Paths with spaces or non-ASCII characters:** git quotes them. Run git with `-c core.quotePath=false` and unquote any `"…"` path. (Task 1 test `TestParseQuotedPath`.)
3. **Huge diffs** (a generated file, a lockfile): cap each file at 3000 lines and mark it truncated. (Task 1 test `TestParseTruncates`.)
4. **Settings that already have other hooks, or no settings file at all:** install must keep everything else, keep the order of top-level keys, and be idempotent. (Task 2 tests `TestInstallKeepsOtherSettings`, `TestInstallIdempotent`, `TestInstallCreatesFile`.)
5. **Two deliveries racing** (PostToolUse and Stop in quick succession, or two hooks at once): only one may print a given send. (Task 2 test `TestClaimOnce`.)

---

### Task 1: diffs, comments and the inbox (`internal/review`)

**Files:**
- Create: `internal/review/diff.go`, `internal/review/comment.go`, `internal/review/inbox.go`
- Test: `internal/review/diff_test.go`, `internal/review/comment_test.go`, `internal/review/inbox_test.go`

**Interfaces (produces):**
```go
type Line struct { Kind byte /* ' ', '+', '-' */; Old, New int /* 0 when absent */; Text string }
type Hunk struct { Header string; Lines []Line }
type File struct { Path string /* relative to Root, slash-separated */; Added, Removed int; Hunks []Hunk; Binary, Untracked, Deleted, Truncated bool }
type Diff struct { Root string; Files []*File }
var ErrNotRepo, ErrNoGit error
func Load(dir string) (*Diff, error)       // runs git; GIT_OPTIONAL_LOCKS=0
func Parse(out string) []*File             // a `git diff` unified diff

type Comment struct { Path string; Lines []Line /* empty: the whole file */; Text string }
func (c Comment) Covers(path string, l Line) bool
func Format(cs []Comment) string           // the text sent to the agent

func InboxDir() string                     // $XDG_STATE_HOME/snoop/inbox, ~/.local/state/…, macOS ~/Library/Application Support/snoop/inbox, Windows %AppData%\snoop\inbox
func Send(dir, session, text string, now time.Time) (path string, err error)
func Claim(dir, session string, now time.Time) (string, error) // "" when nothing waits
```

- [ ] **Step 1: Write the failing tests.**
  - `TestParse`: a fixed diff with a modified file (two hunks), a new file, a deleted file and a binary file gives four Files with the right paths, flags, Added/Removed and line numbers (for example, the first `+` line in `@@ -10,3 +10,4 @@` has `New == 11`, `Old == 0`).
  - `TestParseQuotedPath`: `+++ "b/dir/na\303\257ve file.go"` parses to `dir/naïve file.go`.
  - `TestParseTruncates`: a 3500-line hunk keeps 3000 lines and sets Truncated.
  - `TestLoad`: in a temp repo (`git init`, commit `a.go`, modify it, add untracked `new.txt`, `.gitignore` an `ignored.txt`), the result has `a.go` then `new.txt` (Untracked, all `+`), no `ignored.txt`, and Root equals the repo dir after `filepath.EvalSymlinks`.
  - `TestLoadNoCommits`: `git init` with one untracked file and one staged file shows both.
  - `TestLoadNotRepo`: a plain temp dir returns `ErrNotRepo`.
  - `TestFormat`: matches the spec's example exactly: `path:40-42`, quoted lines `    40 +\t…`, whole-file comments as `path (whole file)`, removed-only ranges with ` (removed)` and old numbers.
  - `TestSendClaim`: two sends, then Claim returns both texts in order, separated by a blank line, and a second Claim returns "".
  - `TestClaimOrphan`: a `.claimed` file whose mtime is 2 minutes old is delivered, and one 10 seconds old isn't.
  - `TestClaimOnce`: 8 goroutines Claim at once after 20 sends, and each text is returned exactly once in total.
- [ ] **Step 2: Run `go test ./internal/review/`.** Expected: FAIL to compile.
- [ ] **Step 3: Implement.** Load runs `git -c core.quotePath=false -C dir rev-parse --show-toplevel`, then `diff <base> --no-color --no-ext-diff -U3`, where `<base>` is `HEAD`, or the empty tree from `git hash-object -t tree --stdin` (empty stdin) when `rev-parse --verify -q HEAD` fails. Then `ls-files --others --exclude-standard -z`. `exec.ErrNotFound` maps to ErrNoGit, and a failing `rev-parse --show-toplevel` maps to ErrNotRepo. Send writes `<n>.tmp` and then renames it to `<n>.txt`. Claim renames each `*.txt`, in name order, to `*.txt.claimed`, and skips any it loses the race for.
- [ ] **Step 4: Run `go test -race ./internal/review/`.** Expected: PASS.
- [ ] **Step 5: Commit:** "Add internal/review: git diffs, review comments and the comment inbox".

### Task 2: hook commands (`snoop hook`, `snoop hooks …`)

**Files:**
- Create: `internal/review/settings.go`, `internal/review/hook.go`, `cmd/snoop/hooks.go`
- Modify: `cmd/snoop/main.go` (dispatch `hook` and `hooks`, the usage comment)
- Test: `internal/review/settings_test.go`, `internal/review/hook_test.go`

**Interfaces (produces):**
```go
var HookEvents = []string{"PostToolUse", "Stop", "UserPromptSubmit"}
func Install(settings, command string) (changed bool, err error)
func Uninstall(settings string) (changed bool, err error)
func Installed(settings ...string) bool   // true if any file has all three snoop hooks
func IsHookCommand(cmd string) bool        // ends in `snoop hook`, `snoop.exe hook`, quoted or not
func Hook(in io.Reader, out io.Writer, dir string, now time.Time) error
func UserSettings(projectsRoot string) string // dir(root)/settings.json
```

- [ ] **Step 1: Write the failing tests.**
  - `TestInstallCreatesFile`: a missing file gets `{"hooks":{…}}` with three events. PostToolUse has `"matcher":"*"`, and each entry is `{"type":"command","command":<cmd>,"timeout":10}`.
  - `TestInstallKeepsOtherSettings`: a file with `{"model":"x","hooks":{"PostToolUse":[<gofmt group>]},"env":{…}}` keeps `model`, `env`, the gofmt group (first) and the key order `model, hooks, env`.
  - `TestInstallIdempotent`: a second Install returns changed=false and leaves the bytes the same.
  - `TestUninstall`: removes only snoop's groups, drops events left empty, and drops `hooks` if it's left empty.
  - `TestInstalledInvalidJSON`: a malformed file returns false, and Install returns an error without writing.
  - `TestHookOutput`: with one send waiting, `PostToolUse` and `UserPromptSubmit` print `{"hookSpecificOutput":{"hookEventName":<event>,"additionalContext":<text>}}`, `Stop` prints `{"decision":"block","reason":<text>}`, and nothing waiting prints nothing.
  - `TestHookBadInput`: garbage on stdin returns an error and prints nothing.
- [ ] **Step 2: Run `go test ./internal/review/`.** Expected: FAIL to compile.
- [ ] **Step 3: Implement.** Settings use an ordered JSON object (key and `json.RawMessage` pairs, decoded with `json.Decoder.Token`) so untouched keys keep their order and content. Writes go to a temp file in the same directory and are then renamed, with `0o644` (or the old file's mode). In `cmd/snoop/hooks.go`, `snoop hook` calls `review.Hook(os.Stdin, os.Stdout, review.InboxDir(), time.Now())`, writes any error to stderr and exits 0. `snoop hooks install|uninstall|status [--project]` uses `"<abs os.Executable()>" hook` as the command and prints what changed. `--project` uses `./.claude/settings.json`.
- [ ] **Step 4: Run `go test -race ./internal/review/ ./cmd/...` and `go build ./cmd/snoop`.** Then smoke-test it: `echo '{"session_id":"s","hook_event_name":"Stop"}' | ./snoop hook` prints nothing.
- [ ] **Step 5: Commit:** "Add snoop hook and snoop hooks install, uninstall and status".

### Task 3: the Diff tab and comments in the UI

**Files:**
- Create: `internal/ui/diff.go`, `internal/ui/diff_test.go`
- Modify: `internal/ui/session.go` (tab 5, key routing, footer, render), `internal/ui/app.go` (tick refresh, delivery check, leave prompt, typing check), `internal/ui/help.go`, `internal/ui/copy.go` (`y` on the Diff tab), `README.md`, `site/content/docs/…`

**Interfaces (consumes):** everything from Tasks 1 and 2. **Produces:** `tabDiff`, `diffMsg{v *view; d *review.Diff; err error}`, `sentMsg`.

- [ ] **Step 1: Write the failing tests** (driving `App.Update` with keys, a temp git repo as the fixture session's `cwd`, and `XDG_STATE_HOME`/`CLAUDE_CONFIG_DIR` pointed at temp dirs):
  - `TestDiffTabLoads`: pressing `5` and then running the returned cmd shows the changed file and its `+` line in `View()`. Files the session edited are listed before the others.
  - `TestDiffNotRepo`: a non-repo cwd shows "not a git repository".
  - `TestCommentLineAndSend`: with hooks installed in the temp settings, the keys `5`, `tab`, `j`, `c`, then type "fix", `enter`, `S` result in one file in the inbox whose text contains `a.go:` and `fix`. The footer shows "sent 1 comment".
  - `TestCommentRange`: `v`, `j`, `j`, `c` produces a 3-line comment.
  - `TestFileComment`: `C` from the file list produces `(whole file)`.
  - `TestSendWithoutHooks`: `S` writes nothing to the inbox, keeps the comment pending, and flashes about `snoop hooks install`.
  - `TestDeleteEditComment`: `x` removes the comment under the cursor, and `e` puts its text back in the input.
  - `TestLeaveWithUnsent`: `esc` with a pending comment flashes the prompt and keeps the view. A second `esc` stays, `d` discards and leaves.
- [ ] **Step 2: Run `go test ./internal/ui/`.** Expected: FAIL.
- [ ] **Step 3: Implement.** The details:
  - **Layout:** a file list (40% when the terminal is wide) and a hand-drawn diff pane, so the cursor and selection are under snoop's control.
  - **Focus:** reuse `focusR` for "diff pane focused", so `esc` steps back the way it does on other tabs.
  - **Refresh:** on entering the tab, on `r`, and from the tick when the count of the session's Edit/Write/NotebookEdit calls changes, at most once every 2s, with a sequence number so only the newest result applies.
  - **Comment input:** a `textinput` reported through a new `v.inputActive()`, which the app uses where it now checks `v.typing`.
  - **Sending:** `S` runs `review.Installed` (user settings, plus `<cwd>/.claude/settings.json` and `settings.local.json`) and then `review.Send` in a cmd. The returned `sentMsg` updates the footer.
  - **Delivery status:** the tick marks a send delivered once its file is gone, and shows "waiting for the session to resume" when the session's last write is over 2 minutes old.
  - **Docs:** help sections and the README/site section "Review and comment".
- [ ] **Step 4: Run the `check` skill's commands, including the prose linter and the site build.** Expected: all pass.
- [ ] **Step 5: Commit:** "Add the Diff tab: review the working tree and send comments to the agent".
