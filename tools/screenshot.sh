#!/usr/bin/env bash
# Updates docs/screenshot.png: the github-dark theme, playing SomaFM: DEF CON
# Radio from the Electronic tag. Needs tmux, freeze and the network.
set -euo pipefail

cd "$(dirname "$0")/.."
home=$(mktemp -d)
session=goradion-screenshot
trap 'tmux kill-session -t $session 2>/dev/null; rm -rf "$home"' EXIT

mkdir -p "$home/.config/goradion"
cat > "$home/.config/goradion/config.yaml" <<'EOF'
theme: github-dark
autoplay: true
volume: 80
tag: Electronic
station: https://somafm.com/defcon256.pls
EOF

tmux new-session -d -s $session -x 160 -y 42 "HOME=$home exec target/release/goradion"
# Time for the stream to buffer and send the song title.
sleep 10
tmux capture-pane -p -e -t $session |
    freeze --language ansi --background '#0d1117' --output docs/screenshot.png
