#!/usr/bin/env bash
# Install the npm packages and the Linux wheel the way users will, into
# throwaway locations, and check that each installs a snoop reporting the
# release's version. Runs on Linux x64, before anything is published.
#
#   scripts/check_packages.sh <version> [dist]
set -euo pipefail

version="${1:?usage: check_packages.sh <version> [dist]}"
version="${version#v}"
dist="$(cd "${2:-dist}" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

expect() {
  local what="$1" out
  shift
  out="$("$@" --version | head -1)"
  if [ "$out" != "snoop v$version" ]; then
    echo "$what: snoop --version printed '$out', want 'snoop v$version'" >&2
    exit 1
  fi
  echo "$what: $out"
}

# npm: install the main package with the Linux x64 one beside it, from the
# tarballs the release publishes. --omit=optional and --offline keep npm from
# looking for the other platforms' packages, which aren't published yet.
packs="$dist/npm-packs"
test "$(find "$packs" -name '*.tgz' | wc -l)" -eq 7 || { echo "expected 7 npm packages" >&2; exit 1; }
for f in "$packs"/jzills-snoop-cli-*.tgz; do
  listing="$(tar -tvzf "$f")"
  grep -q '^-rwx.* package/bin/snoop' <<<"$listing" || { echo "$f: the binary isn't executable" >&2; exit 1; }
done
npm install --global --prefix "$tmp/npm" --offline --omit=optional --no-audit --no-fund \
  "$packs/snoop-cli-$version.tgz" "$packs/jzills-snoop-cli-linux-x64-$version.tgz" >/dev/null
expect npm "$tmp/npm/bin/snoop"

# PyPI: one wheel per target, and the Linux x64 one installs into a fresh
# virtual environment without reaching the index.
test "$(find "$dist/wheels" -name '*.whl' | wc -l)" -eq 6 || { echo "expected 6 wheels" >&2; exit 1; }
python3 -m venv "$tmp/venv"
"$tmp/venv/bin/pip" install --quiet --no-index "$dist"/wheels/*-manylinux_2_17_x86_64.*.whl
expect pypi "$tmp/venv/bin/snoop"
