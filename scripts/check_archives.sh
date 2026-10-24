#!/usr/bin/env bash
# Check the archives scripts/build_binaries.sh produced, as CI and the release
# workflow do before anything is published:
#   - every archive holds snoop/snoop (snoop.exe on Windows) and the LICENSE
#   - each version-free copy is byte-identical to its versioned archive
#   - the native binary runs and reports the expected version
#
#   scripts/check_archives.sh <version> [dist]
set -euo pipefail

version="${1:?usage: check_archives.sh <version> [dist]}"
version="${version#v}"
dist="${2:-dist}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

count=0
for archive in "$dist"/snoop_v"${version}"_*.tar.gz "$dist"/snoop_v"${version}"_*.zip; do
  test -f "$archive" || { echo "no archives for v${version} in $dist"; exit 1; }
  name="$(basename "$archive")"
  extract="$tmp/${name%%.*}"
  mkdir -p "$extract"
  case "$archive" in
    *.zip)
      python3 -m zipfile -e "$archive" "$extract"
      bin="$extract/snoop/snoop.exe" ;;
    *)
      tar -C "$extract" -xzf "$archive"
      bin="$extract/snoop/snoop" ;;
  esac
  test -f "$bin" || { echo "$archive has no $(basename "$bin")"; exit 1; }
  test -f "$extract/snoop/LICENSE" || { echo "$archive has no LICENSE"; exit 1; }
  copy="$dist/snoop_${name#snoop_v"${version}"_}"
  cmp -s "$archive" "$copy" || { echo "$copy is missing or differs from $archive"; exit 1; }
  count=$((count + 1))
done
test "$count" -eq 6 || { echo "expected 6 versioned archives, found $count"; exit 1; }

# First line only: the second reports commit, date and toolchain.
actual="$("$dist/binaries/snoop_linux_amd64/snoop" --version | head -1)"
expected="snoop v${version}"
test "$actual" = "$expected" || { echo "expected '$expected', got '$actual'"; exit 1; }
echo "ok: $count archives, $actual"
