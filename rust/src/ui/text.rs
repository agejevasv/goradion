//! Styled text runs measured in terminal cells. Stream titles are printed as
//! they are, whatever brackets or markup they hold.

use std::time::Duration;

use ratatui::buffer::Buffer;
use ratatui::style::Style;
use unicode_segmentation::UnicodeSegmentation;
use unicode_width::UnicodeWidthStr;

use super::Look;

#[derive(Clone, Debug, PartialEq)]
pub struct Seg {
    pub text: String,
    pub style: Style,
}

pub fn seg(text: impl Into<String>, style: Style) -> Seg {
    Seg { text: text.into(), style }
}

pub fn width(s: &str) -> usize {
    s.graphemes(true).map(UnicodeWidthStr::width).sum()
}

pub fn segs_width(segs: &[Seg]) -> usize {
    segs.iter().map(|s| width(&s.text)).sum()
}

/// Draws segs from x, clipped to max_width cells; returns the cells used.
pub fn draw_segs(buf: &mut Buffer, x: u16, y: u16, max_width: usize, segs: &[Seg]) -> usize {
    let area = buf.area;
    if y < area.top() || y >= area.bottom() {
        return 0;
    }
    let mut used = 0;
    for s in segs {
        for g in s.text.graphemes(true) {
            let w = UnicodeWidthStr::width(g);
            if w == 0 {
                continue;
            }
            if used + w > max_width {
                return used;
            }
            let cx = x as usize + used;
            if cx + w > area.right() as usize {
                return used;
            }
            let cell = &mut buf[(cx as u16, y)];
            cell.reset();
            cell.set_symbol(g).set_style(s.style);
            for extra in 1..w {
                let cell = &mut buf[((cx + extra) as u16, y)];
                cell.reset();
                cell.set_style(s.style);
            }
            used += w;
        }
    }
    used
}

/// Fills cells with spaces in style.
pub fn fill(buf: &mut Buffer, x: u16, y: u16, w: usize, style: Style) {
    let spaces = " ".repeat(w);
    draw_segs(buf, x, y, w, &[seg(spaces, style)]);
}

/// Cuts segs to width, ending in an ellipsis when something was cut.
pub fn fit_segs(look: &Look, segs: &[Seg], w: usize) -> Vec<Seg> {
    if w == 0 {
        return Vec::new();
    }
    if segs_width(segs) <= w {
        return segs.to_vec();
    }
    let mut budget = w.saturating_sub(width(look.g.ellipsis));
    let mut out = Vec::new();
    for s in segs {
        if budget == 0 {
            break;
        }
        let cut = truncate_cells(&s.text, budget);
        budget -= width(cut);
        let whole = cut.len() == s.text.len();
        out.push(seg(cut, s.style));
        if !whole {
            break;
        }
    }
    let style = out.last().map_or(look.t.dim(), |s| s.style);
    out.push(seg(look.g.ellipsis, style));
    out
}

pub fn truncate_cells(s: &str, w: usize) -> &str {
    let (mut used, mut end) = (0, 0);
    for (i, g) in s.grapheme_indices(true) {
        let gw = UnicodeWidthStr::width(g);
        if used + gw > w {
            break;
        }
        used += gw;
        end = i + g.len();
    }
    &s[..end]
}

/// Cells skip..skip+w of s. A wide character cut in half becomes a space.
pub fn slice_cells(s: &str, skip: usize, w: usize) -> String {
    let mut out = String::new();
    let (mut pos, mut used) = (0, 0);
    for g in s.graphemes(true) {
        if used >= w {
            break;
        }
        let gw = UnicodeWidthStr::width(g);
        if pos + gw <= skip {
        } else if pos < skip {
            for _ in 0..(pos + gw - skip).min(w - used) {
                out.push(' ');
                used += 1;
            }
        } else if used + gw <= w {
            out.push_str(g);
            used += gw;
        } else {
            while used < w {
                out.push(' ');
                used += 1;
            }
        }
        pos += gw;
    }
    out
}

const MARQUEE_PAUSE: Duration = Duration::from_secs(2);
const MARQUEE_STEP: Duration = Duration::from_millis(300);
const MARQUEE_GAP: usize = 3;

/// Scrolls text that is wider than w, after a pause at the start.
pub fn marquee(text: &str, w: usize, elapsed: Duration) -> String {
    let text_width = width(text);
    if text_width <= w {
        return text.to_string();
    }
    let cycle = text_width + MARQUEE_GAP;
    let period = MARQUEE_PAUSE + MARQUEE_STEP * cycle as u32;
    let t = Duration::from_nanos((elapsed.as_nanos() % period.as_nanos()) as u64);
    let offset = if t >= MARQUEE_PAUSE { ((t - MARQUEE_PAUSE).as_nanos() / MARQUEE_STEP.as_nanos()) as usize } else { 0 };
    let looped = format!("{text}{}{text}", " ".repeat(MARQUEE_GAP));
    slice_cells(&looped, offset, w)
}

pub fn gauge(look: &Look, w: usize, fraction: f64, fill: Style, track: Style) -> Vec<Seg> {
    if w == 0 {
        return Vec::new();
    }
    let fraction = fraction.clamp(0.0, 1.0);
    let g = look.g;
    let (full, half) = if g.gauge_half.is_empty() {
        ((fraction * w as f64 + 0.5) as usize, 0)
    } else {
        let halves = (fraction * w as f64 * 2.0 + 0.5) as usize;
        (halves / 2, halves % 2)
    };
    let empty = w.saturating_sub(full + half);
    let mut segs = vec![seg(g.gauge_full.repeat(full), fill)];
    if half > 0 {
        segs.push(seg(g.gauge_half, fill));
    }
    segs.push(seg(g.gauge_empty.repeat(empty), track));
    segs
}

pub fn clock(d: Duration) -> String {
    let secs = d.as_secs();
    let (h, m, s) = (secs / 3600, secs / 60 % 60, secs % 60);
    if h > 0 { format!("{h}:{m:02}:{s:02}") } else { format!("{m}:{s:02}") }
}

/// Puts right at the far end of w, after left; drops right when it would
/// leave left fewer than 12 cells.
pub fn spread(look: &Look, left: &[Seg], right: &[Seg], w: usize) -> Vec<Seg> {
    let mut right = right;
    let mut rw = segs_width(right);
    if rw > 0 && w.saturating_sub(rw + 2) < 12 {
        right = &[];
        rw = 0;
    }
    let room = if rw > 0 { w - rw - 2 } else { w };
    let mut out = fit_segs(look, left, room);
    if rw > 0 {
        let pad = w - segs_width(&out) - rw;
        out.push(seg(" ".repeat(pad), look.t.text()));
        out.extend_from_slice(right);
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ui::{glyphs, theme};

    fn look() -> Look {
        Look { t: theme::THEMES[0], g: &glyphs::UNICODE }
    }

    fn text(segs: &[Seg]) -> String {
        segs.iter().map(|s| s.text.as_str()).collect()
    }

    #[test]
    fn fitting() {
        let l = look();
        let segs = [seg("Hello ", l.t.text()), seg("world", l.t.dim())];
        assert_eq!(text(&fit_segs(&l, &segs, 20)), "Hello world");
        assert_eq!(text(&fit_segs(&l, &segs, 8)), "Hello w…");
        assert_eq!(text(&fit_segs(&l, &segs, 0)), "");
    }

    #[test]
    fn wide_characters() {
        assert_eq!(width("日本"), 4);
        assert_eq!(truncate_cells("日本語", 5), "日本");
        assert_eq!(slice_cells("日本語", 1, 4), " 本 ");
    }

    #[test]
    fn marquee_scrolls_after_pause() {
        assert_eq!(marquee("short", 10, Duration::ZERO), "short");
        assert_eq!(marquee("abcdefgh", 4, Duration::from_secs(1)), "abcd");
        assert_eq!(marquee("abcdefgh", 4, MARQUEE_PAUSE + MARQUEE_STEP * 2), "cdef");
    }

    #[test]
    fn gauges_and_clocks() {
        let l = look();
        assert_eq!(text(&gauge(&l, 4, 0.5, l.t.text(), l.t.dim())), "━━──");
        assert_eq!(text(&gauge(&l, 4, 0.625, l.t.text(), l.t.dim())), "━━╸─");
        assert_eq!(clock(Duration::from_secs(75)), "1:15");
        assert_eq!(clock(Duration::from_secs(3725)), "1:02:05");
    }

    #[test]
    fn spreading() {
        let l = look();
        let row = spread(&l, &[seg("left", l.t.text())], &[seg("right", l.t.text())], 20);
        assert_eq!(text(&row), "left           right");
        let row = spread(&l, &[seg("left", l.t.text())], &[seg("right", l.t.text())], 16);
        assert_eq!(text(&row), "left");
    }
}
