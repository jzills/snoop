---
name: writing-docs
description: Use when writing or editing any user-facing text in snoop — README.md, the site's docs and landing page (site/content/, site/layouts/home.html), in-app help (internal/ui/help.go), flag help in cmd/snoop/main.go, or release notes. Covers the project's writing rules, one sentence per line, the README as a pitch with the site as the reference, the release-notes shape, and keeping the copies of the same facts in sync.
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
- Name snoop in the third person ("it reads…", "By default, snoop…"), and
  never write "we".
- The product name is plain text. Code formatting is for what you type:
  `snoop --live`, but "snoop shows…".

## Layout

These come from kx's style guide, so the two projects read alike.

- **One sentence per line, never hard-wrapped.** A list item that runs to a
  second sentence continues on the next line, indented two spaces. Diffs then
  show the sentence that changed, not the paragraph it reflowed.
- **Code blocks are 80 columns or narrower,** because GitHub never wraps them.
- **Images** go in `<p align="center"><img … width="800"/></p>`, with alt text
  that says what the picture shows.

## The README is a pitch; the site is the reference

The README is also the package page on npm and PyPI. It says what snoop does
and why you'd want it, and the site holds every key, flag, table and edge case.

- It opens with the mark, badges and a row of links to the docs, then two
  sentences on what snoop is, then the demo GIF.
- Each section is a few sentences, at most one short command block and one
  picture, on one thing you can do. No tables of keys, flags or alerts.
- Each section ends with "See the [<topic> guide](url) for more." It is
  always "guide", except "See the [alerts reference](url)" for a reference
  page.
- Every link is absolute (`https://jzills.github.io/snoop/docs/…`,
  `https://raw.githubusercontent.com/jzills/snoop/main/…`), because npm and
  PyPI don't resolve paths in the repository.
- Before taking a fact out of the README, check the site already says it.

## Release notes

`release-notes/vX.Y.Z.md` opens the release on GitHub, ahead of the generated
list of pull requests. It answers two questions: what's new for me, and do I
have to change anything?

```markdown
<Lead: two or three sentences, under about 60 words, on what the release is about.>

- **<What you notice, in a few words>:** <One complete sentence, with one example.>
- **<…>:** <…>.

**After upgrading:** <If …, run …>.
```

- **One line per paragraph and per bullet.** GitHub turns every newline in a
  release body into a line break, so this is the one place not to break
  sentences onto lines.
- **At most six bullets,** one per change a user would notice. The label is
  capitalized and bold, and the sentence after the colon opens like any other
  sentence: never with a code span or "snoop".
- **Open on the outcome,** not on "Pressing `X`…": "A finished turn can wait
  for your reply: press `R`…".
- **Leave out mechanism and history.** Hooks, files and the bug's cause belong
  in the pull request.
- **A closing line only when there's something to do,** labeled
  **After upgrading:** (a step such as `snoop hooks install`) or
  **Breaking:**. When there is nothing to do, leave it out.
- **No headings,** versions written with their v (v0.3.1), and about 150
  words in all. A one-fix patch can be the lead alone.

## One fact, several places

The same facts live in several files. When one changes, update all of them
in the same commit:

| Fact | Where it lives |
| --- | --- |
| flags and defaults | `cmd/snoop/main.go`, `site/content/docs/reference/cli.md`, any README section that names the flag |
| keys | `internal/ui/help.go`, `site/content/docs/reference/keys.md`, the guides, any README section that names the key |
| alerts and notifications | `internal/ui/help.go`, README "Alerts", `site/content/docs/reference/alerts.md`, the live-board guide |
| install methods | README "Install", `site/content/docs/install.md`, `site/layouts/home.html`, `site/static/install.sh` |
| files snoop writes | `site/content/docs/reference/data.md`, README "Notes" for the hooks |
| tested Claude Code versions | README "Notes", `site/content/docs/_index.md` |

## Check

A PostToolUse hook runs `.claude/hooks/prose_lint.py` on every edit to the
README, `site/content/`, `site/layouts/home.html` and `release-notes/`. It
reports sentences that break rules 1, 2 and 4, hard-wrapped lines and lines
holding two sentences, README code blocks wider than 80 columns, and release
notes that break their shape. Run it on everything before committing:

```sh
python3 .claude/hooks/prose_lint.py README.md site/layouts/home.html $(find site/content -name '*.md') release-notes/*.md
```

The linter can't judge whether a sentence is complete (rule 3), so reread
captions, subtitles, list items and lead-ins yourself.
