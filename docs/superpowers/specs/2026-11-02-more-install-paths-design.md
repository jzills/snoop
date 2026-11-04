# More install paths: npm, PyPI and Windows — design

Status: built 2026-11-02, not yet released. Scoop and winget were built and then dropped on 2026-11-02 at the user's request; Windows is covered by `install.ps1`, npm and PyPI for now, and the sections on them below are kept for when they come back. Deviations: npm and PyPI publish on every release (they were briefly behind the repository variables `NPM_PUBLISH` and `PYPI_PUBLISH`, removed once the user had set up both registries); the PyPI publisher has no environment, since a second job on the `release` environment would ask for approval again; winget manifests are written by `scripts/windows_manifests.sh` and submitted with `wingetcreate submit` rather than komac; the packaged README has its relative links made absolute (`scripts/package_readme.sh`).

## Context

snoop installs today through the install script, Homebrew (`jzills/tap`), `go install` and the GitHub Release archives. Developers who live in Node or Python tooling expect `npm i -g` or `pipx install`, and Windows users have only a zip to unpack by hand. The user wants npm, PyPI and Windows covered, with one consistent name.

## The name

`snoop` is taken on npm (a test-spy library) and PyPI (a Python debugging tool), and Chocolatey has an unrelated `snoop`. `snoop-cli` returned 404 on npm and PyPI on 2026-11-02.

- **Package name `snoop-cli`** on every shared registry: npm, PyPI (and Chocolatey, if it's ever added).
- **The command is always `snoop`.** Every package installs a `snoop` executable.
- Names scoped to the user stay as they are: `jzills/tap/snoop` in Homebrew, `jzills.snoop` in winget, `snoop` in the user's own Scoop bucket.
- A name is only held once a version is published. The first publish is a real release (v0.2.0), not a placeholder, since PyPI discourages reserving names.

## Goals

- Install with one command from npm, PyPI, Scoop or winget, and get the same `snoop` binary the GitHub Release ships, checked against its checksums.
- Publish everything from the existing tag-driven release, behind the existing `release` approval gate, with nothing to do by hand per release beyond approving.
- Prefer short-lived OIDC credentials (trusted publishing) to long-lived tokens wherever the registry supports it.

## Non-goals

- Building snoop from source on install (no Go toolchain needed by users).
- Chocolatey: its `snoop` name is taken, it needs its own moderation, and Scoop plus winget cover Windows.
- Linux distribution packages (apt, rpm, AUR, Nix).

## Anti-goals

- **No install-time downloads.** npm and PyPI packages carry the binary, so installs work behind proxies and with `--ignore-scripts`, and the registry's own integrity checks cover it.
- **A failed channel never blocks the others.** Each publish job is skipped with a warning when its credential is missing, as the Homebrew job is today.
- **Nothing is published before approval.** All packaging runs in the `build` job, which writes nothing outside the runner.

## npm

The esbuild pattern: a small main package plus one package per platform.

- **`snoop-cli`**: `bin: {"snoop": "bin/snoop.js"}`, and `optionalDependencies` on the six platform packages at the same version. `bin/snoop.js` resolves the package for `process.platform`/`process.arch`, then runs its binary with the same arguments, stdio inherited, and exits with its code. If no package matches (or optional dependencies were skipped), it prints one line pointing to the install script and exits 1.
- **Platform packages** `@jzills/snoop-cli-{linux,darwin,win32}-{x64,arm64}`: each holds just the binary, with `os` and `cpu` set so npm installs only the matching one. Scoped under the user's npm name, so the six names can't collide.
- `scripts/npm_packages.sh <version>` writes all seven package folders under `dist/npm/` from the binaries the build already made, plus the README and license.
- **Publishing:** npm trusted publishing (OIDC from the release workflow, `npm publish --provenance`). Trusted publishing can only be set up for a package that exists, so the first release publishes with an `NPM_TOKEN` (a granular token with publish rights), and the token is removed once each package has the workflow as its trusted publisher. Platform packages publish first, the main package last.

## PyPI

One wheel per platform, each holding the binary as a script, as ruff does.

- `scripts/wheels.py <version>` (standard library only) writes `snoop_cli-<version>-py3-none-<tag>.whl` for each target, with the binary at `snoop_cli-<version>.data/scripts/snoop[.exe]`, so pip, pipx and `uv tool` put `snoop` on PATH. Tags: `manylinux_2_17_x86_64.musllinux_1_1_x86_64`, `manylinux_2_17_aarch64.musllinux_1_1_aarch64` (the binary is static, so it runs on both), `macosx_10_12_x86_64`, `macosx_11_0_arm64`, `win_amd64`, `win_arm64`. `METADATA` carries the README as its description, and `RECORD` the hashes.
- No sdist: there's nothing to build from it without Go.
- **Publishing:** PyPI trusted publishing. PyPI allows a "pending publisher" for a project that doesn't exist yet, so no token is needed even for the first release.

## Windows

- **Scoop:** a bucket repository `jzills/scoop-bucket` holding `bucket/snoop.json` (URLs and SHA-256 hashes of the two Windows zips, `bin: "snoop.exe"`, with `checkver` and `autoupdate` against GitHub Releases). The release workflow renders the manifest in `build` and commits it in a job like the Homebrew one, with a `SCOOP_BUCKET_TOKEN` (fine-grained, Contents read/write on that repo). Usage: `scoop bucket add jzills https://github.com/jzills/scoop-bucket` and then `scoop install snoop`.
- **winget:** package `jzills.snoop`, a portable install of the zip's `snoop.exe`. A job runs `komac update` (or `komac new` the first time) to open a pull request to `microsoft/winget-pkgs`, with a `WINGET_TOKEN` (a classic token with `public_repo`, from an account with a fork of `winget-pkgs`). Microsoft reviews the first submission, which can take days; later versions usually merge automatically. Usage: `winget install jzills.snoop`.
- **PowerShell install script:** `install.ps1` beside `install.sh`, served from the site: it picks the zip for the machine's architecture, checks it against `SHA256SUMS`, extracts `snoop.exe` to `%LOCALAPPDATA%\Programs\snoop`, and adds that folder to the user's PATH. It honors `SNOOP_INSTALL_DIR` and `SNOOP_VERSION` as `install.sh` does. Usage: `irm https://jzills.github.io/snoop/install.ps1 | iex`.

## Checking before publishing

In the `build` job, after the archives are checked:
- **npm:** `npm pack` every package, install the main package and the Linux x64 package into a temporary prefix, and run `snoop --version`, which must print the tag's version.
- **PyPI:** check every wheel's `RECORD` and tags, install the Linux x64 wheel into a fresh virtual environment, and run `snoop --version`.
- **Scoop and winget:** render the manifests and check their hashes against `SHA256SUMS`.

A matrix job in CI does the same installs on macOS and Windows runners, for both npm and PyPI, and runs `install.ps1` against a snapshot on Windows, so a platform-specific break shows up before a release.

## Docs

Every install method is listed in four places that must stay in sync (see the `writing-docs` skill): README "Install", `site/content/docs/install.md`, the landing page's install tabs, and `site/static/install.sh`/`install.ps1`. `RELEASING.md` gains the one-time setup below and what each release now publishes.

## One-time setup (the user's)

1. **npm:** an account that owns the `jzills` scope; create `NPM_TOKEN` (granular, publish) for the first release; afterwards add the release workflow as trusted publisher on each of the seven packages and delete the token.
2. **PyPI:** an account; add a pending trusted publisher for project `snoop-cli`, repository `jzills/snoop`, workflow `release.yml`, environment `release`.
3. **Scoop:** create the public repository `jzills/scoop-bucket` and the `SCOOP_BUCKET_TOKEN` secret.
4. **winget:** fork `microsoft/winget-pkgs` to the user's account and add the `WINGET_TOKEN` secret.

## Build order

1. npm packages and the launcher, with the local install check.
2. PyPI wheels, with the local install check.
3. Scoop manifest, winget job and `install.ps1`, with the Windows checks.
4. The release workflow's publish jobs, the CI matrix, `RELEASING.md`, and the docs.
5. Release v0.2.0 (it also carries everything merged since v0.1.0) once the setup is done.

## Calls made

- `snoop-cli` as the registry name, `snoop` as the command (the user asked for one consistent name).
- Binaries inside the packages rather than downloaded at install.
- Scoped `@jzills/…` names for npm's platform packages.
- No Chocolatey and no sdist.
- Scoop through the user's own bucket rather than Scoop's main bucket, which has its own review and naming rules.
