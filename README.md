# snoop

A fast terminal viewer for Claude Code session transcripts
(`~/.claude/projects/**/*.jsonl`). Keep it open in a second terminal next to
`claude` to watch a session live, or browse past sessions.

```
go install ./cmd/snoop

snoop                  # pick a session (all projects)
snoop --follow         # follow the newest session for this directory, live;
                       # switches automatically when a new session starts
snoop 64f3ec44         # open by session id prefix (or a .jsonl path)
snoop --project DIR    # treat DIR as the current project
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
snoop --follow --bell --alerts error,stall,turn,compact,ctx
snoop --stall 2m                         # 0 disables the waiting alert
```

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
| `?` | every key for the current screen |
| `q` | quit |

In the Tools tab: `space` expands or collapses a group, `enter` on a call
shows it in the timeline, `←`/`→` collapse and expand (`←` on a call moves to
its group), `E` expands everything (again to reset), and `/` search keeps
only matching calls with their groups opened.

In the picker: `/` fuzzy search, `esc` clears it, `c` toggles this project only.

## Notes

- The transcript format is undocumented and changes between Claude Code
  versions. Unknown records are ignored; bulky internal records
  (`api-request*`, `attachment`, `file-history-*`) are skipped without being
  decoded, so a 75 MB transcript loads in ~150 ms.
- Respects `CLAUDE_CONFIG_DIR`.
- `go run ./cmd/probe` parses every local transcript and reports timings.
