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
script stops and suggests installing with Go.
  {{< /tab >}}
  {{< tab name="Homebrew" >}}
Run this on macOS or Linux:

```sh
brew install jzills/tap/snoop
```

Update it with `brew upgrade snoop`.
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
place. To uninstall, delete the binary, and optionally the cache described
in [What snoop reads and writes](../reference/data/):

```sh
rm ~/.local/bin/snoop
```

## Next

The [Quickstart](../quickstart/) covers opening and following a session.
