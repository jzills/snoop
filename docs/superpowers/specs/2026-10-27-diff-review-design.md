# Diff review — design

Status: implemented 2026-10-28. Deviations: the inbox is a directory per session with one file per send, so a send can be claimed atomically; without hooks, S copies the comments right away and keeps them; untracked files are read directly (at most 200); the guide says a running session may need a restart to pick up new hooks.

## Context

snoop shows what an agent did. It doesn't let you respond to it. To review an agent's changes today, you open the diff in another tool, then type into the agent's terminal something like "in store.go around line 40, don't swallow the error".

This feature set closes that loop. In snoop you can:
- see the **git diff** of a session's working directory;
- **comment** on a line, a range of lines or a whole file;
- **send** the comments, and the agent receives them on its own, without you touching its terminal.

The user ranks this above anything else on the roadmap, and wants it inside snoop rather than as a separate tool. snoop already knows each session's ID, directory and whether it's still running, and that's what sending comments back depends on.

## Goals

- Review an agent's changes and steer it from one place.
- Comments reach a running agent within one tool call, or at the end of its turn, with no copy and paste.
- Every comment names the file and lines, and quotes the code, so the agent can act on it without re-reading the file.

## Non-goals

- Staging, committing, reverting or editing code. This is review, not a git client.
- Diffs against branches or commits other than `HEAD`.
- Threads, replies or comment history. A comment is sent once and forgotten.
- Delivering comments to agents other than Claude Code.

## Anti-goals

- **Opening and following a session must not get slower.** `git` runs only when the Diff tab is opened or refreshed, always in a `tea.Cmd`.
- **No surprise writes.** snoop writes nothing until you send comments. It changes Claude Code's settings only when you run `snoop hooks install`. The README states exactly what snoop writes.
- **A comment must never be lost silently, or delivered twice.**

## Part 1: the Diff tab

A fifth tab, **Diff**, in the session view, after Files.

**Source.** In the session's `cwd`, snoop runs:
- `git diff HEAD --no-color --no-ext-diff -U3` for tracked changes;
- `git ls-files --others --exclude-standard` for untracked files, each shown as all added (`git diff --no-index /dev/null <file>`).

If `cwd` isn't a git repo, or `git` isn't installed, the tab shows that in one line instead.

**Layout.** It matches the Files tab: a list of changed files on the left (path relative to the repo root, with `+a −r` and a marker for files that have comments) and the selected file's diff on the right. Colors reuse the `-`/`+` styles in `internal/ui/detail.go`, and hunk headers are dimmed.

**Whose change.** The diff shows the whole working tree, which includes your edits and other agents' edits. Files this session touched (taken from the same tool calls the Files tab uses) are listed first, and other files are marked as not from this session. Attribution is per file, not per hunk.

**Refresh.** `r` re-runs git. The tab also refreshes when it's opened and when the session's Files list changes, debounced to no more than once every 2 seconds.

**Navigation.** `j`/`k` move the cursor by line within the diff, and `J`/`K` move by hunk. `tab` or `enter` switches focus between the file list and the diff. Deleted lines and context lines can be selected, but hunk headers can't.

## Part 2: comments

**Keys** (in the Diff tab):
- `c` comments on the line under the cursor;
- `v` starts a range selection, the cursor extends it, and `c` comments on the range;
- `C` comments on the whole file (works from the list or the diff);
- `x` on a commented line deletes that comment, and `e` edits it;
- `S` sends every pending comment for this session (see Part 3).

Input is a single-line `textinput`, as in search. `enter` saves and `esc` cancels.

**Pending comments** belong to the session and live in memory until they're sent. A gutter marker shows commented lines. The footer shows `3 comments · S send`. When you leave the session with unsent comments, snoop asks once whether to send them, discard them, or stay.

**Line numbers.** Each comment records the new-file line numbers (old-file for deleted lines, labeled `(removed)`) and the quoted lines as they appear in the diff. Because the code is quoted, the comment still makes sense if the file changes before the agent reads it.

**Format sent to the agent:**

```
Review comments from snoop:

internal/store/store.go:40-42
    40 +	if err != nil {
    41 +		return nil
    42 +	}
Don't swallow the error; return it.

internal/ui/board.go (whole file)
This should live in its own package.
```

## Part 3: getting comments to the agent

**Inbox.** On send, snoop appends one JSON record per send to `<state>/snoop/inbox/<session-id>.jsonl`. `<state>` is `$XDG_STATE_HOME`, or else `~/.local/state`; on macOS it's `~/Library/Application Support`. The record holds the formatted text and a timestamp.

**Hook command.** `snoop hook` reads Claude Code's hook input on stdin, which includes `session_id` and `hook_event_name`. If that session's inbox exists, the command claims it by renaming it to `<id>.jsonl.<pid>` (so snoop and two hooks can't race), reads it, deletes it, and prints the comments in the shape the event expects:

| Event | Output | Effect |
| --- | --- | --- |
| `PostToolUse` | `hookSpecificOutput.additionalContext` | The agent sees the comments after its current tool call, mid-turn. |
| `Stop` | `{"decision":"block","reason":…}` | If comments wait when the agent tries to finish, it keeps going and deals with them. |
| `UserPromptSubmit` | `hookSpecificOutput.additionalContext` | Anything left is attached to your next prompt. |

An empty or missing inbox prints nothing and exits 0, quickly; this runs after every tool call, so it must not parse transcripts or load anything else. Any error (bad JSON, unreadable inbox) is written to stderr and the hook exits 0, so a broken snoop never blocks an agent. A claimed file is deleted only after its contents have been printed. If the hook dies between claiming and printing, the `.jsonl.<pid>` file is left behind, and the next `snoop hook` for that session picks up any orphaned claims whose PID is no longer running.

**Install.** `snoop hooks install` adds the three entries to `~/.claude/settings.json`, running `snoop hook` by its absolute path. It writes atomically, keeps every other setting and hook untouched, leaves the hooks alone if they're already present, and prints what it changed. `snoop hooks uninstall` removes exactly those entries, and `snoop hooks status` says whether they're installed. `--project` targets `.claude/settings.json` in the current directory instead.

**In the UI.** Before sending, snoop checks whether the hooks are installed. If they aren't, `S` explains how to install them and offers `y` to copy the formatted comments to the clipboard instead (reusing `copy.go`), so the feature still works without hooks. After sending, the footer shows `sent 3 comments`. Once the hook has claimed the inbox, it changes to `delivered`. snoop learns this by checking whether the inbox file still exists on its next poll.

If the session isn't running, the comments wait in the inbox until it's resumed. The footer says `waiting for the session to resume`.

## Part 4: docs and site

- The README and `site/content/docs/` get a "Review and comment" section covering the Diff tab, the keys, `snoop hooks install`, and exactly what snoop writes.
- `?` help lists the new keys.
- `demo/` gets a screenshot of the Diff tab built from a fictional repo.

## Build order

Three steps, one commit each, tests first:
1. **`internal/review`**: running git and parsing diffs into files, hunks and lines with line numbers; the comment model and formatting; the inbox (append, claim, recover orphans). Unit-tested with a temporary git repo and fixed diffs.
2. **`snoop hook` and `snoop hooks install|uninstall|status`** in `cmd/snoop`. Tested against fixture hook inputs and a temporary settings file, including one with existing hooks.
3. **The Diff tab and comment UI**, then docs and the screenshot.

## Calls I made (the user skipped questions)

- `git diff HEAD` plus untracked files, rather than reconstructing the diff from the transcript, because line numbers must match the files on disk.
- Per-file attribution, not per-hunk.
- One-line comment input, not a multi-line editor.
- Comments are batched and sent with `S`, like a PR review, not sent one at a time.
- All three hook events, so comments arrive whether the agent is working, finishing, or idle.
- Hooks are installed in user settings by default, and only by an explicit command.
- The keys `c`, `C`, `v`, `x`, `e`, `S` and `r`. In the session view `x` and `e` already exist, but the Diff tab overrides them while it's active. The builder should confirm there are no other conflicts in `internal/ui/session.go`.

## Left to the builder

- Package layout inside `internal/review`, and how the diff view shares code with the Files tab.
- How to check the exact hook input and output fields against the current Claude Code hooks documentation before writing step 2.
- How long and wide diffs scroll and wrap.
