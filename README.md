# goradion
Goradion is a command line radio player based on `mpv`.
<p align="center">
  <img alt="goradion" src="https://github.com/user-attachments/assets/1a86f861-10ed-4ad5-b90e-f48c4278c317">
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
The stations are configured using a CSV file with a titile, URL and semicolon `;` separated tag(s), e.g.:

```csv
Title,URL,tag_1[;...;tag_n]
...
```
Stations file can be passed with `-s` argument; goradion supports both local files and HTTP URLs, e.g.:
```bash
goradion -s /path/to/stations.csv

OR

goradion -s https://path-to/stations.csv
```
