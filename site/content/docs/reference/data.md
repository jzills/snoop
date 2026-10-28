---
title: What snoop reads and writes
weight: 4
---

Everything snoop does happens on your machine, and it makes no network
requests.

## Reads

| Path | What |
| --- | --- |
| `~/.claude/projects/<project>/<session>.jsonl` | session transcripts |
| `~/.claude/projects/<project>/<session>/subagents/` | subagent transcripts and their metadata |

When `CLAUDE_CONFIG_DIR` is set, snoop reads `$CLAUDE_CONFIG_DIR/projects`
instead. It never writes to these files.

## Writes

| Path | What |
| --- | --- |
| `<cache>/snoop/summaries.json` | the summary and find index, one entry per transcript, rebuilt when a transcript changes |
| `<cache>/snoop/notified/` | one small file per notification sent, so several snoops send it only once; each is removed after an hour |
| `./<id>-<date>-<time>.md` | timelines you export with `e` |
| `<state>/snoop/inbox/<session>/` | comments sent from the Diff tab, one file per send, removed once a hook delivers them |
| `~/.claude/settings.json` | snoop's three hook entries, only when you run `snoop hooks install`; `snoop hooks uninstall` removes them |

Here `<cache>` is your user cache directory:

| Platform | Cache directory |
| --- | --- |
| Linux | `$XDG_CACHE_HOME`, or `~/.cache` |
| macOS | `~/Library/Caches` |
| Windows | `%LocalAppData%` |

Here `<state>` is `$XDG_STATE_HOME` or `~/.local/state` on Linux,
`~/Library/Application Support` on macOS and `%AppData%` on Windows.

The Diff tab also runs `git` in the session's working directory, read-only and
without taking the index lock.

Deleting the cache is safe; snoop rebuilds it on the next scan.
