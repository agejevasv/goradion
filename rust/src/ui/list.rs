//! A bordered, scrolling list pane with a cursor bar and a type-to-filter
//! query.

use ratatui::buffer::Buffer;
use ratatui::layout::Rect;
use ratatui::style::{Color, Modifier};

use super::Look;
use super::text::{Seg, draw_segs, fill, seg, width};

#[derive(Default, Debug)]
pub struct ListState {
    pub cursor: usize,
    pub offset: usize,
    pub filter: String,
    height: usize,
    /// Where the rows were last drawn.
    rows_area: Rect,
}

impl ListState {
    pub fn clamp(&mut self, len: usize) {
        self.cursor = self.cursor.min(len.saturating_sub(1));
        self.offset = self.offset.min(self.cursor);
    }

    pub fn move_by(&mut self, delta: isize, len: usize) {
        if len == 0 {
            return;
        }
        self.cursor = self.cursor.saturating_add_signed(delta).min(len - 1);
    }

    pub fn page(&self) -> isize {
        self.height.max(1) as isize
    }

    /// Scrolls the view, dragging the cursor along at the edges.
    pub fn scroll(&mut self, delta: isize, len: usize) {
        let max_offset = len.saturating_sub(self.height);
        self.offset = self.offset.saturating_add_signed(delta).min(max_offset);
        if self.height > 0 && len > 0 {
            self.cursor = self.cursor.clamp(self.offset, (self.offset + self.height - 1).min(len - 1));
        }
    }

    /// The row drawn at screen row y.
    pub fn row_at(&self, y: u16, len: usize) -> Option<usize> {
        let area = self.rows_area;
        if y < area.y || y >= area.bottom() {
            return None;
        }
        let i = self.offset + (y - area.y) as usize;
        (i < len).then_some(i)
    }

    fn ensure_visible(&mut self) {
        if self.cursor < self.offset {
            self.offset = self.cursor;
        } else if self.height > 0 && self.cursor >= self.offset + self.height {
            self.offset = self.cursor + 1 - self.height;
        }
    }
}

/// Whether every word of the filter appears in text, ignoring case.
pub fn matches(filter: &str, text: &str) -> bool {
    let text = text.to_lowercase();
    filter.to_lowercase().split_whitespace().all(|w| text.contains(w))
}

#[derive(Default)]
pub struct Row {
    pub indent: usize,
    /// One cell before the label, such as the playing mark.
    pub mark: Option<Seg>,
    pub label: Vec<Seg>,
    /// Recolours the label, such as the station playing.
    pub tint: Option<Color>,
    /// Dim text at the right end, such as a count.
    pub aside: String,
}

pub struct Pane<'a> {
    pub title: Vec<Seg>,
    pub focused: bool,
    pub rows: &'a [Row],
    /// Shown in place of rows when there are none.
    pub empty: Option<Seg>,
}

pub fn draw(look: &Look, buf: &mut Buffer, area: Rect, pane: &Pane<'_>, state: &mut ListState) {
    let t = &look.t;
    let border = if pane.focused { t.fg(t.border()) } else { t.dim() };
    let mut title = pane.title.clone();
    if !pane.focused {
        for s in &mut title {
            if s.style.fg == Some(t.text) {
                s.style = s.style.fg(t.dim);
            }
        }
    }
    if !state.filter.is_empty() {
        let style = if pane.focused { t.fg(t.warn) } else { t.dim() };
        title.push(seg(format!("/ {} ", state.filter), style));
    }
    super::draw_box(look, buf, area, border, &title);
    if area.width < 6 || area.height < 3 {
        return;
    }
    let inner = Rect { x: area.x + 1, y: area.y + 1, width: area.width - 2, height: area.height - 2 };
    draw_rows(look, buf, inner, pane, state);
}

/// Draws the rows in area: the cursor's edge in the first column, the labels
/// from the third.
pub fn draw_rows(look: &Look, buf: &mut Buffer, area: Rect, pane: &Pane<'_>, state: &mut ListState) {
    let t = &look.t;
    if area.width < 4 || area.height == 0 {
        return;
    }
    let (x, w) = (area.x + 2, area.width as usize - 3);
    state.rows_area = area;
    state.height = area.height as usize;
    state.clamp(pane.rows.len());
    state.ensure_visible();

    if pane.rows.is_empty() {
        if let Some(empty) = &pane.empty {
            draw_segs(buf, x, area.y, w, std::slice::from_ref(empty));
        }
        return;
    }

    let reversed_cursor = t.surface == Color::Reset;
    for (i, row) in pane.rows.iter().enumerate().skip(state.offset).take(state.height) {
        let y = area.y + (i - state.offset) as u16;
        let cursor = i == state.cursor;
        let bar = cursor && pane.focused;
        if bar {
            fill(buf, area.x, y, w + 2, t.selected());
        }
        let restyle = |mut s: Seg| {
            if bar && reversed_cursor {
                // A reversed coloured text would turn into a coloured bar.
                s.style = t.selected();
            } else if bar {
                s.style = s.style.bg(t.surface).add_modifier(Modifier::BOLD);
            } else if cursor {
                s.style = s.style.add_modifier(Modifier::BOLD);
            }
            s
        };

        let mut segs = vec![seg(" ".repeat(row.indent), t.text())];
        if let Some(mark) = &row.mark {
            segs.push(mark.clone());
            segs.push(seg(" ", t.text()));
        }
        for s in &row.label {
            let mut s = s.clone();
            if let Some(tint) = row.tint
                && s.style.fg == Some(t.text)
            {
                s.style = s.style.fg(tint);
            }
            segs.push(s);
        }
        let segs: Vec<Seg> = segs.into_iter().map(restyle).collect();
        let label_end = draw_segs(buf, x, y, w, &segs);

        let aside_w = width(&row.aside);
        if aside_w > 0 && label_end + 2 + aside_w <= w {
            let aside = restyle(seg(row.aside.as_str(), t.dim()));
            let aside = if bar && reversed_cursor {
                aside
            } else {
                seg(aside.text, aside.style.fg(t.dim).remove_modifier(Modifier::BOLD))
            };
            draw_segs(buf, x + (w - aside_w) as u16, y, aside_w, &[aside]);
        }

        if cursor && !look.g.edge.is_empty() {
            let edge_fg = if pane.focused { t.accent } else { t.dim };
            let mut style = t.fg(edge_fg);
            if bar && !reversed_cursor {
                style = style.bg(t.surface);
            }
            draw_segs(buf, area.x, y, 1, &[seg(look.g.edge, style)]);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn filter_words() {
        assert!(matches("soma groove", "SomaFM: Groove Salad"));
        assert!(matches("", "anything"));
        assert!(!matches("soma jazz", "SomaFM: Groove Salad"));
    }

    #[test]
    fn scrolling_drags_cursor() {
        let mut s = ListState { height: 5, ..ListState::default() };
        s.scroll(3, 20);
        assert_eq!((s.offset, s.cursor), (3, 3));
        s.scroll(100, 20);
        assert_eq!((s.offset, s.cursor), (15, 15));
        s.scroll(-2, 20);
        assert_eq!((s.offset, s.cursor), (13, 15));
    }

    #[test]
    fn cursor_stays_visible() {
        let mut s = ListState { height: 5, ..ListState::default() };
        s.move_by(7, 20);
        s.ensure_visible();
        assert_eq!((s.offset, s.cursor), (3, 7));
        s.move_by(-100, 20);
        s.ensure_visible();
        assert_eq!((s.offset, s.cursor), (0, 0));
    }
}
