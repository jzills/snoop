# Branch scope in the Diff tab — design

Status: approved 2026-10-30.

## Context

The Diff tab shows `git diff HEAD` plus untracked files. Once the agent commits, its work leaves the tab, so reviewing everything a branch changed means opening a PR on GitHub or Azure DevOps. The user wants to see and comment on all of a branch's changes locally, so that review feeds straight back into the agent's loop.

## Goals

- See every change on the branch, committed or not, in the Diff tab, and comment on it as today.
- Need no setup in the common workflows: a feature branch off `main`, or off `develop`.
- Always make the comparison visible, and make a wrong guess one key to fix.

## Non-goals

- Reviewing commit by commit, or marking hunks as committed or uncommitted.
- Comparing against an arbitrary commit.
- Asking GitHub or Azure DevOps for a PR's target branch.

## Anti-goals

- **Comments must keep working unchanged.** Branch scope diffs the working tree, so new-side line numbers are the files on disk, as today.
- **Opening a session must not get slower.** Git runs only when the Diff tab loads, in a `tea.Cmd`.
- **No surprise writes.** snoop writes to git config only when you pick a base with `B`.

## Scopes

The tab has two scopes, shown in a line at the top of the tab:

```
 Uncommitted   Branch · develop…feat/rate-limit · 4 commits          b switch · B base
```

The active scope is bold in the accent color; the other is dim. `b` switches.

- **Uncommitted** is today's tab: `git diff HEAD` plus untracked files.
- **Branch** is `git diff <merge-base>` against the working tree, plus untracked files, where the merge base is `git merge-base <base> HEAD`. It's what a PR would show, plus uncommitted work. Files that also have uncommitted changes (in `git diff HEAD --name-only`, or untracked) are tagged `uncommitted`, like the existing `new` and `deleted` tags.

**Default.** On first load, snoop guesses the base and counts the commits since the merge base. With at least one commit, the tab opens in Branch; with none, in Uncommitted. Once you press `b`, your choice holds for every session view for the rest of the run (like the Files tree).

**Refresh.** Unchanged: the tab reloads when the session edits files or finishes a Bash call (which covers the agent's commits), and `r` reloads.

## The base

`B` lists bases to choose from; the choice is saved for the current branch.

The guess, in order:
1. `git config branch.<current>.snoopBase`, set by `B`.
2. `git config snoop.base`, for a whole repo.
3. The closest of the usual bases: for each of `develop`, `main`, `master` and `trunk`, the remote-tracking `origin/<name>` if it exists, otherwise the local branch. Closest means the fewest commits in `<base>..HEAD`. Ties go to the earlier name in that list (so `develop` wins a tie with `main`, since a repo with `develop` merges into it). Remote-tracking refs come first because a stale local `main` would pull other people's commits into the diff.
4. With none of those, the tab stays in Uncommitted and says there's no base.

A guess or a configured base that no longer resolves falls through to the next step.

**`B`** opens a list in place of the file list: the candidates from step 3 with their commit counts, then up to 10 other branches by most recent commit (local first, then `origin/`), excluding the current branch. `enter` picks, saving `branch.<current>.snoopBase` with `git config`, switches to Branch and reloads; `esc` cancels. A detached HEAD has no current branch, so the choice isn't saved.

## Build

One branch, tests first:
1. `internal/review`: `LoadBranch(dir, base)`, `GuessBase(dir)`, `Bases(dir)`, `SaveBase(dir, base)`, with tests against temporary repositories.
2. `internal/ui`: the scope line, `b`, `B` and its list, the `uncommitted` tag, the default, the shared preference.
3. Help, README, the review guide, the keys and data references, and the Diff screenshot.

## Calls made

- The base guess and its tie-break, with remote-tracking refs preferred (agreed with the user).
- The choice from `B` is saved per branch in git config; `snoop.base` is read but never written.
- No separate HEAD watcher: Bash calls already trigger a reload, and `r` covers commits made outside the session.
- The tag reads `uncommitted` rather than a glyph, matching `new` and `deleted`.
