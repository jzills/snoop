---
title: Install
weight: 2
---

You can install snoop, a single binary for macOS, Linux and Windows, in any
of these ways.

{{< tabs >}}
  {{< tab name="Install script" >}}
Run this on macOS or Linux:

```sh
curl -fsSL https://jzills.github.io/snoop/install.sh | sh
```

The script picks the build for your machine, checks it against the
release's `SHA256SUMS`, and installs it to `~/.local/bin` without sudo.

| Variable | Effect |
| --- | --- |
| `SNOOP_INSTALL_DIR` | install somewhere other than `~/.local/bin` |
| `SNOOP_VERSION` | install a given release, such as `v0.1.0`, instead of the newest |

Release builds cover Linux and macOS on amd64 and arm64. Elsewhere, the
script stops and suggests installing with Go. On Windows, use the PowerShell
script on the Windows tab.
  {{< /tab >}}
  {{< tab name="Homebrew" >}}
Run this on macOS or Linux:

```sh
brew install jzills/tap/snoop
```

Update it with `brew upgrade snoop`.
  {{< /tab >}}
  {{< tab name="npm or PyPI" >}}
Install it with npm, pipx or uv:

```sh
npm install -g snoop-cli
pipx install snoop-cli
uv tool install snoop-cli
```

The package is named `snoop-cli`, since `snoop` was taken on both
registries, and the command it installs is `snoop`. Each package carries the
binary for your platform, so nothing is downloaded or built when it
installs, and you don't need Node or Python for anything else. Update it
with `npm update -g snoop-cli`, `pipx upgrade snoop-cli` or
`uv tool upgrade snoop-cli`.
  {{< /tab >}}
  {{< tab name="Windows" >}}
Install it with the PowerShell script, which checks the download against the
release's `SHA256SUMS`, installs `snoop.exe` to
`%LOCALAPPDATA%\Programs\snoop` and adds that folder to your `PATH`:

```powershell
irm https://jzills.github.io/snoop/install.ps1 | iex
```

The script reads `SNOOP_INSTALL_DIR` and `SNOOP_VERSION` like the shell
script. Builds cover Windows on x64 and arm64. The npm and PyPI packages
work on Windows too.
  {{< /tab >}}
  {{< tab name="Go" >}}
With Go 1.26 or later installed, run:

```sh
go install github.com/jzills/snoop/cmd/snoop@latest
```

The binary goes to `$(go env GOPATH)/bin`.
  {{< /tab >}}
  {{< tab name="Binary" >}}
Download the archive for your platform from the
[latest release](https://github.com/jzills/snoop/releases/latest). Each
archive holds the `snoop` executable, the license and the README. Put the
executable somewhere on your `PATH`.
  {{< /tab >}}
{{< /tabs >}}

## Check it works

```sh
snoop --version
```

If your shell says `snoop` isn't found, the install directory isn't on your
`PATH`. For the install script, add `~/.local/bin`:

```sh
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc   # or ~/.bashrc
```

## Update and uninstall

Run the same install command again to update; it replaces the binary in
place. To uninstall, use the package manager you installed with (such as
`npm uninstall -g snoop-cli` or `pipx uninstall snoop-cli`), or delete the
binary, and optionally the cache described in
[What snoop reads and writes](../reference/data/):

```sh
rm ~/.local/bin/snoop
```

## Next

The [Quickstart](../quickstart/) covers opening and following a session.
