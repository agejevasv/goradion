# goradion
Goradion is a command line radio player based on `mpv`. Goradion is inspired by [pyradio](https://github.com/coderholic/pyradio) and supports it's stations playlists.

<img width="725" alt="goradion" src="https://github.com/agejevasv/goradion/assets/1682086/4d0aa823-8662-42f8-a6ad-c601486fcf6d">

## Setup

1. Prerequisites: [mpv](https://mpv.io/)
   ```bash
   # Mac
   brew install mpv

   # Ubuntu
   apt install mpv

   # Arch
   pacman -S mpv

   # Windows
   Download: https://sourceforge.net/projects/mpv-player-windows/files/latest/download
   Unpack e.g. into c:\mpv
   Add this dir to the PATH, either via GUI or: `setx /M PATH "%PATH%;c:\mpv"`

   # Other OSes:
   Install mpv using your package manager or refer to https://mpv.io/installation/
   ```
3. [Download goradion](https://github.com/agejevasv/goradion/releases/latest)
4. Mark it as executable:
   ```bash
   chmod +x goradion-<version>
   ```

## Run
```bash
# Starts with preset radio stations
goradion-<version>
```

For your own stations you can create a public [gist](https://gist.github.com/) file and pass it with `-s` flag, e.g.:

```bash
goradion -s https://gist.githubusercontent.com/agejevasv/58afa748a7bc14dcccab1ca237d14a0b/raw/stations.csv
```

You can also create this file locally if you prefer:

```bash
goradion -s /path/to/stations.csv
```
## Keyboard Control
```
Keyboard Control

  a-z
    Toggle playing a station marked with a given letter.

  Enter and Space
    Toggle playing currently selected station.

  Left and Right, - and +
    Change the volume in increments of 5.

  Up and Down
    Cycle through the radio station list.

  PgUp and PgDown
    Jump to a beginning/end of a station list.

  Esc
    Close current window.

  ?
    Toggle help.
```
