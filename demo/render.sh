#!/usr/bin/env bash
# Render demo/demo.gif from demo/demo.tape against fictional sessions.
#
# Builds snoop into a temporary bin, seeds demo transcripts with
# demo/seed.py, and points CLAUDE_CONFIG_DIR at them so no real session is
# ever recorded. XDG_CACHE_HOME is temporary too, so the summary cache of your
# real sessions is left alone.
#
# Prereqs: vhs (https://github.com/charmbracelet/vhs) with ttyd and ffmpeg,
# python3, and the "Ubuntu Mono" font (or edit FontFamily in the tape).
#
#   demo/render.sh
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

go build -o "$tmp/bin/snoop" ./cmd/snoop
python3 demo/seed.py "$tmp/claude"

export PATH="$tmp/bin:$PATH"
export CLAUDE_CONFIG_DIR="$tmp/claude"
export XDG_CACHE_HOME="$tmp/cache"
export PS1='$ '

vhs demo/demo.tape
echo "wrote demo/demo.gif"
