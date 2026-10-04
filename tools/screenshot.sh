#!/usr/bin/env bash
# Updates docs/screenshot.png: the github-dark theme, playing SomaFM: DEF CON
# Radio from the Electronic tag, in foot on a headless sway. Needs sway, foot,
# grim, the DejaVu fonts (ttf-dejavu) and the network.
set -euo pipefail

cd "$(dirname "$0")/.."
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

mkdir -p "$work/home/.config/goradion"
cat > "$work/home/.config/goradion/config.yaml" <<'EOF'
theme: github-dark
autoplay: true
volume: 80
tag: Electronic
station: https://somafm.com/defcon256.pls
EOF

cat > "$work/foot.ini" <<'EOF'
font=DejaVu Sans Mono:size=13
pad=12x12
EOF

# The stream gets 10 s to buffer and send the song title; then sway quits.
cat > "$work/sway.conf" <<EOF
output HEADLESS-1 resolution 2880x1696 scale 2
default_border none
exec foot -c $work/foot.ini env HOME=$work/home $PWD/target/release/goradion
exec sleep 10 && grim $PWD/docs/screenshot.png && swaymsg exit
EOF

if [[ -z ${XDG_RUNTIME_DIR:-} ]]; then
    export XDG_RUNTIME_DIR=$work/run
    mkdir -m 700 "$XDG_RUNTIME_DIR"
fi
WLR_BACKENDS=headless WLR_LIBINPUT_NO_DEVICES=1 WLR_RENDERER=pixman sway -c "$work/sway.conf" 2>"$work/sway.log" ||
    { cat "$work/sway.log" >&2; exit 1; }
