---
name: check
description: Use before saying a change to snoop is done, and before every commit — runs the same checks CI runs (gofmt, vet on Linux, macOS and Windows, race tests, govulncheck, the site build, the prose linter) and covers snoop's testing and commit conventions.
---

# Checking a change

Run these from the repository root and read the output. Don't report a
change as done, or commit it, until they pass.

```sh
gofmt -l .                                   # prints nothing
go vet ./...
GOOS=windows go vet ./... && GOOS=darwin go vet ./...
go test -race ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
```

Add these when the change touches the site or user-facing text:

```sh
(cd site && hugo --gc --minify -d /tmp/out)  # no errors or warnings
python3 .claude/hooks/prose_lint.py README.md site/layouts/home.html $(find site/content -name '*.md')
```

When a change touches parsing or loading, check that opening a session didn't
get slower. `go run ./cmd/probe` parses every local transcript and reports
timings; compare them with a run on the previous commit.

## Testing conventions

- **Write the test first** for a bug, and check that it fails on the old
  code before fixing it (`git stash` the fix, run the test, `git stash pop`).
- **Test behavior through the model.** The UI tests drive `App.Update` with
  key messages and inspect `View()` or the view's state, as in
  `internal/ui/*_test.go`.
- **Fixtures are JSONL lines** in the test files. Never read real
  transcripts in tests.
- For terminal behavior that tests can't reach (focus, notifications,
  escape sequences), drive the built binary in a pty with Python's `pty`
  module, as a smoke test.

## Commit conventions

- Commit only when asked, and push only when asked.
- No attribution lines (`Co-Authored-By` and the like) in commits or pull
  requests.
- Write the subject as what the change does, in the imperative, without a
  prefix: "Name a subagent's export after the subagent".
- One concern per commit. A formatting-only change gets its own commit.
- Nothing is published (releases, packages, the site) without the user
  saying so.
