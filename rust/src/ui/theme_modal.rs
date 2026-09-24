//! The theme picker previews the theme under the cursor on the whole UI.
//! Enter saves it, Esc goes back to the saved one.

use std::io::Write;

use ratatui::buffer::Buffer;
use ratatui::crossterm::event::{KeyCode, KeyEvent, MouseButton, MouseEvent, MouseEventKind};
use ratatui::layout::{Position, Rect};
use ratatui::style::Color;

use super::list::{self, ListState, Pane, Row};
use super::text::{fill, seg};
use super::theme::{self, THEMES};
use super::{App, Modal};
use crate::log;

const NAME_WIDTH: usize = 20;

impl App {
    pub(super) fn show_theme_modal(&mut self) {
        let mut state = ListState::default();
        state.cursor = THEMES.iter().position(|t| t.name == self.config.theme).unwrap_or(0);
        self.modal = Some(Modal::Theme(state));
    }

    fn cancel_theme_modal(&mut self) {
        self.look.t = theme::find(&self.config.theme).unwrap_or(THEMES[0]);
        self.modal = None;
    }

    fn save_theme(&mut self, i: usize) {
        self.look.t = THEMES[i];
        self.config.theme = THEMES[i].name.to_string();
        if let Err(e) = self.config.save(&["theme"]) {
            log!("config: {e}");
        }
        self.modal = None;
    }

    fn theme_cursor(&mut self) -> Option<&mut ListState> {
        match &mut self.modal {
            Some(Modal::Theme(state)) => Some(state),
            _ => None,
        }
    }

    fn preview_theme(&mut self) {
        if let Some(i) = self.theme_cursor().map(|s| s.cursor) {
            self.look.t = THEMES[i];
        }
    }

    pub(super) fn on_theme_key(&mut self, k: KeyEvent) {
        let Some(state) = self.theme_cursor() else { return };
        let page = state.page();
        let delta = match k.code {
            KeyCode::Up => -1,
            KeyCode::Down => 1,
            KeyCode::PageUp => -page,
            KeyCode::PageDown => page,
            KeyCode::Home => isize::MIN / 2,
            KeyCode::End => isize::MAX / 2,
            KeyCode::Enter | KeyCode::Char(' ') => {
                let i = state.cursor;
                self.save_theme(i);
                return;
            }
            KeyCode::Esc => {
                self.cancel_theme_modal();
                return;
            }
            KeyCode::Char('t') if k.modifiers.contains(ratatui::crossterm::event::KeyModifiers::CONTROL) => {
                self.cancel_theme_modal();
                return;
            }
            _ => return,
        };
        state.move_by(delta, THEMES.len());
        self.preview_theme();
    }

    pub(super) fn on_theme_mouse(&mut self, m: MouseEvent) {
        let area = self.modal_area;
        if !area.contains(Position::new(m.column, m.row)) {
            return;
        }
        let Some(state) = self.theme_cursor() else { return };
        match m.kind {
            MouseEventKind::ScrollUp => state.scroll(-1, THEMES.len()),
            MouseEventKind::ScrollDown => state.scroll(1, THEMES.len()),
            MouseEventKind::Down(MouseButton::Left) => {
                if let Some(i) = state.row_at(m.row, THEMES.len()) {
                    self.save_theme(i);
                }
                return;
            }
            _ => return,
        }
        self.preview_theme();
    }

    /// Each row shows the theme's own colours, so themes can be compared
    /// without previewing each one.
    pub(super) fn draw_theme_modal(&mut self, buf: &mut Buffer, screen: Rect) {
        let (t, g) = (self.look.t, self.look.g);
        let rows: Vec<Row> = THEMES
            .iter()
            .map(|th| {
                let saved = th.name == self.config.theme;
                let mark = if saved { format!("{} ", g.check) } else { "  ".to_string() };
                let mut label = vec![seg(format!("{mark}{:<NAME_WIDTH$}  ", th.name), t.text())];
                for c in [th.accent, th.bright, th.warn, th.danger] {
                    label.push(seg(g.swatch, t.fg(c)));
                }
                Row { label, ..Row::default() }
            })
            .collect();
        let width = (5 + 2 + NAME_WIDTH + 2 + 4 * 2) as u16;
        let height = (THEMES.len() as u16 + 2).min(screen.height);
        let area = Rect {
            x: screen.x + screen.width.saturating_sub(width) / 2,
            y: screen.y + screen.height.saturating_sub(height) / 2,
            width: width.min(screen.width),
            height,
        };
        self.modal_area = area;
        for y in area.top()..area.bottom() {
            fill(buf, area.x, y, area.width as usize, t.text());
        }
        let pane = Pane { title: vec![seg(" Theme ", t.text())], focused: true, rows: &rows, empty: None };
        if let Some(Modal::Theme(state)) = &mut self.modal {
            list::draw(&self.look, buf, area, &pane, state);
        }
    }

    /// The terminal's padding around the cell grid can't be drawn on, so a
    /// themed background is also set as the terminal's default background
    /// (OSC 11), and reset on exit (OSC 111). Terminals without OSC 11
    /// ignore both.
    pub(super) fn sync_term_bg(&mut self) {
        let bg = self.look.t.bg;
        if bg == self.term_bg {
            return;
        }
        let seq = match bg {
            Color::Rgb(r, g, b) => format!("\x1b]11;#{r:02x}{g:02x}{b:02x}\x1b\\"),
            _ => "\x1b]111\x1b\\".to_string(),
        };
        let mut out = std::io::stdout();
        if let Err(e) = out.write_all(seq.as_bytes()).and_then(|_| out.flush()) {
            log!("terminal background: {e}");
        }
        self.term_bg = bg;
    }
}
