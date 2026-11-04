#!/usr/bin/env bash
# Print the README for a package page (npm, PyPI), with its relative links
# made absolute: those pages don't resolve paths inside the repository.
#
#   scripts/package_readme.sh
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
raw="https://raw.githubusercontent.com/jzills/snoop/main"
blob="https://github.com/jzills/snoop/blob/main"

sed -E \
  -e "s#(src|srcset)=\"(site|demo)/#\\1=\"$raw/\\2/#g" \
  -e "s#\\]\\((site|demo)/#]($raw/\\1/#g" \
  -e "s#\\]\\(LICENSE\\)#]($blob/LICENSE)#g" \
  "$root/README.md"
