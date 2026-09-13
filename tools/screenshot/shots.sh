#!/usr/bin/env bash
# Builds goradion and captures the standard screens with a fake mpv, at
# 80x24 and 120x40 and in ASCII mode.
#
# Usage: tools/screenshot/shots.sh [output-dir]    (default: ./screenshots)
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../.." && pwd)"
out="$(mkdir -p "${1:-screenshots}" && cd "${1:-screenshots}" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

go build -C "$repo" -o "$work/bin/goradion" .
export PATH="$here/bin:$work/bin:$PATH"
export FAKEMPV_SONG="${FAKEMPV_SONG:-3}"

# run COLS ROWS SCENARIO [GORADION FLAGS...]
run() {
	local cols=$1 rows=$2 scenario=$3
	shift 3
	rm -rf "$work/home" && mkdir -p "$work/home"
	(cd "$work" && python3 "$here/shoot.py" --cols "$cols" --rows "$rows" --out "$out" \
		"$here/$scenario" -- env HOME="$work/home" LANG=C.UTF-8 goradion "$@")
}

run 80 24 narrow.json
run 120 40 wide.json
run 80 24 ascii.json -ascii
run 80 24 spectrum.json
FAKEMPV_SPECTRUM=0 run 80 24 fallback.json
FAKEMPV_SPECTRUM=late run 80 24 watchdog.json
echo "Screenshots are in $out"
