#!/usr/bin/env python3
"""Drive a terminal program in a pseudo-terminal and save PNG screenshots.

Usage: shoot.py [--cols N] [--rows N] SCRIPT.json -- COMMAND [ARGS...]

The script is a JSON list of steps, run in order:

  {"keys": "a<down><enter>"}   type keys; <esc> <enter> <tab> <s-tab> <space>
                               <up> <down> <left> <right> <pgup> <pgdn>
                               <c-x> for Ctrl+X, <m-x> for Alt+X
  {"mouse": [x, y, "click"]}   click, wheelup or wheeldown at 0-based cell x, y
  {"resize": [cols, rows]}     resize the terminal
  {"wait": 1.5}                seconds to let the program react (default 0.6)
  {"shot": "name.png"}         save a screenshot (relative to --out)

Requires pyte and Pillow; fontTools is optional and improves glyph fallback.
"""
import argparse, fcntl, json, os, pty, re, select, signal, struct, sys, termios, time

import pyte
from PIL import Image, ImageDraw, ImageFont

FONT_DIRS = ["/usr/share/fonts/truetype/dejavu", "/usr/share/fonts/TTF", "/usr/share/fonts/dejavu"]
FONTS = ["DejaVuSansMono.ttf", "DejaVuSans.ttf"]
BOLD = ["DejaVuSansMono-Bold.ttf", "DejaVuSans-Bold.ttf"]
CW, CH, PT, PAD = 9, 19, 14, 12

BG, FG = "#1d1f21", "#c5c8c6"
PALETTE = {
    "black": "#1d1f21", "red": "#cc6666", "green": "#b5bd68", "brown": "#f0c674", "yellow": "#f0c674",
    "blue": "#81a2be", "magenta": "#b294bb", "cyan": "#8abeb7", "white": "#c5c8c6",
    "brightblack": "#707880", "brightred": "#d54e53", "brightgreen": "#b9ca4a", "brightbrown": "#e7c547",
    "brightyellow": "#e7c547", "brightblue": "#7aa6da", "brightmagenta": "#c397d8", "brightcyan": "#70c0b1",
    "brightwhite": "#eaeaea",
}
KEYS = {"esc": "\x1b", "enter": "\r", "tab": "\t", "s-tab": "\x1b[Z", "space": " ", "bs": "\x7f",
        "up": "\x1b[A", "down": "\x1b[B", "right": "\x1b[C", "left": "\x1b[D",
        "pgup": "\x1b[5~", "pgdn": "\x1b[6~", "home": "\x1b[H", "end": "\x1b[F"}


def find_font(name):
    for d in FONT_DIRS:
        path = os.path.join(d, name)
        if os.path.exists(path):
            return path
    sys.exit(f"font {name} not found; install DejaVu fonts")


class Fonts:
    def __init__(self):
        self.regular = [ImageFont.truetype(find_font(n), PT) for n in FONTS]
        self.bold = [ImageFont.truetype(find_font(n), PT) for n in BOLD]
        self.cmaps = []
        try:
            from fontTools.ttLib import TTFont
            self.cmaps = [set(TTFont(find_font(n)).getBestCmap()) for n in FONTS]
        except ImportError:
            pass

    def pick(self, ch, bold):
        fonts = self.bold if bold else self.regular
        for i, cmap in enumerate(self.cmaps):
            if ord(ch[0]) in cmap:
                return fonts[i]
        return fonts[0]


def color(name, default, bold=False):
    if name == "default":
        return default
    if bold and name in PALETTE and not name.startswith("bright") and name != "black":
        return PALETTE.get("bright" + name, PALETTE[name])
    if name in PALETTE:
        return PALETTE[name]
    if re.fullmatch(r"[0-9a-fA-F]{6}", name):
        return "#" + name
    return default


# The background set with OSC 11, as goradion's themes do; pyte ignores it.
OSC_BG = re.compile(rb"\x1b\](11;(#[0-9a-fA-F]{6})|111)(\x1b\\|\x07)")


def render(screen, fonts, path, term_bg=BG):
    cols, rows = screen.columns, screen.lines
    img = Image.new("RGB", (cols * CW + 2 * PAD, rows * CH + 2 * PAD), term_bg)
    draw = ImageDraw.Draw(img)
    for y in range(rows):
        line = screen.buffer[y]
        for x in range(cols):
            ch = line[x]
            fg, bg = color(ch.fg, FG), color(ch.bg, term_bg)
            if ch.reverse:
                fg, bg = bg, fg
            px, py = PAD + x * CW, PAD + y * CH
            if bg != term_bg:
                draw.rectangle([px, py, px + CW - 1, py + CH - 1], fill=bg)
            if ch.data and ch.data.strip():
                if 0x2500 <= ord(ch.data[0]) <= 0x259F:
                    draw_block(draw, ch.data[0], px, py, fg) or draw.text((px, py), ch.data, fill=fg, font=fonts.pick(ch.data, ch.bold))
                else:
                    draw.text((px, py + 1), ch.data, fill=fg, font=fonts.pick(ch.data, ch.bold))
            if ch.underscore:
                draw.line([px, py + CH - 2, px + CW - 1, py + CH - 2], fill=fg)
    img.save(path)


def draw_block(draw, ch, px, py, fg):
    """Draw box-drawing and block characters edge to edge, like a terminal does."""
    mx, my = px + CW // 2, py + CH // 2
    right, bottom = px + CW - 1, py + CH - 1
    light = {"─": "lr", "│": "tb", "╭": "rb", "╮": "lb", "╰": "rt", "╯": "lt", "┌": "rb", "┐": "lb",
             "└": "rt", "┘": "lt", "├": "tbr", "┤": "tbl", "┬": "lrb", "┴": "lrt", "┼": "lrtb", "╴": "l", "╶": "r"}
    heavy = {"━": "lr", "┃": "tb", "╸": "l", "╺": "r"}
    for table, width in ((light, 1), (heavy, 3)):
        if ch in table:
            segs = table[ch]
            for s in segs:
                if s == "l": draw.rectangle([px, my - width // 2, mx, my + width // 2], fill=fg)
                if s == "r": draw.rectangle([mx, my - width // 2, right, my + width // 2], fill=fg)
                if s == "t": draw.rectangle([mx - width // 2, py, mx + width // 2, my], fill=fg)
                if s == "b": draw.rectangle([mx - width // 2, my, mx + width // 2, bottom], fill=fg)
            return True
    blocks = "▁▂▃▄▅▆▇█"
    if ch in blocks:
        h = (blocks.index(ch) + 1) * CH // 8
        draw.rectangle([px, bottom - h + 1, right, bottom], fill=fg)
        return True
    if ch == "▀":
        draw.rectangle([px, py, right, my - 1], fill=fg); return True
    if ch == "▄":
        draw.rectangle([px, my, right, bottom], fill=fg); return True
    return False


def parse_keys(text):
    out, i = [], 0
    while i < len(text):
        m = re.match(r"<([a-z0-9-]+)>", text[i:])
        if m:
            name = m.group(1)
            if name in KEYS:
                out.append(KEYS[name])
            elif name.startswith("c-") and len(name) == 3:
                out.append(chr(ord(name[2]) & 0x1F))
            elif name.startswith("m-") and len(name) == 3:
                out.append("\x1b" + name[2])
            else:
                raise SystemExit(f"unknown key <{name}>")
            i += len(m.group(0))
        else:
            out.append(text[i])
            i += 1
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--cols", type=int, default=100)
    ap.add_argument("--rows", type=int, default=30)
    ap.add_argument("--out", default=".")
    ap.add_argument("script")
    ap.add_argument("command", nargs=argparse.REMAINDER)
    opts = ap.parse_args()
    command = opts.command[1:] if opts.command[:1] == ["--"] else opts.command
    steps = json.load(open(opts.script))
    os.makedirs(opts.out, exist_ok=True)

    screen = pyte.Screen(opts.cols, opts.rows)
    stream = pyte.ByteStream(screen)
    fonts = Fonts()

    pid, fd = pty.fork()
    if pid == 0:
        os.environ.update(TERM="xterm-256color", COLORTERM="", LANG=os.environ.get("LANG", "C.UTF-8"))
        os.execvp(command[0], command)

    def set_size(cols, rows):
        fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))
        screen.resize(rows, cols)
        os.kill(pid, signal.SIGWINCH)

    term = {"bg": BG, "tail": b""}

    def pump(seconds):
        end = time.time() + seconds
        while time.time() < end:
            ready, _, _ = select.select([fd], [], [], 0.02)
            if ready:
                try:
                    data = os.read(fd, 1 << 16)
                except OSError:
                    return
                # Keep a tail, as a sequence may be split between reads.
                for m in OSC_BG.finditer(term["tail"] + data):
                    term["bg"] = m.group(2).decode() if m.group(2) else BG
                term["tail"] = data[-32:]
                stream.feed(data)

    set_size(opts.cols, opts.rows)
    pump(1.5)
    for step in steps:
        if "resize" in step:
            set_size(*step["resize"])
        if "keys" in step:
            for k in parse_keys(step["keys"]):
                os.write(fd, k.encode())
                pump(0.08)
        if "mouse" in step:
            x, y, what = step["mouse"]
            button = {"click": 0, "wheelup": 64, "wheeldown": 65}[what]
            os.write(fd, f"\x1b[<{button};{x + 1};{y + 1}M".encode())
            if what == "click":
                pump(0.05)
                os.write(fd, f"\x1b[<{button};{x + 1};{y + 1}m".encode())
        pump(step.get("wait", 0.6))
        if "shot" in step:
            path = os.path.join(opts.out, step["shot"])
            render(screen, fonts, path, term["bg"])
            print("saved", path)
    # The child leads its own session, so this also stops the mpv it started.
    for sig in (signal.SIGTERM, signal.SIGKILL):
        try:
            os.killpg(pid, sig)
        except ProcessLookupError:
            break
        pump(0.3)
    try:
        os.waitpid(pid, 0)
    except ChildProcessError:
        pass


if __name__ == "__main__":
    main()
