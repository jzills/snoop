# Answering permission prompts — design

Status: approved 2026-10-29; spike done (see "Spike results"), which changed the default wait to 5 minutes and added the hook-PID check.

## Spike results (2026-10-29, Claude Code 2.1.314)

Run in a throwaway project with a `PermissionRequest` hook that logged its input, slept, then optionally printed a decision:

- **The terminal prompt doesn't wait for the hook.** It appeared within a second, while the hook was still sleeping.
- **The hook's answer closes the prompt.** A hook that printed `allow` after 3 seconds closed the prompt already on screen, and the transcript shows "Allowed by PermissionRequest hook".
- **Answering in the terminal kills the hook.** The hook's last line was never logged, so a killed hook leaves its request file behind.
- **Input fields** match the docs: `session_id`, `transcript_path`, `cwd`, `permission_mode`, `hook_event_name`, `tool_name`, `tool_input`, and `permission_suggestions`. A subagent request wasn't tried; `agent_id` and `agent_type` stay optional.
- **Diff review works.** The user sent a comment from the Diff tab in a real session, and the agent received it and acted on it correctly.

**What changed because of it** (decided by the builder, since the user was away and had asked for the work to go ahead):
- Waiting no longer delays anyone, so the default `ask-wait` is 5 minutes (range 1s to 30m), long enough to walk back to the board. The settings timeout stays `ask-wait + 10s`.
- The listening check stays. It keeps requests from being written when no snoop could show them.
- Because a killed hook can't clean up, snoop treats a request whose hook process has exited as gone: the board drops it, and it's deleted. Pressing `A` or `D` on it says `already answered`. The PID is in the request's file name.

## Context

The live board shows every running session, but it only guesses when one is waiting on you: a tool call with no result after `--stall` (30s) is marked `◌ waiting … · may be a permission prompt`. Even when the guess is right, you still have to find that session's terminal and answer the prompt there. With several agents running, permission prompts are most of the time they spend waiting.

Claude Code has a `PermissionRequest` hook that runs the moment it's about to ask for permission. The hook gets the tool and its full input, and can answer for you. snoop already installs hooks and keeps an inbox for review comments (`internal/review`), so it can use this hook to:
- know right away, and for certain, that a session is asking and what it's asking for;
- let you allow or deny from the live board or the session view;
- send desktop notifications you can act on ("Bash wants to run `npm test`").

This builds on the diff review work (`2026-10-27-diff-review-design.md`) and makes snoop a place to run several agents at once, not only watch them.

## Goals

- A session asking for permission shows up on the board within a second, with the tool and what it wants to do.
- You can allow it once, or deny it with an optional reason, without leaving snoop.
- A session's prompt never waits on snoop for long, and never waits at all when no snoop is open.

## Non-goals

- "Always allow" or any other permission rule (`updatedPermissions`), and changing the permission mode. Allow once and deny come first; rules may follow later.
- Editing the tool's input before allowing it (`updatedInput`).
- Answering other kinds of prompts, such as MCP elicitations or questions the agent asks you.
- Agents other than Claude Code.

## Anti-goals

- **A broken or closed snoop must never stall or block an agent.** On any error, or when no snoop is open, the hook prints nothing and exits at once, so the normal prompt appears.
- **Nothing is allowed without a deliberate keypress** on a request whose full input you can see. snoop never answers on its own, and a timeout always falls through to the normal prompt, never to allow or deny.
- **Opening and following a session must not get slower.** Reading requests means listing one small folder per poll and stays out of `Update`.
- **No surprise writes.** The hook is added only by `snoop hooks install`, and the README says exactly what it does.

## Hook behavior (from the Claude Code hooks docs, 2026-10-29)

- Input: the common fields (`session_id`, `cwd`, `transcript_path`, `permission_mode`, `hook_event_name`, and `agent_id` inside a subagent), plus `tool_name`, `tool_input`, `mcp_server` for MCP tools, and `permission_suggestions`. There's no `tool_use_id`.
- Output to answer: `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`, or `"behavior":"deny"` with an optional `"message"` (the reason, which Claude sees). Deny and ask rules still apply over a hook's allow.
- No output leaves the normal flow unchanged, so the prompt appears. **The exception:** in sessions that can't show a prompt (background subagents in non-interactive mode), no decision means the call is denied. That's Claude Code's behavior with or without snoop.
- Command hooks default to a 600s timeout. A timed-out hook's output is discarded.
- **Unknown:** whether Claude Code shows the terminal prompt while the hook is still running, or only after it returns. The design must be safe if the prompt waits for the hook (see Step 0).

## Part 1: the request and the hook

**Asks folder.** Next to the inbox, at `<state>/snoop/asks/<session-id>/`. `InboxDir`'s logic picks `<state>`, and the session ID is checked the same way `sessionDir` checks it. Each request is one JSON file named `<unix-nanos>-<pid>.json`, written under a temporary name and renamed into place. It holds:
- the session ID, `cwd` and `agent_id`;
- the tool name and `tool_input`, kept verbatim as raw JSON;
- the time it was asked, and the time the hook stops waiting (its deadline).

**Answer.** snoop answers by writing `<same name>.answer`, also through a temporary name and a rename, holding `{"behavior":"allow"}` or `{"behavior":"deny","message":"…"}`.

**`snoop hook` on `PermissionRequest`:**
1. If no snoop is listening (see below), print nothing and exit.
2. Write the request file.
3. Poll for the answer file every 100ms until the deadline (`ask-wait`, default 5 minutes; see Spike results).
4. With an answer, print the decision in the shape above, then delete both files.
5. Without one by the deadline, delete the request file and print nothing, so the normal prompt appears.
6. On any error, write it to stderr, print nothing and exit 0, as the hook does today.

The hook must stay fast in the common case (step 1), so it reads only its input and the presence file, as the comment hook does.

**Listening.** While a live board or a session view is open, snoop rewrites `<state>/snoop/listening` every 2 seconds with its PID and the time. The hook treats snoop as listening if that file was written in the last 5 seconds. One file covers every open snoop. A session view counts as listening for every session, not only its own. That's simpler, and the cost when you're not watching that session is at most `ask-wait` seconds before the normal prompt.

**Deciding first.** Several snoops can be open, and you can also answer in the terminal (if the prompt shows while the hook runs). The first snoop to answer claims the request by renaming the request file to `.claimed` before writing the answer, so only one answer is ever written. Once claimed, other snoops drop the request. If the request disappears before anyone answers (because of a timeout, or an answer in the terminal), snoop drops it, and an `A` or `D` pressed too late shows `already answered` in the footer.

**Hook timeout in settings.** The hooks installed today use a 10s timeout. The `PermissionRequest` entry needs `ask-wait` plus a margin (310s with the 5-minute default), so Claude Code doesn't kill the hook before its own deadline.

**`ask-wait` setting.** `snoop hooks install --ask-wait 15s` writes the wait into the hook command (`"…/snoop" hook --ask-wait 15s`) and sets that entry's timeout to match. `IsHookCommand` must still recognize the command, so `uninstall` and `status` find it. Running `install` again with a different value updates the entry.

**Leftovers.** A request whose deadline passed more than a minute ago was left by a hook that died. Any snoop that sees one deletes it, and the hook deletes any it finds for its own session.

## Part 2: the board and the session view

**Board state.** A new state above `◌ waiting`, `? asking`, uses the accent color in bold, and its rows come first:

```
api        ? asking   Bash: npm test -race ./...            ctx 41%  $1.20  now
```

The detail line names the tool and a one-line summary of the input:
- Bash: the command;
- Edit and Write: the path;
- WebFetch: the URL;
- MCP: `server·tool`;
- anything else: the tool name.

A request from a subagent adds its type (`via Explore`). The header counts asking sessions in `N waiting on you`, along with stalled ones.

When a session has an open request, its stall guess is replaced by `asking`. A session without the hook (or with no snoop listening when it asked) keeps the 30s guess as today.

**Seeing it in full.** With an asking row selected, the board shows the request in full under the list or in a pane: the Bash command with its description, or the Edit diff (reusing `detail.go`'s rendering), and the session's `cwd`. You always see the whole input before you can answer.

**Keys** (on the board, and in the session view when that session is asking):
- `A` allows once;
- `D` opens a one-line text input for an optional reason, like the comment input; `enter` denies (with an empty reason, Claude gets no message), and `esc` cancels.

They're capitals to avoid accidents, and because `a` and `y` already have meanings in the session view. Neither is bound anywhere in `internal/ui` today. When a session has more than one open request (parallel subagents), the keys answer the oldest, and the detail shows `1 of 3`.

**Session view.** An open request raises an alert in the header (`? Bash wants to run npm test`). `!` jumps to the related `◌` tool row in the timeline when the transcript shows it. `A` and `D` work from anywhere in the session view while the alert stands, except in text inputs and the Diff tab's comment input.

**After answering.** The footer shows `allowed` or `denied`. The row leaves `asking` when the request is gone, and the transcript shows the result as usual.

**Notifications.** A new alert kind, `ask`, is on by default and can be chosen in `--alerts`. With `--notify`, it sends `api · Bash wants to run npm test` through the existing notifier, with the same per-minute limit and the same between-snoops dedupe. Because the asks folder is read on the board's poll, an asking session raises the alert within one poll.

## Part 3: install and status

- `snoop hooks install` adds `PermissionRequest` to `HookEvents`. Running it again on an existing install adds the missing entry and leaves the others alone.
- `snoop hooks status` lists any of snoop's events that are missing, so an install from before this feature says that `PermissionRequest` is missing.
- The Diff tab's "hooks installed" check keeps requiring only the three comment events. The board shows asks only from the hook, so it needs no check, but `?` help mentions `snoop hooks install` next to `A` and `D`.

## Part 4: docs

- The README and `site/content/docs/` get a section on answering permission prompts: what you see, `A` and `D`, `ask-wait`, the listening file, what happens when snoop is closed, and the non-interactive deny caveat. Follow the `writing-docs` skill and keep the copies in sync.
- The live board's state table gains `? asking`.
- `?` help lists `A` and `D`.
- `demo/` gets a board screenshot with an asking row, from fictional sessions.

## Build order

Step 0 is a throwaway spike. The rest is one commit per step, tests first.

0. **Spike: does the prompt wait for the hook?** Run a real interactive `claude` session with a test `PermissionRequest` hook that sleeps 10s, then prints nothing. Note whether the terminal prompt appears at once or after 10s, and whether answering in the terminal while the hook sleeps works and cancels the hook. Run the same check with a hook that answers `allow`, and with a subagent. Write the results into this document. If the prompt does wait for the hook, keep the 15s default. If it doesn't, the default can be much longer (60s), and that change belongs to the user. Also confirm in the spike that the 3 comment events still work (the open item from diff review).
1. **`internal/review`**: the asks folder (write, claim, answer, wait, clean up leftovers), the listening file, and the `PermissionRequest` branch of `Hook`, including the "not listening" fast path and the deadline. Tested with temporary folders and a fake clock where possible.
2. **Install and status** for the new event, `--ask-wait`, its per-entry timeout, and recognizing the new command form. Tested against temporary settings files, including an install from before this feature.
3. **UI**: the listening heartbeat (in a `tea.Cmd` on a tick), reading asks on the board's poll, the `asking` state, the full-request detail, `A`/`D` with the reason input, the session alert, the `ask` alert kind and notifications.
4. **Docs, help and the screenshot.**

## Calls I made

- Allow once and deny only. The user chose this. No rules, no editing the input.
- The hook waits only while some snoop is listening, and a timeout always falls through to the normal prompt.
- A 5-minute default wait, after the spike showed the prompt never waits for the hook (first proposed as 15s).
- One listening file for all of snoop, rather than per session.
- `A` and `D`, capitals, with an optional reason on deny.
- Files and atomic renames for requests and answers, like the inbox, rather than a socket. That keeps the hook a short-lived command with no server.
- The 30s stall guess stays for sessions without the hook.

## Left to the builder

- Package layout: whether asks live in `internal/review` or a new package beside it.
- Where exactly the full request renders on the board (a pane or under the list), within the TUI look rules: keep the colors, and use whitespace rather than density.
- How to summarize unusual tool inputs in one line.
- How to match a request to its `◌` row in the timeline. The hook gets no `tool_use_id`, so the newest unfinished call with the same tool name and input is the likely match. If none matches, `!` does nothing.
- Poll intervals within the limits above.
