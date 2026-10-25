---
title: Overview
weight: 1
---

This documentation covers snoop, a terminal viewer for Claude Code sessions.
You can keep it open beside `claude` to watch a session as it works, or open
any past session to see what each prompt cost, how full the context got,
which files changed, what the subagents did and where the time went.

It reads the transcripts Claude Code already writes to `~/.claude/projects`.
There is nothing to configure, and nothing is sent anywhere.

## Get started

Install snoop, then run it in any terminal:

```sh
curl -fsSL https://jzills.github.io/snoop/install.sh | sh
snoop
```

The first screen lists your sessions, newest first. Press `enter` to open one.
The [Quickstart](quickstart/) walks through the rest in five minutes, and
[Install](install/) covers Homebrew, Go and downloading a binary.

## Learn more

{{< cards >}}
  {{< card link="guides/following/" title="Following a session" subtitle="Watch a session live in a second terminal, and read every call in full." >}}
  {{< card link="guides/cost-and-context/" title="Cost and context" subtitle="See what each prompt cost, how full the context is and where the time went." >}}
  {{< card link="guides/live-board/" title="The live board" subtitle="See every session at work across your projects and which ones need you." >}}
  {{< card link="reference/keys/" title="Keyboard shortcuts" subtitle="Look up every key on every screen." >}}
{{< /cards >}}

## How it works

Claude Code writes each session to a JSONL transcript under
`~/.claude/projects/<project>/`, one record per line, and subagents to
transcripts of their own beside it. When you open a session, snoop parses
its file, and after that it reads only the lines added since. A 75 MB
transcript opens in about 150 ms.

The transcript format is undocumented and changes between Claude Code
versions. Records that snoop doesn't recognize are ignored, so a new version
may show less rather than break. Testing covers Claude Code 2.1.280 through
2.1.310. If something goes missing, please
[open an issue](https://github.com/jzills/snoop/issues).
