# Screenshots without a terminal

These tools run goradion in a pseudo-terminal, emulate the screen and save
PNG files, so UI changes can be reviewed without a desktop, a sound card or
network access.

- `bin/mpv` is a stand-in for mpv. It answers goradion's IPC commands,
  pretends to buffer and play, sends track titles and a moving audio level.
  Its environment knobs are listed at the top of the file.
- `shoot.py` drives the program with a scenario of key presses, mouse
  events and resizes, and saves screenshots. The scenario format is
  described at the top of the file.
- `shots.sh` builds goradion and runs the three scenarios in this folder.

## Requirements

Go, Python 3 and the DejaVu fonts, plus these Python packages:

```bash
pip install pyte pillow fonttools
```

`fonttools` is optional; without it, symbols missing from the monospace
font are drawn as boxes.

## Run

```bash
tools/screenshot/shots.sh            # writes ./screenshots
tools/screenshot/shots.sh /tmp/shots
```

Colours come from a fixed dark palette, not from your terminal theme.
