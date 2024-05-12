# goradion
Terminal based online radio player

<img width="621" alt="goradion" src="https://github.com/agejevasv/goradion/assets/1682086/5d11d334-39f2-4c76-a12c-59cb14df80c2">

Goradion is compatible with [pyradio](https://github.com/coderholic/pyradio) stations playlists.

The stations can be toggled with Enter/Space keys or by using letter keys on a keyboard (a letter next to a radio station). The stations can also be toggled by a mouse click.

## Setup

1. Install [mpv](https://mpv.io/) and make sure it is available on path.
2. Download the latest version of [goradion](https://github.com/agejevasv/goradion/releases/latest) for your architecture.
3. `chmod +x <downloaded-goradion-binary>`

If you have your own or any [pyradio](https://github.com/coderholic/pyradio) stations CSV, you can pass it with a `-s <url or file>` flag, e.g.:

```bash
goradion -s https://raw.githubusercontent.com/coderholic/pyradio/master/pyradio/stations.csv
```

## Tips

For your own stations you can create a public Gist file and then run `goradion` with it's raw version, e.g.:

```bash
goradion -s https://gist.githubusercontent.com/agejevasv/58afa748a7bc14dcccab1ca237d14a0b/raw/stations.csv
```

Of course you can create this file locally if you prefer.
