# goradion
Terminal based online radio player

<img width="621" alt="goradion" src="https://github.com/agejevasv/goradion/assets/1682086/5d11d334-39f2-4c76-a12c-59cb14df80c2">

Goradion is compatible with [pyradio](https://github.com/coderholic/pyradio) stations playlists.

The stations can be toggled with Enter/Space keys or by using letter keys on a keyboard (a letter next to a radio station). The stations can also be toggled by a mouse click.

## Setup

1. Install [mpv](https://mpv.io/) and make sure it is available in path.
2. Download a latest version of [goradion](https://github.com/agejevasv/goradion/releases/latest) for your architecture.
3. `chmod +x <downloaded-goradion-binary>`

If you have your own or any [pyradio](https://github.com/coderholic/pyradio) stations CSV, you can pass it with a `-s <url or file>` flag, e.g. on my mac:

```bash
goradion-darwin-arm64 -s https://raw.githubusercontent.com/coderholic/pyradio/master/pyradio/stations.csv
```
