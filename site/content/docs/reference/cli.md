---
title: Command line
weight: 1
---

```text
snoop [--follow | --live] [--project dir] [--bell] [--notify] [session-id|path]
snoop export <session-id|path> [-o file]
snoop hooks install|uninstall|status [--project] [--ask-wait duration]
```

## Commands

| Command | What it does |
| --- | --- |
| `snoop` | open the session picker, across all projects |
| `snoop <session>` | open a session by id prefix, such as `64f3ec44`, or by a `.jsonl` path |
| `snoop --follow` | follow the newest session for this directory, and switch to new ones |
| `snoop --live` | open the live board: every session written in the last 30 minutes |
| `snoop export <session>` | write a session as Markdown to stdout, without thinking blocks |
| `snoop hooks install` | add the hooks that deliver the Diff tab's comments and let snoop answer permission requests; `--project` uses `./.claude/settings.json`, and `--ask-wait` (default `5m`, from `1s` to `30m`) sets how long a request waits for snoop |
| `snoop hooks uninstall` | remove exactly those hooks |
| `snoop hooks status` | report whether they're installed, exiting with 1 if not; an older install without the permission hook is reported as such |
| `snoop hook` | run by the hooks: deliver the comments waiting for the session, or wait for snoop's answer to a permission request; you don't run it yourself |

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--follow`, `-f` | off | follow the newest session for the project and switch to new ones |
| `--live` | off | show the live board. Can't be combined with `--follow` or a session |
| `--project DIR` | current directory | the project whose sessions `--follow` and `c` use |
| `--stall DURATION` | `30s` | raise the waiting alert when a tool call has no result after this long. `0` disables it |
| `--bell` | off | ring the terminal bell on the alerts chosen with `--alerts` |
| `--notify[=MODE]` | off | send desktop notifications on those alerts. `MODE` is `auto` (the default), `terminal` or `system` |
| `--alerts LIST` | `error,turn,ask` | which alerts ring the bell or notify: any of `error`, `stall`, `turn`, `compact`, `ctx`, `ask` |
| `--version` | | print the version and exit |

The export command takes `-o FILE` to write to a file instead of stdout.

## Environment

| Variable | Effect |
| --- | --- |
| `CLAUDE_CONFIG_DIR` | where Claude Code keeps its data; snoop reads `$CLAUDE_CONFIG_DIR/projects`, or `~/.claude/projects` when it's unset |
| `XDG_CACHE_HOME` | on Linux, moves snoop's cache. See [What snoop reads and writes](../data/) |
