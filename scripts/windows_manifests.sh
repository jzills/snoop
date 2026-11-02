#!/usr/bin/env bash
# Write the Scoop and winget manifests for a release, from the Windows zips
# scripts/build_binaries.sh produced:
#
#   dist/scoop/snoop.json   committed to jzills/scoop-bucket as bucket/snoop.json
#   dist/winget/*.yaml      submitted to microsoft/winget-pkgs as jzills.snoop
#
#   scripts/windows_manifests.sh <version> [dist]
set -euo pipefail

version="${1:?usage: windows_manifests.sh <version> [dist]}"
version="${version#v}"
dist="${2:-dist}"
repo="https://github.com/jzills/snoop"
description="A terminal viewer for Claude Code sessions: follow one live, or see what any past one cost, changed and spent its time on."

sha() {
  local f="$dist/snoop_v${version}_windows_$1.zip"
  test -f "$f" || { echo "missing $f" >&2; exit 1; }
  sha256sum "$f" | cut -d' ' -f1
}
url() { echo "$repo/releases/download/v${version}/snoop_v${version}_windows_$1.zip"; }

# Computed up front, so a missing zip stops the script before a manifest
# goes out with an empty hash.
sha_amd64="$(sha amd64)"
sha_arm64="$(sha arm64)"

mkdir -p "$dist/scoop"
cat >"$dist/scoop/snoop.json" <<JSON
{
    "version": "$version",
    "description": "$description",
    "homepage": "https://jzills.github.io/snoop/",
    "license": "MIT",
    "architecture": {
        "64bit": {
            "url": "$(url amd64)",
            "hash": "$sha_amd64"
        },
        "arm64": {
            "url": "$(url arm64)",
            "hash": "$sha_arm64"
        }
    },
    "extract_dir": "snoop",
    "bin": "snoop.exe",
    "checkver": "github",
    "autoupdate": {
        "architecture": {
            "64bit": {
                "url": "$repo/releases/download/v\$version/snoop_v\$version_windows_amd64.zip"
            },
            "arm64": {
                "url": "$repo/releases/download/v\$version/snoop_v\$version_windows_arm64.zip"
            }
        },
        "hash": {
            "url": "\$baseurl/SHA256SUMS"
        }
    }
}
JSON

id="jzills.snoop"
manifest_version="1.6.0"
w="$dist/winget"
rm -rf "$w"
mkdir -p "$w"
upper() { tr '[:lower:]' '[:upper:]' <<<"$1"; }

cat >"$w/$id.yaml" <<YAML
PackageIdentifier: $id
PackageVersion: $version
DefaultLocale: en-US
ManifestType: version
ManifestVersion: $manifest_version
YAML

cat >"$w/$id.installer.yaml" <<YAML
PackageIdentifier: $id
PackageVersion: $version
InstallerType: zip
NestedInstallerType: portable
NestedInstallerFiles:
  - RelativeFilePath: snoop\\snoop.exe
    PortableCommandAlias: snoop
Installers:
  - Architecture: x64
    InstallerUrl: $(url amd64)
    InstallerSha256: $(upper "$sha_amd64")
  - Architecture: arm64
    InstallerUrl: $(url arm64)
    InstallerSha256: $(upper "$sha_arm64")
ManifestType: installer
ManifestVersion: $manifest_version
YAML

cat >"$w/$id.locale.en-US.yaml" <<YAML
PackageIdentifier: $id
PackageVersion: $version
PackageLocale: en-US
Publisher: jzills
PublisherUrl: https://github.com/jzills
PackageName: snoop
PackageUrl: https://jzills.github.io/snoop/
License: MIT
LicenseUrl: $repo/blob/main/LICENSE
ShortDescription: $description
Moniker: snoop
Tags:
  - claude
  - claude-code
  - agents
  - tui
ReleaseNotesUrl: $repo/releases/tag/v$version
ManifestType: defaultLocale
ManifestVersion: $manifest_version
YAML

echo "wrote $dist/scoop/snoop.json and $w/"
