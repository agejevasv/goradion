# Rust rewrite

Goal: one native binary, no mpv. The Go app stays the release until every box
below is ticked; then `rust/` replaces it in one commit.

A box is ticked when the behaviour works and its Go tests are ported.

## Stack

- Audio: symphonia (decode), cpal (output), rubato (resampling), biquad bands like FFmpeg's `bandpass` (spectrum)
- Streams: own HTTP/1.1 client on rustls, because Shoutcast v1 answers `ICY 200 OK`
- UI: ratatui + crossterm
- Other: the same HTTP client for radio-browser and the stations CSV, tiny_http (remote), qrcode

## Audio engine (`rust/src/audio`)

- [x] HTTP(S) stream: redirects, timeouts, `ICY 200 OK`
- [x] Playlists: `.pls`, `.m3u` (by content type or extension)
- [x] ICY metadata: strip blocks, `StreamTitle` → song
- [x] Ogg/FLAC/Vorbis/Opus comments → `Artist - Title` (the two FLAC stations send none)
- [x] Decode MP3, AAC-LC, FLAC, Vorbis
- [x] Decode Opus (opus-rs, pure Rust: 85 dB SNR against libopus, ~0.3% CPU)
- [x] HE-AAC plays (core only)
- [ ] HE-AAC with SBR: syom 0.6 decodes it but refuses the rate change when joining a stream midway
- [x] Output via cpal, resampled to the device rate (PulseAudio null sink: real time, ~2% CPU)
- [x] States: idle, buffering, playing, stopped, failed (retry with backoff 1s…30s), unsupported (no retry)
- [x] Volume 0–100 (mpv's cubic curve), fades
- [x] Station switch: the old station plays on until the new one has audio, then a 1 s crossfade; shuffle uses it
- [x] Bitrate
- [x] Song history (20)
- [x] Level meter (RMS, -32 dBFS floor)
- [x] Spectrum: 12 bands, 40 Hz – 12 kHz, tilt and floor as in Go
- [x] Recover from device loss (tried by restarting PulseAudio mid-stream); a device at another rate reconnects the station
- [x] `-c`: play a few seconds of every station through the engine, report codec and failures
- [ ] Legacy TLS servers (RSA key exchange only, which rustls refuses): none left in `rust/stations.csv`

## App

- [x] Stations CSV: built-in, file, URL (`-s`)
- [x] Tags list with counts, bookmarks row
- [x] Last search row
- [x] Station list, cursor bar, playing and bookmark marks
- [x] Type-to-filter replaces a–z/A–Z shortcuts (no `j`/`k`: letters filter)
- [x] `1`–`9` play the first nine bookmarks
- [x] Wide layout (tags and stations side by side), `Tab`
- [x] Now playing card: station, song, previous song, status, spinner, live glow
- [x] Gauges: volume, spectrum
- [x] Mouse on the volume gauge
- [x] Mouse: click to play, wheel scrolls
- [x] Random station (`*`), shuffle (`Ctrl+R`, `Alt+1`–`9`)
- [x] Sleep timer (`Ctrl+Z`), fade out, click countdown to cancel
- [x] Bookmarks (`Ctrl+B`), saved to file
- [x] Search own stations and radio-browser (`:`, `Ctrl+F`, `Ctrl+S`)
- [x] Help (`?`), hint bar
- [x] Themes (`Ctrl+T`), terminal background via OSC 11
- [x] `--ascii` glyphs, locale detection
- [x] `--no-vu`
- [x] Config YAML: defaults, save `volume`/`tag`/`station`/`theme` keeping layout and comments
- [x] Session restore, `autoplay`
- [x] Remote: web server, access code, QR modal (`Ctrl+P`), `-r [key]`, `-p`, same API and page
- [x] Debug log (`-d`), version (`-v`)

## Release

- [x] `just check` in `rust/`: rustfmt, clippy with pedantic lints and no `unsafe`, tests, docs, cargo-deny (RustSec advisories, licences, sources), unused dependencies, minimum Rust 1.88; CI runs it too, and weekly
- [ ] Linux, macOS, Windows builds in CI, and a pre-release for each `rust-v*` tag (`.github/workflows/rust.yml`, not run yet)
- [ ] README without mpv
- [ ] AUR package
