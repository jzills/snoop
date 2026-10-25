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

Here `<cache>` is your user cache directory:

| Platform | Cache directory |
| --- | --- |
| Linux | `$XDG_CACHE_HOME`, or `~/.cache` |
| macOS | `~/Library/Caches` |
| Windows | `%LocalAppData%` |

Deleting the cache is safe; snoop rebuilds it on the next scan.
