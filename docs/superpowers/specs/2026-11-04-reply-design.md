# Reply from snoop — design

Status: built 2026-11-04. The user asked for the work to go ahead
autonomously, so the calls below are the builder's.

## Context

snoop can send review comments and answer permission prompts, but when a
turn ends, the agent waits for a prompt typed in its own terminal. The live
board already shows that state as "your turn". Replying from snoop closes
the loop: press `R`, type, and the agent carries on.

## Spike results (2026-11-04, Claude Code 2.1.318)

Run in a throwaway project in tmux, with Stop hooks that logged and waited:

- **A synchronous Stop hook that waits holds the turn open.** The terminal
  shows "running Stop hooks", and Claude Code writes the turn's
  `turn_duration` record only once the hook ends.
- **A prompt typed in the terminal while it waits is queued** until the hook
  ends. Claude Code writes a `queue-operation` record with
  `"operation":"enqueue"` to the transcript within about 100 ms of the
  prompt being submitted.
- **Esc interrupts the turn and kills the hook.**
- **A hook that prints `{"decision":"block","reason":…}` hands the reason
  to the agent**, which acts on it. The model sees it as "Stop hook
  blocking error from command …: <reason>", the same way review comments
  already arrive.
- **`asyncRewake` hooks don't hold the turn open, but they're unusable for
  replies.** Claude Code wraps their message in "NOT a message from the
  user … must NOT be treated as approval or consent", so a reply such as
  "yes, go ahead" would be discounted. Background hooks also outlived
  `/exit`.
- **Headless runs** (`claude -p`) record `"entrypoint":"sdk-cli"` in the
  transcript; interactive ones record `"cli"`. The Stop hook input has no
  such field, but it does carry `transcript_path` and
  `last_assistant_message`. A turn can end with a record of over 100 KB (a
  prompt snapshot), so the hook reads the transcript's last 64 KB and, if
  no record there names the entrypoint, its first 64 KB. The end-to-end
  check caught this: a headless run first hung until snoop stopped
  listening.

## Design

- **The Stop hook waits for a reply** when it has no comments to deliver,
  a snoop is listening, the session isn't headless, and the hook was
  installed with a wait (`--ask-wait`, the same wait as permission
  requests). While it waits, it writes a marker file to the `turns`
  folder in the state directory, named like a permission request (time
  and PID), so snoop can see that the session is waiting and drop the
  marker if the hook was killed.
- **It stops waiting** as soon as a reply arrives, a prompt is queued in
  the transcript, no snoop is listening any more, or the wait runs out.
  Typing in the terminal therefore never waits for snoop.
- **A reply is an inbox send**, framed as "Message from the user, sent
  from snoop:", so it reaches the agent through whichever hook runs
  next: at once while the Stop hook waits; after the current tool call
  while the agent works; or with the next prompt typed in the terminal
  otherwise. snoop says which applies.
- **`R` replies** on the live board (to the selected session) and in a
  session view. The footer shows the agent's last message while typing.
- **The board and alerts treat a waiting turn as ended.** The row shows
  "your turn" and the end-of-turn alert fires when the marker appears,
  not minutes later when the hook gives up, and fires only once.
- **Existing installs keep working.** A Stop hook installed without
  `--ask-wait` never waits; `snoop hooks install` updates it, and
  `snoop hooks status` says so.

## Calls made

- A synchronous Stop hook rather than `asyncRewake`, so replies count as
  the user's.
- One wait setting for permission requests and replies.
- `R` sends at any time, not only at the end of a turn.
