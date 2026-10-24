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
3. **Create the tap repository** `jzills/homebrew-tap` (public, empty apart
   from a README). Homebrew finds it as `jzills/tap`.
4. **Add the `HOMEBREW_TAP_TOKEN` secret** to this repository: a fine-grained
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

## Re-rendering the demo

`demo/render.sh` builds snoop, generates fictional sessions with
`demo/seed.py`, and records `demo/demo.tape` to `demo/demo.gif` with
[vhs](https://github.com/charmbracelet/vhs). It never touches your own
transcripts or summary cache.
