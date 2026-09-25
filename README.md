# goradion
Goradion is a TUI online radio player.

Listen to a curated list of stations, or search radio-browser.info for more.

<p align="center">
  <img alt="goradion" src="docs/screenshot.png">
</p>

## Install
[Download goradion](https://github.com/agejevasv/goradion/releases/latest) for Linux, macOS or Windows.

### Build from source
You need [Rust](https://rustup.rs) 1.88 or newer, and on Linux the ALSA headers and pkg-config
(`apt install libasound2-dev pkg-config`, `dnf install alsa-lib-devel`, `pacman -S alsa-lib`):
```bash
cargo install --locked --git https://github.com/agejevasv/goradion
```
Or in a clone, `cargo build --release` leaves the binary in `target/release/`.

## Stations
Your own list, a CSV file or URL given with `-s`, has a station per line:
```csv
Title,URL,tag_1[;...;tag_n]
...
```

## Search
`Ctrl+S` searches radio-browser.info; press it again to search your own stations.

## Bookmarks
`Ctrl+B` bookmarks the station under the cursor, or the one playing; again removes it.

## Remote control
`Ctrl+P` shows a QR code and an access code, to control goradion from a phone on the same network.

## Themes
`Ctrl+T` picks a colour theme, saved in the [config](#config).

## Config
Goradion creates `config.yaml` on the first run, in `~/.config/goradion`
(`~/Library/Application Support/goradion` on macOS, `%APPDATA%\goradion` on Windows):

```yaml
theme: terminal         # Ctrl+T saves it here
autoplay: true          # play the last station at launch; false only selects it

volume: 80              # remembered from the last run
tag: Jazz
station: https://...

remote:                 # the phone remote, Ctrl+P
  autostart: false      # start it with goradion, like -r
  port: 7373            # preferred port, like -p
  key: my secret code   # a fixed access code; "" for none; leave out for a random one per run
```

Flags win over the file. Goradion keeps your edits and comments when it saves.
