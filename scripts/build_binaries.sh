#!/usr/bin/env bash
# Cross-compile snoop for every published target and package the release
# archives. Adapted from kx's build script: a CGO_ENABLED=0 Go binary needs no
# cross toolchain and links against no libc, so one machine builds them all.
#
#   scripts/build_binaries.sh <version> [outdir]
#
# Produces, under outdir (default dist/):
#   binaries/snoop_<os>_<arch>/snoop[.exe]  the executables
#   snoop_v<version>_<os>_<arch>.tar.gz     release archives (linux, darwin)
#   snoop_v<version>_windows_<arch>.zip     release archives (windows)
#   snoop_<os>_<arch>.tar.gz, snoop_windows_<arch>.zip
#                                           the same archives without the version
#
# The version-free copies exist for one URL: GitHub's
# releases/latest/download/<asset> only resolves a name that is the same in
# every release. They are byte-identical copies, so SHA256SUMS lists each pair
# with the same hash.
#
# Each archive holds snoop/snoop (snoop.exe on Windows), LICENSE and README.md.
set -euo pipefail

version="${1:?usage: build_binaries.sh <version> [outdir]}"
version="${version#v}"
outdir="${2:-dist}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Reported by `snoop --version`. Best-effort: a source tarball with no git
# history still builds, it just has less to say about itself.
commit="$(git -C "$root" rev-parse --short HEAD 2>/dev/null || true)"
date="$(date -u +%Y-%m-%d)"

targets=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
  "windows amd64"
  "windows arm64"
)

mkdir -p "$outdir"
# Resolved once: the zip step runs from inside the staging directory, where a
# relative outdir no longer points here.
outdir="$(cd "$outdir" && pwd)"
binaries="$outdir/binaries"
rm -rf "$binaries" "$outdir"/snoop_*.tar.gz "$outdir"/snoop_*.zip
mkdir -p "$binaries"

for target in "${targets[@]}"; do
  read -r os arch <<<"$target"
  stage="$binaries/snoop_${os}_${arch}"
  mkdir -p "$stage"

  # Windows will not execute a file without the extension.
  exe=""
  if [ "$os" = "windows" ]; then
    exe=".exe"
  fi

  echo "building ${os}/${arch}"
  # -s -w strip the symbol and DWARF tables. -trimpath also drops the VCS
  # stamping, so commit and date are passed explicitly for --version.
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -C "$root" -trimpath -ldflags "-s -w -X main.version=${version} \
      -X main.commit=${commit} -X main.date=${date}" \
    -o "$stage/snoop${exe}" ./cmd/snoop

  archive_root="$outdir/.stage_${os}_${arch}"
  rm -rf "$archive_root"
  mkdir -p "$archive_root/snoop"
  cp "$stage/snoop${exe}" "$archive_root/snoop/snoop${exe}"
  cp "$root/LICENSE" "$root/README.md" "$archive_root/snoop/"
  name="snoop_v${version}_${os}_${arch}"
  if [ "$os" = "windows" ]; then
    # python3's zipfile rather than zip(1), which is not installed everywhere.
    (cd "$archive_root" && python3 -m zipfile -c "$outdir/${name}.zip" snoop)
    cp "$outdir/${name}.zip" "$outdir/snoop_${os}_${arch}.zip"
  else
    tar -C "$archive_root" -czf "$outdir/${name}.tar.gz" snoop
    cp "$outdir/${name}.tar.gz" "$outdir/snoop_${os}_${arch}.tar.gz"
  fi
  rm -rf "$archive_root"
done

echo
echo "archives:"
ls -1 "$outdir"/snoop_*.tar.gz "$outdir"/snoop_*.zip
