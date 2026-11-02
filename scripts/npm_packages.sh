#!/usr/bin/env bash
# Write snoop's npm packages from the binaries scripts/build_binaries.sh
# built: snoop-cli, whose `snoop` command runs packaging/npm/snoop.js, and one
# @jzills/snoop-cli-<platform>-<arch> package per target holding just its
# binary. snoop-cli lists them as optional dependencies, and their os and cpu
# fields make npm install only the one for the machine.
#
#   scripts/npm_packages.sh <version> [dist]
#
# Writes dist/npm/snoop-cli/ and dist/npm/snoop-cli-<platform>-<arch>/, and
# packs each into dist/npm-packs/*.tgz. The release publishes the tarballs,
# not the folders: they keep the binaries' executable bit, which a workflow
# artifact drops.
set -euo pipefail

version="${1:?usage: npm_packages.sh <version> [dist]}"
version="${version#v}"
dist="${2:-dist}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out="$dist/npm"
repo='{"type": "git", "url": "git+https://github.com/jzills/snoop.git"}'
homepage="https://jzills.github.io/snoop/"

rm -rf "$out"
mkdir -p "$out"

# Go's names for each target, and Node's (process.platform, process.arch).
targets=(
  "linux amd64 linux x64"
  "linux arm64 linux arm64"
  "darwin amd64 darwin x64"
  "darwin arm64 darwin arm64"
  "windows amd64 win32 x64"
  "windows arm64 win32 arm64"
)

optional=""
for target in "${targets[@]}"; do
  read -r goos goarch platform cpu <<<"$target"
  name="@jzills/snoop-cli-${platform}-${cpu}"
  dir="$out/snoop-cli-${platform}-${cpu}"
  exe="snoop"
  if [ "$goos" = windows ]; then
    exe="snoop.exe"
  fi
  bin="$dist/binaries/snoop_${goos}_${goarch}/$exe"
  test -f "$bin" || { echo "missing $bin" >&2; exit 1; }
  mkdir -p "$dir/bin"
  cp "$bin" "$dir/bin/$exe"
  chmod 755 "$dir/bin/$exe"
  cp "$root/LICENSE" "$dir/"
  cat >"$dir/package.json" <<JSON
{
  "name": "$name",
  "version": "$version",
  "description": "The snoop binary for ${platform} on ${cpu}. Install snoop-cli instead.",
  "license": "MIT",
  "repository": $repo,
  "homepage": "$homepage",
  "os": ["$platform"],
  "cpu": ["$cpu"],
  "files": ["bin"],
  "preferUnplugged": true
}
JSON
  printf '%s\n' "snoop's binary for ${platform}/${cpu}. Install [snoop-cli](https://www.npmjs.com/package/snoop-cli), which picks the right one." >"$dir/README.md"
  optional+="${optional:+,
}    \"$name\": \"$version\""
done

main="$out/snoop-cli"
mkdir -p "$main/bin"
cp "$root/packaging/npm/snoop.js" "$main/bin/snoop.js"
chmod 755 "$main/bin/snoop.js"
cp "$root/LICENSE" "$main/"
"$root/scripts/package_readme.sh" >"$main/README.md"
cat >"$main/package.json" <<JSON
{
  "name": "snoop-cli",
  "version": "$version",
  "description": "A terminal viewer for Claude Code sessions: follow one live, or see what any past one cost, changed and spent its time on.",
  "license": "MIT",
  "repository": $repo,
  "homepage": "$homepage",
  "keywords": ["claude", "claude-code", "agents", "tui", "terminal"],
  "bin": {"snoop": "bin/snoop.js"},
  "files": ["bin"],
  "engines": {"node": ">=18"},
  "optionalDependencies": {
$optional
  }
}
JSON

packs="$dist/npm-packs"
rm -rf "$packs"
mkdir -p "$packs"
packs="$(cd "$packs" && pwd)"
for dir in "$out"/*/; do
  (cd "$dir" && npm pack --silent --pack-destination "$packs" >/dev/null)
done

echo "npm packages in $packs:"
ls -1 "$packs"
