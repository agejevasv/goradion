# goradion
Terminal based online radio player

It uses and is compatible with the stations CSV file used by [pyradio](https://github.com/coderholic/pyradio).

## Setup

1. Install [mpv](https://mpv.io/) and make sure it is available in path.
2. Download a latest version of [goradion](https://github.com/agejevasv/goradion/releases/latest) for your architecture.
3. `chmod +x <downloaded-goradion-binary>`

If you have your own or any [pyradio](https://github.com/coderholic/pyradio) stations CSV, you can pass it with a `-s <url or file>` flag, e.g. on my mac:

```bash
goradion-darwin-arm64 -s https://raw.githubusercontent.com/coderholic/pyradio/master/pyradio/stations.csv
```
