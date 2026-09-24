# Rust rewrite

Goal: one native binary, no mpv. The Go app stays the release until every box
below is ticked; then `rust/` replaces it in one commit.

A box is ticked when the behaviour works and its Go tests are ported.

## Stack

- Audio: symphonia (decode), cpal (output), rubato (resampling), biquad bands like FFmpeg's `bandpass` (spectrum)
- Streams: own HTTP/1.1 client on rustls, because Shoutcast v1 answers `ICY 200 OK`
- UI: ratatui + crossterm
- Other: ureq (radio-browser, stations CSV), tiny_http (remote), qrcode

## Audio engine (`rust/src/audio`)

- [x] HTTP(S) stream: redirects, timeouts, `ICY 200 OK`
- [x] Playlists: `.pls`, `.m3u` (by content type or extension)
- [x] ICY metadata: strip blocks, `StreamTitle` → song
- [ ] Ogg/FLAC/Vorbis comments → `Artist - Title` (coded, no title seen yet on the two FLAC stations)
- [x] Decode MP3, AAC-LC, FLAC, Vorbis
- [ ] Decode Opus (libopus, static)
- [x] HE-AAC plays (core only until SBR exists)
- [ ] Output via cpal, resampled to the device rate (works on ALSA null device; needs a real sound card test)
- [ ] States: idle, buffering, playing, stopped, failed (retry with backoff 1s…30s), unsupported (no retry), no device
- [ ] Volume 0–100 (mpv's cubic curve), fades
- [x] Bitrate
- [ ] Song history (20)
- [x] Level meter (RMS, -32 dBFS floor)
- [x] Spectrum: 12 bands, 40 Hz – 12 kHz, tilt and floor as in Go
- [ ] Recover from device loss (Bluetooth headphones off)
- [x] `-c`: play a few seconds of every station through the engine, report codec and failures
- [ ] Legacy TLS servers (HiOnLine Classic fails the rustls handshake; mpv plays it)

## App

- [x] Stations CSV: built-in, file, URL (`-s`)
- [ ] Tags list with counts, bookmarks row, last search row
- [ ] Station list, cursor bar, playing and bookmark marks
- [ ] Type-to-filter replaces a–z/A–Z shortcuts; `j`/`k`
- [ ] `1`–`9` play the first nine bookmarks
- [ ] Wide layout (tags and stations side by side), `Tab`
- [ ] Now playing card: station, song, previous song, status, spinner, live glow
- [ ] Gauges: volume, level meter, spectrum; mouse on volume gauge
- [ ] Mouse: click to play, wheel scrolls
- [ ] Random station (`*`), shuffle (`Ctrl+R`, `Alt+1`–`9`)
- [ ] Sleep timer (`Ctrl+Z`), fade out, click countdown to cancel
- [ ] Bookmarks (`Ctrl+B`), saved to file
- [ ] Search own stations and radio-browser (`:`, `Ctrl+F`, `Ctrl+S`)
- [ ] Help (`?`), hint bar
- [ ] Themes (`Ctrl+T`), terminal background via OSC 11
- [ ] `--ascii` glyphs, locale detection
- [ ] `--no-vu`
- [ ] Config YAML: defaults, save `volume`/`tag`/`station`/`theme` keeping layout and comments
- [ ] Session restore, `autoplay`
- [ ] Remote: web server, access code, QR modal (`Ctrl+P`), `-r [key]`, `-p`, same API and page
- [x] Debug log (`-d`), version (`-v`)

## Release

- [ ] Linux, macOS, Windows builds in CI
- [ ] README without mpv
- [ ] AUR package
