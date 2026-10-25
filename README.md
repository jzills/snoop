# snoop

A fast terminal viewer for Claude Code session transcripts
(`~/.claude/projects/**/*.jsonl`). Keep it open in a second terminal next to
`claude` to watch a session live, or browse past sessions: what each prompt
cost, how full the context is, which files changed, what the subagents did,
and where the time went. Site: https://jzills.github.io/snoop/

![snoop: the summary across sessions, a session's timeline with a turn's
waterfall, an expanded subagent, the Files tab and Stats](demo/demo.gif)

## Install

```
brew install jzills/tap/snoop                      # macOS and Linux
curl -fsSL https://jzills.github.io/snoop/install.sh | sh
go install github.com/jzills/snoop/cmd/snoop@latest
```

The install script picks the build for your machine, checks it against the
release's `SHA256SUMS`, and installs it to `~/.local/bin` without sudo.
`SNOOP_INSTALL_DIR` changes where it goes, and `SNOOP_VERSION=v0.1.0`
installs that release instead of the newest.

Or download a binary for Linux, macOS or Windows from the
[latest release](https://github.com/jzills/snoop/releases/latest); each
archive holds the `snoop` executable, the license and this README.

Tested with Claude Code 2.1.280 through 2.1.310. The transcript format is
undocumented and changes between versions; snoop ignores what it doesn't
recognise, so a new version may show less rather than break. Please open an
issue if something goes missing.

## Use

```
snoop                  # pick a session (all projects)
snoop --follow         # follow the newest session for this directory, live;
                       # switches automatically when a new session starts
snoop --live           # every session at work right now, across projects
snoop 64f3ec44         # open by session id prefix (or a .jsonl path)
snoop --project DIR    # treat DIR as the current project
snoop --version
```

## What you see

**Timeline**: every prompt, reply, thinking block and tool call, one per line.
Tool rows show duration and status (`✓` done, `✗` error, `◌` running).
Built-in tools `▸`, skills `✦`, MCP calls `⇄` (shown as `server·tool`) and
subagents `⤷` each have their own colour. The right pane shows the selected
item in full: Bash commands, Edit diffs, Write content, agent prompts and
results. Each prompt row shows that turn's tokens, estimated cost (`≈`) and
duration, and compactions appear as `⟳ compacted (auto) 253k → 68k`. The
header shows how full the context window is (`ctx 142k/200k ▰▰▰▰▰▰▰▱▱▱ 71%`),
turning to the accent colour at 70% and red at 90%. Selecting a prompt shows
where its turn's time went: totals, the five slowest tool calls, and a
waterfall placing each call by its start and end, so parallel calls overlap.
`x` on an Agent call expands its subagent inline, indented under a `│` rule
and kept live while it runs; `X` expands every subagent, nested ones too.
Search and filters reach into subagents: a collapsed call whose subagent has
matches is kept and marked `●`.

**Tools**: every tool call grouped by kind (built-in, skills, MCP,
subagents), then by name (tool, skill, MCP `server·tool`, subagent type),
then the individual calls, with counts, errors and average latency. Selecting
a group summarises it in the right pane; `enter` on a call jumps to it in the
timeline.

**Files**: every file the session and its subagents read, edited or wrote,
with read/edit/write counts and lines added and removed. Changed files come
first, then files only read, each most recent first. The right pane shows a
file's history with every change as a diff (`n`/`N` step through them), and
`enter` shows the latest change in the timeline. Changes made through Bash
aren't tracked.

**Stats**: duration, prompts, tool mix, token usage (input, output, cache,
thinking), context (current, peak, compactions), cost when Claude Code
recorded it, skills, MCP servers, subagent types and models, and a Turns
breakdown: each prompt's duration, tokens, estimated cost and context at the
end of the turn.

Cost per prompt is an estimate. Claude Code records only each model's total
cost for the session, so that total is split across prompts by token use
(weighted by relative token price). The prompt costs add up to the recorded
total. Sessions without a recorded cost show tokens only.

## Many sessions at once

**Live board**: `snoop --live`, or `w` in the picker, lists every session
written in the last 30 minutes, across projects, most urgent first:

- `◌ waiting`: a tool call has had no result for `--stall` (30s), usually a
  permission prompt. The board's header counts these (`1 waiting on you`).
- `✓ your turn`: the turn ended, or you interrupted it.
- `● working`: replying, or running a tool or subagent (shown underneath).
- `· idle`: no prompts yet, or nothing written for 10 minutes.

Each row shows the project, the session, how full its context is, its cost
and when it last wrote. `enter` opens a session and `esc` comes back. Only
sessions on the board are read, and after the first read only what they add.

**Find**: `f` in the picker finds sessions by a file they read or changed, a
command they ran, or a prompt, subagents included. Every word must match the
same file, command or prompt. Each result shows what matched, and opening it
searches its timeline for the same words. Sessions are indexed by the same
background scan as the summary, and cached with it.

## Summary across sessions

`s` in the session picker switches to a summary of the sessions listed (so
it follows `c` and the search): sessions, prompts, tool calls and error rate,
cost and tokens, a daily cost chart for the last 30 days, and the top tools,
skills, MCP servers, subagent types and models with their error rates. Every
transcript is parsed in the background (`scanning 12/48` in the header) and
the results cached in `~/.cache/snoop/summaries.json` (the user cache
directory), so later opens are instant. Unreadable transcripts are counted
and skipped.

## Copy and export

`y` copies the selection to the clipboard over OSC 52 (works over SSH, and in
tmux with `set-clipboard on`): the command for Bash, the diff for edits, the
content for Write, the full text of messages, and the result for other tools.
In the Files tab it copies all of the file's changes.

`e` exports the timeline as shown (search, filter, thinking and expanded
subagents included) to `./<session-id-8>-<YYYYMMDD-HHMM>.md`. Prompts become
headings, tool calls collapsible `<details>` blocks, and subagent events are
quoted under their call. From the shell:

```
snoop export 64f3ec44              # whole session to stdout, without thinking
snoop export 64f3ec44 -o notes.md
```

## Alerts

While a session is receiving new lines, the header flags what needs your
attention: `✗ error` (with a count of unseen errors), `◌ waiting 30s on Bash`
for a tool call with no result after `--stall` (default 30s; this may be a
permission prompt, since those aren't recorded until answered), `✓ your turn`
when Claude finishes, `⟳ compacted`, and `ctx 90%`. Any key hides the alert;
`!` jumps to it. Lines already in the transcript when it opens never raise
alerts.

```
snoop --follow --bell                    # ring the bell on errors and turn ends
snoop --live --notify                    # a desktop notification when any session needs you
snoop --follow --bell --alerts error,stall,turn,compact,ctx
snoop --stall 2m                         # 0 disables the waiting alert
```

The live board raises the same alerts for every session on it, prefixed with
the project (`api: ✓ your turn`).

`--notify` sends desktop notifications through the terminal, so they work
over SSH: OSC 9 in iTerm2, WezTerm and Ghostty, OSC 99 in kitty, OSC 777
elsewhere (foot, rxvt, VTE terminals with notification support). In tmux,
allow them through with `set -g allow-passthrough on`. Terminals that don't
understand them ignore them.

## Keys

Press `?` anywhere for the keys of the current screen. The full list:

| Key | Action |
| --- | --- |
| `↑↓` / `j k`, wheel | move (wheel over the right pane scrolls it) |
| `enter` / `tab` | focus the detail pane (zoom it on narrow terminals) |
| `J K` / `ctrl+d ctrl+u` | scroll the detail pane |
| `/` | search text, commands, paths and results (all words must match) |
| `f` / `F` | cycle filter: all, chat, tools, skills, mcp, agents, errors |
| `t` | show thinking blocks |
| `n` / `N` | next / previous prompt |
| `x` / `X` | expand a subagent inline / expand or collapse all |
| `a` | open the subagent transcript for an Agent call (`esc` returns) |
| `g G` | top / bottom (bottom re-enables following) |
| `1 2 3 4`, `[ ]` | switch tabs |
| `esc` | clear search/filter, then go back |
| `!` | jump to the latest alert |
| `y` / `e` | copy the selection / export the view to Markdown |
| `?` | every key for the current screen |
| `q` | quit |

In the Tools tab: `space` expands or collapses a group, `enter` on a call
shows it in the timeline, `←`/`→` collapse and expand (`←` on a call moves to
its group), `E` expands everything (again to reset), and `/` search keeps
only matching calls with their groups opened.

In the picker: `/` fuzzy search, `f` find across sessions, `esc` clears either,
`c` toggles this project only, `s` switches to the summary, `w` opens the live
board.

## Notes

- The transcript format is undocumented and changes between Claude Code
  versions. Unknown records are ignored; bulky internal records
  (`api-request*`, `attachment`, `file-history-*`) are skipped without being
  decoded, so a 75 MB transcript loads in ~150 ms.
- Respects `CLAUDE_CONFIG_DIR`.
- `go run ./cmd/probe` parses every local transcript and reports timings.
- The demo GIF is rendered from `demo/demo.tape` against fictional sessions:
  `demo/render.sh` (needs [vhs](https://github.com/charmbracelet/vhs)).

snoop is an independent project, not affiliated with or endorsed by
Anthropic. Claude and Claude Code are trademarks of Anthropic.

## License

[MIT](LICENSE)
