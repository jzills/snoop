#!/usr/bin/env bash
# Render the site's screenshots (site/static/shots/*.png) from
# demo/shots.tape, demo/live.tape, demo/ask.tape and demo/diff.tape, the way demo/render.sh renders the GIF: a temporary build,
# fictional sessions and a temporary cache, never your own sessions.
#
#   demo/shots.sh
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

tmp="$(mktemp -d)"
# Stands in for the permission hook, which snoop checks is running.
sleep 600 &
hook=$!
trap 'kill "$hook" 2>/dev/null; rm -rf "$tmp"' EXIT

go build -o "$tmp/bin/snoop" ./cmd/snoop
python3 demo/seed.py "$tmp/claude"
mkdir -p site/static/shots

export PATH="$tmp/bin:$PATH"
export CLAUDE_CONFIG_DIR="$tmp/claude"
export XDG_CACHE_HOME="$tmp/cache"
export XDG_STATE_HOME="$tmp/state"
export PS1='$ '

vhs demo/shots.tape -o "$tmp/shots.gif"
# The live sessions are seeded last, so they are seconds old when shot.
python3 demo/seed.py "$tmp/claude" --live-only
vhs demo/live.tape -o "$tmp/live.gif"
# Then the waiting session asks for permission.
python3 demo/seed.py "$tmp/claude" --ask "$tmp/state" "$hook"
vhs demo/ask.tape -o "$tmp/ask.gif"
# The Diff tab needs a real repository, with the session that changed it.
python3 demo/seed.py "$tmp/claude" --diff "$tmp/src"
vhs demo/diff.tape -o "$tmp/diff.gif"
echo "wrote site/static/shots/"
