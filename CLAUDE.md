# snoop

A terminal viewer for Claude Code session transcripts, written in Go with
Bubble Tea. The README describes what it does, and `site/content/docs/` is
the full documentation.

## Layout

| Path | What |
| --- | --- |
| `cmd/snoop` | the CLI: flags, `snoop export`, notifier selection |
| `cmd/probe` | parses every local transcript and reports timings |
| `internal/parse` | JSONL records to events, turns, usage and cost; lenient by design |
| `internal/store` | finding transcripts, loading them, following them as they grow |
| `internal/tail` | watching a transcript and noticing new sessions |
| `internal/ui` | the Bubble Tea app: picker, session views, live board, alerts, help |
| `internal/notify` | desktop notifications, one delivery method per environment |
| `internal/summary` | the cached per-session summaries behind the summary and find |
| `internal/stats`, `internal/export` | the Stats tab, and Markdown export |
| `site/` | the Hugo site and docs; it serves `install.sh` |
| `demo/` | the README GIF and the site's screenshots, from fictional sessions |
| `docs/superpowers/` | design specs and plans for past feature sets |

## Conventions

- **Checks:** before calling anything done, run the checks in the `check`
  skill. A hook runs `gofmt -w` on every Go file you edit.
- **Writing:** user-facing text follows the `writing-docs` skill: complete
  American English sentences, none starting with a command, key or "snoop".
  A hook checks the README and the site on every edit.
- **Site:** see the `site` skill for the design conventions, previewing and
  re-rendering screenshots.
- **Speed:** opening and following a session must not get slower. Parse only
  what's needed, and do slow work in a `tea.Cmd`, never inside `Update`.
- **Transcripts:** the format is undocumented. Ignore unknown records rather
  than failing, and never read real transcripts in tests or recordings.
- **Git:** commit or push only when asked, with no attribution lines, and
  publish nothing without being asked.
