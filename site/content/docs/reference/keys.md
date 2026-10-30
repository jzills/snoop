---
title: Keyboard shortcuts
weight: 2
---

Press `?` on any screen for that screen's keys. This page lists them all.

## Everywhere

| Key | Action |
| --- | --- |
| `?` | show or hide the keys for the current screen |
| `esc` | clear the search and filter, then go back |
| `q`, `ctrl+c` | quit |

## Session picker

| Key | Action |
| --- | --- |
| `↑` `↓`, `j` `k` | move |
| `pgup` `pgdn` | page (also `ctrl+u` `ctrl+d`) |
| `g` `G` | first and last session |
| `enter`, `→` | open the session |
| `/` | fuzzy search titles, projects and branches |
| `f` | find sessions by a file, command or prompt |
| `c` | only sessions for this directory |
| `o` | sort by the next column: age, project, branch, session, size |
| `O` | reverse the order |
| `s` | switch between the list and the summary |
| `w` | open the live board |

## Live board

| Key | Action |
| --- | --- |
| `↑` `↓`, `j` `k` | move |
| `g` `G` | first and last session |
| `enter`, `→` | open the session; `esc` comes back |
| `A` | allow the selected session's permission request once |
| `D` | deny it, with an optional reason Claude sees |
| `o` | sort by the next column: state, project, session, context, cost, age |
| `O` | reverse the order |
| `esc`, `w` | back to the session picker |

## Session

These work on every tab.

| Key | Action |
| --- | --- |
| `1` `2` `3` `4` `5` | Timeline, Tools, Stats, Files, Diff |
| `[` `]` | previous and next tab |
| `!` | jump to the latest alert |
| `A` `D` | allow once or deny the session's permission request |
| `y` | copy the selection |
| `e` | export the timeline as shown to Markdown |

## Timeline

| Key | Action |
| --- | --- |
| `↑` `↓`, `j` `k` | move |
| `pgup` `pgdn` | page (also `ctrl+b` `ctrl+f`, `space`) |
| `g` | first event |
| `G` | last event, and resume following |
| `n` `N` | next and previous prompt |
| `/` | search text, commands, paths and results |
| `f` `F` | cycle the filter: all, chat, tools, skills, MCP, agents, errors |
| `t` | show thinking blocks |
| `enter`, `tab`, `→` | focus the detail pane |
| `J` `K` | scroll the detail pane (also `ctrl+d` `ctrl+u`) |
| `x` | expand or collapse a subagent inline |
| `X` | expand or collapse every subagent |
| `a`, `o` | open a subagent's transcript |

## Tools

| Key | Action |
| --- | --- |
| `space` | expand or collapse a group |
| `enter` | expand a group; on a call, show it in the timeline |
| `→` `l` | expand a group, or focus the detail pane |
| `←` `h` | collapse a group, or move to a call's group |
| `E` | expand everything; again to reset |
| `/` | keep only matching calls |

## Files

| Key | Action |
| --- | --- |
| `enter` | show the file's latest change in the timeline |
| `n` `N` | next and previous change |
| `/` | keep only files whose path matches |
| `T` | switch between the list and a tree of folders |
| `space`, `←` `→` | in the tree, close or open a folder (`←` on a file moves to its folder) |
| `E` | in the tree, open every folder, or go back |

## Diff

| Key | Action |
| --- | --- |
| `b` | switch between the uncommitted changes and the whole branch |
| `B` | choose the base the branch is compared with |
| `enter`, `tab`, `→` | move into the file's lines |
| `tab`, `←`, `esc` | back to the files |
| `J` `K`, `n` `N` | next and previous hunk |
| `c` | comment on the line, or on the selected range |
| `v` | start or clear a range selection |
| `C` | comment on the whole file |
| `e` `x` | edit and delete the comment on the line |
| `S` | send every comment to the agent |
| `r` | read the diff again |
| `y` | copy the file's diff |
