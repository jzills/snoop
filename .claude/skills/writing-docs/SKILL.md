---
name: writing-docs
description: Use when writing or editing any user-facing text in snoop — README.md, the site's docs and landing page (site/content/, site/layouts/home.html), in-app help (internal/ui/help.go), flag help in cmd/snoop/main.go, or release notes. Covers the project's writing rules and keeping the copies of the same facts in sync.
---

# Writing snoop's docs

snoop's text is plain, factual and short, in the style of the Ghostty and
Claude Code docs: say what something does and what to do next, without
marketing language.

## Rules

Every sentence is a complete American English sentence.

1. **Never start a sentence with a code span.** That covers commands, flags,
   keys, paths and variables. Lead with a word instead.
   - "`y` copies the selection." → "Press `y` to copy the selection."
   - "`--follow` opens the newest session." → "With `--follow`, snoop opens the newest session."
   - "`<cache>` is your cache directory." → "Here `<cache>` is your cache directory."
2. **Never start a sentence with "snoop".** The name is lowercase, so a
   sentence can't open with it.
   - "snoop reads transcripts." → "It reads transcripts." or "By default, snoop reads transcripts."
3. **Write no fragments.** Captions, card subtitles, list items and lead-ins
   are sentences too.
   - Caption "A turn's waterfall." → "In a turn's waterfall, calls that ran in parallel overlap."
   - Subtitle "Every command and flag." → "Look up every command and flag."
   - "On macOS and Linux:" → "Run this on macOS or Linux:"
   - "One binary, nothing to configure." → "It's a single binary with nothing to configure."
4. **Use American spelling:** color, recognize, labeled, canceled, gray.
5. **Table cells and headings may be phrases.** Keep each cell one phrase or
   one clause, with no trailing period. Don't mix a phrase and a sentence in
   one cell; join them with a semicolon instead.

## Voice

- Address the reader as "you", and use the imperative for steps.
- State facts, not claims: "A 75 MB transcript opens in about 150 ms," not
  "blazing fast".
- Don't promise what the code doesn't do. Check each claim against the
  code before writing it, especially flags, defaults, paths and platform
  support.
- Prefer short paragraphs. A guide covers one task, and the reference covers
  every option.

## One fact, several places

The same facts live in several files. When one changes, update all of them
in the same commit:

| Fact | Where it lives |
| --- | --- |
| flags and defaults | `cmd/snoop/main.go`, README "Use", `site/content/docs/reference/cli.md` |
| keys | `internal/ui/help.go`, README "Keys", `site/content/docs/reference/keys.md`, the guides |
| alerts and notifications | `internal/ui/help.go`, README "Alerts", `site/content/docs/reference/alerts.md`, the live-board guide |
| install methods | README "Install", `site/content/docs/install.md`, `site/layouts/home.html`, `site/static/install.sh` |
| files snoop writes | README, `site/content/docs/reference/data.md` |

## Check

A PostToolUse hook runs `.claude/hooks/prose_lint.py` on every edit to the
README, `site/content/` and `site/layouts/home.html`, and reports sentences
that break rules 1, 2 and 4. Run it on everything before committing:

```sh
python3 .claude/hooks/prose_lint.py README.md site/layouts/home.html $(find site/content -name '*.md')
```

The linter can't judge whether a sentence is complete (rule 3), so reread
captions, subtitles, list items and lead-ins yourself.
