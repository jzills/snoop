---
title: "snoop — watch Claude Code work"
toc: false
---

<div class="snoop-hero">
  <span class="snoop-hero__logo">{{< snoop-mark >}}</span>
  <p class="snoop-hero__name">snoop</p>
  <p class="snoop-hero__tagline">Watch Claude Code work.</p>
  <p class="snoop-hero__blurb">
    A fast terminal viewer for Claude Code sessions. Follow one live in a
    second terminal, or open any past session to see what each prompt cost,
    how full the context got, which files changed, what the subagents did
    and where the time went.
  </p>
</div>

<div class="snoop-section">
  <img class="snoop-demo" src="demo/demo.gif" width="1400" height="820"
    alt="snoop showing the summary across sessions, a session's timeline with a turn's waterfall, an expanded subagent, the Files tab and Stats">
</div>

<div class="snoop-section" id="install">
  <h2 class="snoop-section__title">Install</h2>
  <p class="snoop-section__lede">
    One self-contained binary for Linux, macOS and Windows. It reads the
    transcripts Claude Code already writes, so there is nothing to configure.
  </p>
  <div class="snoop-install">
    <div class="snoop-install__card snoop-install__card--lead" data-snoop-copy>
      <div class="snoop-install__head">
        <div class="snoop-install__label">Linux &amp; macOS</div>
        <button type="button" class="snoop-install__copy" aria-label="Copy the install script command" hidden><svg viewBox="0 0 16 16" aria-hidden="true" class="snoop-install__icon snoop-install__icon--copy"><rect x="5" y="5" width="9" height="9" rx="1.5"/><path d="M11 5V3.5A1.5 1.5 0 0 0 9.5 2h-6A1.5 1.5 0 0 0 2 3.5v6A1.5 1.5 0 0 0 3.5 11H5"/></svg><svg viewBox="0 0 16 16" aria-hidden="true" class="snoop-install__icon snoop-install__icon--done"><path d="M3 8.5 6.5 12 13 4.5"/></svg></button>
      </div>
      <div class="snoop-install__command">curl -fsSL https://jzills.github.io/<wbr>snoop/<wbr>install.sh | sh</div>
      <p class="snoop-install__note">Checks the download against the release&rsquo;s checksums and installs to <code>~/.local/bin</code>, without sudo.</p>
      <span class="snoop-install__status" role="status"></span>
    </div>
    <div class="snoop-install__card" data-snoop-copy>
      <div class="snoop-install__head">
        <div class="snoop-install__label">Homebrew</div>
        <button type="button" class="snoop-install__copy" aria-label="Copy the Homebrew install command" hidden><svg viewBox="0 0 16 16" aria-hidden="true" class="snoop-install__icon snoop-install__icon--copy"><rect x="5" y="5" width="9" height="9" rx="1.5"/><path d="M11 5V3.5A1.5 1.5 0 0 0 9.5 2h-6A1.5 1.5 0 0 0 2 3.5v6A1.5 1.5 0 0 0 3.5 11H5"/></svg><svg viewBox="0 0 16 16" aria-hidden="true" class="snoop-install__icon snoop-install__icon--done"><path d="M3 8.5 6.5 12 13 4.5"/></svg></button>
      </div>
      <div class="snoop-install__command">brew install jzills/tap/snoop</div>
      <span class="snoop-install__status" role="status"></span>
    </div>
    <div class="snoop-install__card snoop-install__card--wrap" data-snoop-copy>
      <div class="snoop-install__head">
        <div class="snoop-install__label">Go</div>
        <button type="button" class="snoop-install__copy" aria-label="Copy the go install command" hidden><svg viewBox="0 0 16 16" aria-hidden="true" class="snoop-install__icon snoop-install__icon--copy"><rect x="5" y="5" width="9" height="9" rx="1.5"/><path d="M11 5V3.5A1.5 1.5 0 0 0 9.5 2h-6A1.5 1.5 0 0 0 2 3.5v6A1.5 1.5 0 0 0 3.5 11H5"/></svg><svg viewBox="0 0 16 16" aria-hidden="true" class="snoop-install__icon snoop-install__icon--done"><path d="M3 8.5 6.5 12 13 4.5"/></svg></button>
      </div>
      <div class="snoop-install__command">go install github.com/<wbr>jzills/snoop/<wbr>cmd/snoop@latest</div>
      <span class="snoop-install__status" role="status"></span>
    </div>
  </div>
  <p class="snoop-fineprint">
    Or download a binary from the
    <a href="https://github.com/jzills/snoop/releases/latest">latest release</a>.
    Then run <code>snoop</code> to pick a session, or <code>snoop --follow</code>
    to watch this directory&rsquo;s newest one.
  </p>
</div>

<div class="snoop-section" id="features">
  <h2 class="snoop-section__title">What you see</h2>
  <p class="snoop-section__lede">
    Everything comes from the session&rsquo;s transcript on your machine.
    Nothing is sent anywhere.
  </p>
  <div class="snoop-features">
    <div class="snoop-feature">
      <p class="snoop-feature__title">Every session at a glance</p>
      <p class="snoop-feature__body"><code>snoop --live</code> lists every session at work across your projects, and which ones are waiting on you. Add <code>--notify</code> for a desktop notification when one needs you, or <code>--follow</code> one session as it happens.</p>
    </div>
    <div class="snoop-feature">
      <p class="snoop-feature__title">What each prompt cost</p>
      <p class="snoop-feature__body">Each prompt shows its tokens, an estimated cost and its duration, and the header shows how full the context window is, turning red as it nears the limit.</p>
    </div>
    <div class="snoop-feature">
      <p class="snoop-feature__title">Where the time went</p>
      <p class="snoop-feature__body">Select a prompt to see its five slowest tool calls and a waterfall of every call by start and end, so parallel calls show as parallel.</p>
    </div>
    <div class="snoop-feature">
      <p class="snoop-feature__title">Subagents, inline</p>
      <p class="snoop-feature__body">Press <code>x</code> on an Agent call to expand what the subagent did under it, kept live while it runs. Search and filters reach inside subagents too.</p>
    </div>
    <div class="snoop-feature">
      <p class="snoop-feature__title">Every file it touched</p>
      <p class="snoop-feature__body">The Files tab lists each file read, edited or written, with lines added and removed, and steps through every change as a diff.</p>
    </div>
    <div class="snoop-feature">
      <p class="snoop-feature__title">Across all your sessions</p>
      <p class="snoop-feature__body">Find the session that touched a file, ran a command or got a prompt. Or see a summary of them all: cost by day, error rates, and the tools, skills, MCP servers and models you use most.</p>
    </div>
  </div>
  <p class="snoop-fineprint">
    snoop is an independent project, not affiliated with or endorsed by
    Anthropic. Claude and Claude Code are trademarks of Anthropic.
    <a href="https://github.com/jzills/snoop/blob/main/LICENSE">MIT licensed</a>.
  </p>
</div>
