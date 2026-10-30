<h1>
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="site/static/mark-dark.svg">
    <img src="site/static/mark-light.svg" alt="" height="28">
  </picture>
  snoop
</h1>

This is a fast terminal viewer for Claude Code session transcripts
(`~/.claude/projects/**/*.jsonl`). You can keep it open in a second terminal
next to `claude` to watch a session live, or browse past sessions to see what
each prompt cost, how full the context is, which files changed, what the
subagents did and where the time went. The documentation is at
https://jzills.github.io/snoop/.

![snoop: the summary across sessions, a session's timeline with a turn's
waterfall, an expanded subagent, the Files tab and Stats](demo/demo.gif)

## Install

```
brew install jzills/tap/snoop                      # macOS and Linux
curl -fsSL https://jzills.github.io/snoop/install.sh | sh
go install github.com/jzills/snoop/cmd/snoop@latest
```

The install script picks the build for your machine, checks it against the
release's `SHA256SUMS`, and installs it to `~/.local/bin` without sudo. To
install somewhere else, set `SNOOP_INSTALL_DIR`, and to install a given
release instead of the newest, set `SNOOP_VERSION`, such as `v0.1.0`.

You can also download a binary for Linux, macOS or Windows from the
[latest release](https://github.com/jzills/snoop/releases/latest). Each
archive holds the `snoop` executable, the license and this README.

Testing covers Claude Code 2.1.280 through 2.1.310. The transcript format is
undocumented and changes between versions. Records that snoop doesn't
recognize are ignored, so a new version may show less rather than break.
Please open an issue if something goes missing.

## Use

```
snoop                  # pick a session (all projects)
snoop --follow         # follow the newest session for this directory, live;
                       # switches automatically when a new session starts
snoop --live           # every session at work right now, across projects
snoop 64f3ec44         # open by session id prefix (or a .jsonl path)
snoop --project DIR    # treat DIR as the current project
snoop hooks install    # let the Diff tab send comments to the agent
snoop --version
```

## What you see

**Timeline:** The timeline shows every prompt, reply, thinking block and tool
call, one per line, with a blank line before each prompt so every turn
stands apart. A row in the same minute as the one above shows only its
seconds (`:42`). Tool rows show their duration and status (`✓` done, `✗`
error, `◌` running). Built-in tools (`▸`), skills (`✦`), MCP calls (`⇄`,
shown as `server·tool`) and subagents (`⤷`) each have their own color. The
right pane shows the selected item in full: Bash commands, Edit diffs, Write
content, agent prompts and results.

Each prompt row shows that turn's tokens, estimated cost (`≈`) and duration,
and compactions appear as `⟳ compacted (auto) 253k → 68k`. The header shows
how full the context window is (`ctx 142k/200k ▰▰▰▰▰▰▰▱▱▱ 71%`), turning to
the accent color at 70% and red at 90%. Selecting a prompt shows where its
turn's time went: totals, the five slowest tool calls, and a waterfall that
places each call by its start and end, so parallel calls overlap.

Pressing `x` on an Agent call expands its subagent inline, indented under a
`│` rule and kept live while it runs. Pressing `X` expands every subagent,
nested ones too. Search and filters reach into subagents, and a collapsed call
whose subagent has matches is kept and marked `●`.

**Tools:** The Tools tab groups every tool call by kind (built-in, skills,
MCP, subagents), then by name (tool, skill, MCP `server·tool`, subagent type),
then lists the individual calls, with counts, errors and average latency.
Selecting a group summarizes it in the right pane, and pressing `enter` on a
call jumps to it in the timeline.

**Files:** The Files tab lists every file the session and its subagents read,
edited or wrote, with read, edit and write counts and the lines added and
removed. Changed files come first, then files only read, each most recent
first. The right pane shows a file's history with every change as a diff,
which `n` and `N` step through, and pressing `enter` shows the latest change
in the timeline. Pressing `T` switches to a tree of folders, each with the
totals of its files. Changes made through Bash aren't tracked.

**Stats:** The Stats tab shows the duration, prompts, tool mix, token usage
(input, output, cache and thinking), context (current, peak and compactions),
the cost when Claude Code recorded it, and the skills, MCP servers, subagent
types and models used. Its Turns breakdown lists each prompt's duration,
tokens, estimated cost and context at the end of the turn.

Cost per prompt is an estimate. Claude Code records only each model's total
cost for the session, so that total is split across prompts by token use,
weighted by relative token price. The prompt costs add up to the recorded
total. Sessions without a recorded cost show tokens only.

## Many sessions at once

**Live board:** The live board, opened with `snoop --live` or by pressing `w`
in the picker, lists every session written in the last 30 minutes, across
projects, most urgent first.

| State | Meaning |
| --- | --- |
| `? asking` | the session asks for permission to use a tool, which you can answer here |
| `◌ waiting` | a tool call has had no result for `--stall` (30s), usually a permission prompt |
| `✓ your turn` | the turn ended, or you interrupted it |
| `● working` | replying, or running a tool or subagent (shown underneath) |
| `· idle` | no prompts yet, or nothing written for 10 minutes |

The board's header counts the sessions waiting on you (`1 waiting on you`).
Each row shows the project, the session, how full its context is, its cost
and when it last wrote. Press `enter` to open a session and `esc` to come
back, or `o` and `O` to sort by another column. Only sessions on the board are read, and after the first read, only
what they add.

**Answer permission prompts:** With snoop's hooks installed (see
[Review and comment](#review-and-comment)), a session that asks for
permission shows as `? asking` on the board, with the request in full under
the list: the command, the edit's diff, or the tool's input. Press `A` to
allow it once, or `D` to deny it, optionally with a reason that Claude sees.
The same keys work in the session's own view, where its footer shows the
request. Claude Code's own prompt still appears in the session's terminal,
and whichever answer comes first wins.

The hook waits for an answer only while a live board or session view is
open, for at most 5 minutes (set with `snoop hooks install --ask-wait 10m`),
and answering in the terminal ends the wait at once. There is no "always
allow" here, so rules stay in Claude Code's own prompt.

**Find:** Pressing `f` in the picker finds sessions by a file they read or
changed, a command they ran, or a prompt, subagents included. Every word must
match the same file, command or prompt. Each result shows what matched, and
opening it searches its timeline for the same words. Sessions are indexed by
the same background scan as the summary and cached with it.

## Review and comment

The Diff tab (`5`) shows the session's changes in one of two scopes, and `b`
switches between them. **Uncommitted** is `git diff HEAD`, with untracked
files shown as added. **Branch** is everything the branch changed since it
left its base, committed or not, which is what a pull request would show plus
the work in progress, so you can review an agent's whole branch without
opening a PR. Files with changes since the last commit are tagged
`uncommitted`. The tab opens in Branch when the branch has commits of its own.

Unless you choose one, snoop compares with the closest of `develop`, `main`,
`master` and `trunk`, preferring `origin/` copies, since a local one may be
out of date. The line at the top of the tab names the base. Press `B` to pick
another base, which is saved for the branch in its git config
(`branch.<name>.snoopBase`), or set `git config snoop.base` for a whole
repository.

Files the session changed come first, and files changed by you or another
agent are dimmed. The tab refreshes as the session edits files or runs a
command, and `r` reads the diff again.

Press `tab` to move into a file's lines, then `c` to comment on a line. Press
`v` and move to select a range first, or press `C` to comment on the whole
file. Press `e` or `x` to edit or delete the comment under the cursor.
Comments wait until you press `S`, which sends them all at once. Each comment
names the file and lines and quotes the code, so the agent doesn't have to
reread the file.

Sending needs Claude Code hooks, which you add once:

```
snoop hooks install              # in ~/.claude/settings.json
snoop hooks install --project    # in ./.claude/settings.json instead
snoop hooks status               # or uninstall
```

The hooks run `snoop hook` after each tool call, when the agent tries to
finish, and when you submit a prompt. The agent sees your comments at the
first of these, so a working agent gets them mid-turn and an agent that has
finished keeps going. The footer shows when they are delivered. Without the
hooks, `S` copies the comments to the clipboard instead. A fourth hook runs
when a session asks for permission, so the live board can
[answer it](#many-sessions-at-once). If you installed the hooks before that
hook existed, `snoop hooks status` says so, and running `snoop hooks install`
again adds it.

The hooks are the only change snoop makes to Claude Code's settings, and
`snoop hooks uninstall` removes exactly those entries. Sent comments wait in a
`snoop/inbox` folder in your state directory (`~/.local/state` on Linux) until
a hook delivers them. Permission requests wait in `snoop/asks` beside it, and
open snoops refresh a small `snoop/listening` file every 2 seconds, which is
how the hook knows to wait. A session that was already running may need a restart
before it uses the hooks.

## Summary across sessions

Pressing `s` in the session picker switches to a summary of the sessions
listed, so it follows `c` and the search. It shows sessions, prompts, tool
calls and the error rate, cost and tokens, a daily cost chart for the last 30
days, and the top tools, skills, MCP servers, subagent types and models with
their error rates. Every transcript is parsed in the background
(`scanning 12/48` in the header), and the results are cached in
`~/.cache/snoop/summaries.json` (the user cache directory), so later opens are
instant. Unreadable transcripts are counted and skipped.

## Copy and export

Pressing `y` copies the selection to the clipboard over OSC 52, which works
over SSH, and in tmux with `set-clipboard on`. It copies the command for
Bash, the diff for edits, the content for Write, the full text of messages,
and the result for other tools. In the Files tab, it copies all of the file's
changes.

Pressing `e` exports the timeline as shown (search, filter, thinking and
expanded subagents included) to `./<session-id-8>-<YYYYMMDD-HHMM>.md`.
Prompts become headings, tool calls become collapsible `<details>` blocks,
and subagent events are quoted under their call. You can also export from the
shell:

```
snoop export 64f3ec44              # whole session to stdout, without thinking
snoop export 64f3ec44 -o notes.md
```

## Alerts

While a session is receiving new lines, the header flags what needs your
attention:

| Alert | Raised when |
| --- | --- |
| `✗ error` | a tool call fails; the header counts unseen errors |
| `◌ waiting 30s on Bash` | a tool call has no result after `--stall` (default 30s), which may be a permission prompt, since those aren't recorded until answered |
| `✓ your turn` | Claude finishes |
| `⟳ compacted` | the context is compacted |
| `ctx 90%` | the context window passes 90% |
| `? Bash: npm test` | the session asks for permission (needs the hooks) |

Any key hides the alert, and pressing `!` jumps to it. Lines already in the
transcript when it opens never raise alerts.

```
snoop --follow --bell                    # ring the bell on errors, turn ends and permission requests
snoop --live --notify                    # a desktop notification when any session needs you
snoop --follow --bell --alerts error,stall,turn,compact,ctx,ask
snoop --stall 2m                         # 0 disables the waiting alert
```

The live board raises the same alerts for every session on it, prefixed with
the project (`api: ✓ your turn`).

With `--notify`, snoop sends a desktop notification naming the session and
what happened (`api · Fix the flaky test` / `Your turn · finished after
2m10s`), so you can work elsewhere and come back when a session needs you. It
picks one way to deliver them:

| Where snoop runs | Notification |
| --- | --- |
| iTerm2, WezTerm, Ghostty, kitty | the terminal's own (clicking it focuses the tab) |
| over SSH | the terminal's own, so it shows on your machine, not the server |
| WSL | a Windows toast, through `powershell.exe` |
| macOS, other terminals | `osascript` |
| Linux desktop | `notify-send` |
| anywhere else | the terminal's own escape sequence (OSC 777) |

To override the choice, use `--notify=terminal` or `--notify=system`. In
tmux, let the terminal's own notifications through with
`set -g allow-passthrough on`.

Nothing is sent while snoop's window has focus (in terminals that report
focus). At most one notification goes out per session and alert a minute, and
several snoops watching the same session send one between them.

## Keys

Press `?` anywhere for the keys of the current screen. The full list follows.

| Key | Action |
| --- | --- |
| `↑↓` / `j k`, wheel | move (the wheel over the right pane scrolls it) |
| `enter` / `tab` | focus the detail pane (zoom it on narrow terminals) |
| `J K` / `ctrl+d ctrl+u` | scroll the detail pane |
| `/` | search text, commands, paths and results (all words must match) |
| `f` / `F` | cycle the filter: all, chat, tools, skills, mcp, agents, errors |
| `t` | show thinking blocks |
| `n` / `N` | next / previous prompt |
| `x` / `X` | expand a subagent inline / expand or collapse all |
| `a` | open the subagent transcript for an Agent call (`esc` returns) |
| `g G` | top / bottom (bottom re-enables following) |
| `1 2 3 4 5`, `[ ]` | switch tabs |
| `esc` | clear the search or filter, then go back |
| `!` | jump to the latest alert |
| `A` / `D` | allow once / deny the session's permission request |
| `y` / `e` | copy the selection / export the view to Markdown |
| `?` | every key for the current screen |
| `q` | quit |

In the Tools tab, `space` expands or collapses a group, `enter` on a call
shows it in the timeline, and `←` and `→` collapse and expand (`←` on a call
moves to its group). Pressing `E` expands everything (press it again to
reset), and a `/` search keeps only matching calls with their groups opened.

In the Files tab, `T` switches between the list and a tree of folders. In
the tree, `space` and `enter` open or close a folder, `←` and `→` close and
open (`←` on a file moves to its folder), and `E` opens every folder (press
it again to reset).

In the Diff tab, `b` switches between the uncommitted changes and the whole
branch, and `B` picks the branch's base. Press `tab` to move between the
files and their lines, and `J` and `K` to jump between hunks. The keys `c`,
`v`, `C`, `e`, `x` and `S` work as described in
[Review and comment](#review-and-comment).

In the picker, `/` starts a fuzzy search and `f` finds across sessions, and
`esc` clears either. Pressing `c` toggles this project only, `s` switches to
the summary, and `w` opens the live board. On the picker and the live board,
`o` sorts by the next column and `O` reverses the order. Search results stay
best match first until you choose a column.

## Notes

- The transcript format is undocumented and changes between Claude Code
  versions. Unknown records are ignored, and bulky internal records
  (`api-request*`, `attachment`, `file-history-*`) are skipped without being
  decoded, so a 75 MB transcript loads in about 150 ms.
- It respects `CLAUDE_CONFIG_DIR`.
- Running `go run ./cmd/probe` parses every local transcript and reports
  timings.
- The demo GIF is rendered from `demo/demo.tape` against fictional sessions
  by `demo/render.sh`, which needs [vhs](https://github.com/charmbracelet/vhs).

This independent project isn't affiliated with or endorsed by Anthropic.
Claude and Claude Code are trademarks of Anthropic.

## License

[MIT](LICENSE)
