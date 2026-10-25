# Releasing

Releases are cut by pushing a version tag. `.github/workflows/release.yml`
then tests, cross-compiles and checks every archive, waits for approval, and
publishes.

## One-time setup

1. **Make the repository public.** Homebrew and `go install` download from it.
2. **Protect the `release` environment** (Settings → Environments → `release`
   → Required reviewers: you). The publish jobs wait for your approval there;
   without a reviewer GitHub creates the environment unprotected and they run
   straight away.
3. **Add snoop to the tap** `jzills/homebrew-tap` (it already holds kx;
   Homebrew finds it as `jzills/tap`): a README entry, and `snoop` alongside
   `kx` in its `formula.yml` audit, install and test steps.
4. **Turn on GitHub Pages** (Settings → Pages → Source: GitHub Actions). The
   site at https://jzills.github.io/snoop/ serves `install.sh`, so this has to
   happen before the README's install command works. `.github/workflows/pages.yml`
   publishes `site/` on every push to `main` that touches it; run it once by
   hand (Actions → Pages → Run workflow) after turning Pages on.
5. **Add the `HOMEBREW_TAP_TOKEN` secret** to this repository: a fine-grained
   personal access token with *Contents: read and write* on
   `jzills/homebrew-tap` only. Without it the release still publishes and the
   tap step is skipped with a warning.

## Each release

1. Make sure `main` is green in CI.
2. Optionally write `release-notes/vX.Y.Z.md`: a paragraph that opens the
   release notes, ahead of GitHub's generated list of changes. Commit it.
3. Tag and push:

   ```
   git tag v0.1.0
   git push origin v0.1.0
   ```

4. Approve the run when it reaches the `release` environment. It then:
   - creates the GitHub Release with six archives (Linux, macOS and Windows,
     amd64 and arm64), version-free copies for `releases/latest/download/`,
     and `SHA256SUMS`;
   - commits `Formula/snoop.rb` to the tap.
5. Check the install paths:

   ```
   brew install jzills/tap/snoop && snoop --version
   curl -fsSL https://jzills.github.io/snoop/install.sh | sh
   go install github.com/jzills/snoop/cmd/snoop@v0.1.0
   ```

A run stopped at the approval gate has published nothing. Delete the tag
(`git push --delete origin vX.Y.Z`) to abandon it.

## Building locally

```
scripts/build_binaries.sh 0.1.0      # dist/: binaries and archives
scripts/check_archives.sh 0.1.0      # what CI and the release check
scripts/homebrew_formula.sh 0.1.0    # the formula the release would commit
```

## Previewing the site

The site is Hugo with the Hextra theme, in `site/` as its own Go module. With
[Hugo extended](https://gohugo.io/installation/) 0.164.0:

```
cd site && hugo server    # http://localhost:1313/snoop/
```

## Re-rendering the site's screenshots

`demo/shots.sh` renders `site/static/shots/*.png` the same way, from
`demo/shots.tape` and `demo/live.tape`. Run it after a change to how snoop
looks, so the docs show the current build.

## Re-rendering the demo

`demo/render.sh` builds snoop, generates fictional sessions with
`demo/seed.py`, and records `demo/demo.tape` to `demo/demo.gif` with
[vhs](https://github.com/charmbracelet/vhs). It never touches your own
transcripts or summary cache.
