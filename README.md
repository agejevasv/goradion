# goradion
Goradion is a command line radio player based on `mpv`. Goradion is inspired by [pyradio](https://github.com/coderholic/pyradio) and supports it's stations playlists.
<p align="center">
  <img width="777" alt="goradion" src="https://github.com/agejevasv/goradion/assets/1682086/dff2d402-76dc-4212-a1ef-86e2fad2ff73">
</p>

## Setup

1. Prerequisites: [mpv](https://mpv.io/)
    - Mac
      - `brew install mpv`
    
    - Ubuntu
      - `apt install mpv`
     
    - Arch Linux
      - `pacman -S mpv`
        
    - Windows
      - Download: [mpv](https://sourceforge.net/projects/mpv-player-windows/files/)
      - Unpack e.g. into c:\mpv
      - Add this dir to the PATH, either via GUI or: `setx /M PATH "%PATH%;c:\mpv"`
         
    - Other OSes
      - Install mpv using your package manager or refer to https://mpv.io/installation/

2. [Download goradion](https://github.com/agejevasv/goradion/releases/latest)
3. Mark it as executable (not needed on Windows):
```bash
chmod +x goradion-<version>
```
**Warning**: _[On some Windows machines](https://github.com/agejevasv/goradion/issues/1), a virus scanner identifies the binary as infected (https://go.dev/doc/faq#virus), in this case it's best to build the binary yourself: `go build .`._

## Run
On Windows just double click the downloaded exe (or run via cmd to use flags), on other OSes:
```bash
# Starts with preset radio stations
goradion-<version>
```

## Stations
For your own stations you can create a public [gist](https://gist.github.com/) file and pass a link with a raw version of it with `-s` flag, e.g.:

```bash
goradion -s https://gist.githubusercontent.com/agejevasv/58afa748a7bc14dcccab1ca237d14a0b/raw/stations.csv
```

You can also create this file locally if you prefer. You can start with downloading and then editing [default stations](https://gist.githubusercontent.com/agejevasv/58afa748a7bc14dcccab1ca237d14a0b/raw/stations.csv):

```bash
goradion -s /path/to/stations.csv
```
## Keyboard Control
```
Keyboard Control

  *
    Toggle playing a random station.

	/
		Show tag selection screen.

	#
		Clear selected tag.

  a-z and A-Z
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
    Show help screen.
```
