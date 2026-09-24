//! The search modal: your own stations as you type, or radio-browser.info on
//! Enter. Picking a result shows the results in the stations list and adds the
//! search to the tags.

use std::sync::mpsc::{self, Receiver, TryRecvError};
use std::thread;

use ratatui::buffer::Buffer;
use ratatui::crossterm::event::{KeyCode, KeyEvent, KeyModifiers, MouseButton, MouseEvent, MouseEventKind};
use ratatui::layout::{Position, Rect};
use ratatui::style::{Color, Modifier};

use super::list::{self, ListState, Pane, Row};
use super::text::{draw_segs, fill, seg};
use super::{App, LastSearch, Modal, Page, TagRef};
use crate::radiobrowser::{self, OnlineStation};
use crate::stations::Station;

const ONLINE_HINT: &str = "Press Enter to search radio-browser.info";

type OnlineResult = (u64, Result<Vec<OnlineStation>, String>);

pub(super) struct Search {
    online: bool,
    input: String,
    /// In chars.
    caret: usize,
    results: Vec<Found>,
    /// The query the results are for.
    results_query: String,
    status: Option<(String, Color)>,
    list: ListState,
    focus_results: bool,
    generation: u64,
    pending: Option<Receiver<OnlineResult>>,
}

struct Found {
    station: Station,
    meta: String,
}

/// The stations whose title or tags contain every word of the query,
/// ignoring case.
pub(super) fn search_stations(stations: &[Station], query: &str) -> Vec<Station> {
    let words: Vec<String> = query.split_whitespace().map(str::to_lowercase).collect();
    if words.is_empty() {
        return Vec::new();
    }
    stations
        .iter()
        .filter(|s| {
            let title = s.title.to_lowercase();
            words.iter().all(|w| title.contains(w) || s.tags.iter().any(|t| t.to_lowercase().contains(w)))
        })
        .cloned()
        .collect()
}

fn online_meta(r: &OnlineStation) -> String {
    let mut parts = Vec::new();
    if !r.country_code.is_empty() {
        parts.push(r.country_code.clone());
    }
    if r.bitrate > 0 {
        parts.push(format!("{}k", r.bitrate));
    }
    parts.join(", ")
}

impl App {
    fn search_modal(&mut self) -> Option<&mut Search> {
        match &mut self.modal {
            Some(Modal::Search(s)) => Some(s),
            _ => None,
        }
    }

    /// Opens the modal in the given mode; when it is open, flips the mode and
    /// runs the query again.
    pub(super) fn show_search_modal(&mut self, online: bool) {
        if let Some(s) = self.search_modal() {
            let flipped = !s.online;
            self.set_search_mode(flipped);
            return;
        }
        let search = Search {
            online,
            input: String::new(),
            caret: 0,
            results: Vec::new(),
            results_query: String::new(),
            status: None,
            list: ListState::default(),
            focus_results: false,
            generation: 0,
            pending: None,
        };
        self.modal = Some(Modal::Search(Box::new(search)));
        self.set_search_mode(online);
    }

    fn set_search_mode(&mut self, online: bool) {
        let t = self.look.t;
        let Some(s) = self.search_modal() else { return };
        s.online = online;
        s.generation += 1;
        s.pending = None;
        s.focus_results = false;
        if !online {
            self.update_local_results();
        } else if !s.input.is_empty() {
            self.start_online_search();
        } else {
            s.results.clear();
            s.status = Some((ONLINE_HINT.to_string(), t.dim));
        }
    }

    fn update_local_results(&mut self) {
        let t = self.look.t;
        let found = match &self.modal {
            Some(Modal::Search(s)) => search_stations(&self.stations, &s.input),
            _ => return,
        };
        let Some(s) = self.search_modal() else { return };
        s.results = found.into_iter().map(|station| Found { station, meta: String::new() }).collect();
        s.results_query = s.input.clone();
        s.list = ListState::default();
        s.status = (s.results.is_empty() && !s.input.trim().is_empty()).then(|| ("No stations match".to_string(), t.dim));
    }

    fn start_online_search(&mut self) {
        let t = self.look.t;
        let Some(s) = self.search_modal() else { return };
        s.generation += 1;
        let generation = s.generation;
        let query = s.input.clone();
        s.results.clear();
        s.status = Some(("Searching...".to_string(), t.warn));
        let (tx, rx) = mpsc::channel();
        s.pending = Some(rx);
        thread::Builder::new()
            .name("search".into())
            .spawn(move || {
                let _ = tx.send((generation, radiobrowser::search(&query)));
            })
            .expect("spawning a thread");
    }

    /// Takes the online results in, unless a newer search or mode made them
    /// stale.
    pub(super) fn poll_search(&mut self) {
        let t = self.look.t;
        let Some(s) = self.search_modal() else { return };
        let Some(rx) = &s.pending else { return };
        let (generation, result) = match rx.try_recv() {
            Ok(r) => r,
            Err(TryRecvError::Empty) => return,
            Err(TryRecvError::Disconnected) => {
                s.pending = None;
                return;
            }
        };
        s.pending = None;
        if generation != s.generation {
            return;
        }
        match result {
            Ok(found) => {
                s.results = found.iter().map(|r| Found { station: r.station.clone(), meta: online_meta(r) }).collect();
                s.results_query = s.input.clone();
                s.list = ListState::default();
                s.status = s.results.is_empty().then(|| ("No stations match".to_string(), t.dim));
            }
            Err(e) => s.status = Some((format!("Error: {e}"), t.danger)),
        }
    }

    pub(super) fn searching(&self) -> bool {
        matches!(&self.modal, Some(Modal::Search(s)) if s.pending.is_some())
    }

    /// The station under the results cursor, when the results have focus.
    pub(super) fn search_cursor_station(&self) -> Option<Station> {
        match &self.modal {
            Some(Modal::Search(s)) if s.focus_results => s.results.get(s.list.cursor).map(|f| f.station.clone()),
            _ => None,
        }
    }

    fn select_search_result(&mut self, i: usize) {
        let Some(s) = self.search_modal() else { return };
        let Some(selected) = s.results.get(i).map(|f| f.station.clone()) else { return };
        let stations = s.results.iter().map(|f| f.station.clone()).collect();
        let (query, online) = (s.results_query.clone(), s.online);
        self.open_search(&query, stations, online);
        self.select_station(&selected.url);
        self.toggle_play_manual(selected);
    }

    /// Shows search results in the stations list, and adds the search to the
    /// tags.
    pub(super) fn open_search(&mut self, query: &str, stations: Vec<Station>, online: bool) {
        self.last_search = Some(LastSearch { query: query.to_string(), stations, online });
        self.load_tag(TagRef::search(query));
        self.tags_state.filter.clear();
        self.stations_state.filter.clear();
        self.modal = None;
        self.show(Page::Main);
    }

    pub(super) fn on_search_key(&mut self, k: KeyEvent) {
        let ctrl = k.modifiers.contains(KeyModifiers::CONTROL);
        let Some(s) = self.search_modal() else { return };
        match k.code {
            KeyCode::Esc => self.modal = None,
            KeyCode::Char('f') if ctrl => self.show_search_modal(false),
            KeyCode::Char('s') if ctrl => self.show_search_modal(true),
            KeyCode::Char('b') if ctrl => self.toggle_bookmark(),
            KeyCode::Char('r') if ctrl => self.toggle_shuffle(std::time::Instant::now()),
            KeyCode::Char('z') if ctrl => self.cycle_sleep(std::time::Instant::now()),
            KeyCode::Char(_) if ctrl || k.modifiers.contains(KeyModifiers::ALT) => {}
            KeyCode::Enter if s.focus_results => {
                let i = s.list.cursor;
                self.select_search_result(i);
            }
            KeyCode::Enter => {
                if s.input.trim().is_empty() {
                    return;
                }
                let (query, online) = (s.input.clone(), s.online);
                if online {
                    self.start_online_search();
                } else {
                    let stations = search_stations(&self.stations, &query);
                    if !stations.is_empty() {
                        self.open_search(&query, stations, false);
                    }
                }
            }
            KeyCode::Down | KeyCode::Tab if !s.focus_results => {
                if !s.results.is_empty() {
                    s.focus_results = true;
                    s.list.cursor = 0;
                }
            }
            KeyCode::Up if s.focus_results && s.list.cursor == 0 => s.focus_results = false,
            KeyCode::Up if s.focus_results => s.list.move_by(-1, s.results.len()),
            KeyCode::Down if s.focus_results => s.list.move_by(1, s.results.len()),
            KeyCode::PageUp if s.focus_results => s.list.move_by(-s.list.page(), s.results.len()),
            KeyCode::PageDown if s.focus_results => s.list.move_by(s.list.page(), s.results.len()),
            KeyCode::Left if !s.focus_results => s.caret = s.caret.saturating_sub(1),
            KeyCode::Right if !s.focus_results => s.caret = (s.caret + 1).min(s.input.chars().count()),
            KeyCode::Home if !s.focus_results => s.caret = 0,
            KeyCode::End if !s.focus_results => s.caret = s.input.chars().count(),
            KeyCode::Backspace => {
                s.focus_results = false;
                if s.caret > 0 {
                    let at = s.input.char_indices().nth(s.caret - 1).map_or(0, |(i, _)| i);
                    s.input.remove(at);
                    s.caret -= 1;
                    if !s.online {
                        self.update_local_results();
                    }
                }
            }
            KeyCode::Char(c) => {
                s.focus_results = false;
                let at = s.input.char_indices().nth(s.caret).map_or(s.input.len(), |(i, _)| i);
                s.input.insert(at, c);
                s.caret += 1;
                if !s.online {
                    self.update_local_results();
                }
            }
            _ => {}
        }
    }

    pub(super) fn on_search_mouse(&mut self, m: MouseEvent) {
        if !self.modal_area.contains(Position::new(m.column, m.row)) {
            return;
        }
        let Some(s) = self.search_modal() else { return };
        let len = s.results.len();
        match m.kind {
            MouseEventKind::ScrollUp => s.list.scroll(-1, len),
            MouseEventKind::ScrollDown => s.list.scroll(1, len),
            MouseEventKind::Down(MouseButton::Left) => {
                if let Some(i) = s.list.row_at(m.row, len) {
                    self.select_search_result(i);
                }
            }
            _ => {}
        }
    }

    pub(super) fn draw_search_modal(&mut self, buf: &mut Buffer, screen: Rect) {
        let (t, g) = (self.look.t, self.look.g);
        let margin_x = screen.width / 20;
        let margin_y = screen.height / 20;
        let area = Rect {
            x: screen.x + margin_x,
            y: screen.y + margin_y,
            width: screen.width - 2 * margin_x,
            height: screen.height - 2 * margin_y,
        };
        self.modal_area = area;
        if area.width < 20 || area.height < 4 {
            return;
        }
        for y in area.top()..area.bottom() {
            fill(buf, area.x, y, area.width as usize, t.text());
        }
        let bookmarked: Vec<bool> = match &self.modal {
            Some(Modal::Search(s)) => s.results.iter().map(|f| self.bookmarks.has(&f.station.url)).collect(),
            _ => return,
        };
        let Some(Modal::Search(s)) = &mut self.modal else { return };

        let mode = if s.online { t.warn } else { t.accent };
        let badge = |label: &str, on: bool| {
            if on {
                seg(format!(" {label} "), t.text().fg(t.on_accent).bg(mode).add_modifier(Modifier::BOLD))
            } else {
                seg(format!(" {label} "), t.text())
            }
        };
        let title = vec![seg(" Station Search:", t.text()), badge("Local", !s.online), badge("Online", s.online), seg(" ", t.text())];
        super::draw_box(&self.look, buf, area, t.dim(), &title);

        let (x, y, w) = (area.x + 3, area.y + 1, area.width as usize - 5);
        let label = seg("Search: ", t.fg(mode).add_modifier(Modifier::BOLD));
        let used = draw_segs(buf, x, y, w, &[label]);
        let caret_style = t.text().add_modifier(Modifier::REVERSED);
        let (text, text_style) = if s.input.is_empty() {
            ("station name or tag".to_string(), t.dim())
        } else {
            (s.input.clone(), t.text())
        };
        let mut segs = Vec::new();
        for (i, c) in text.chars().enumerate() {
            let style = if i == s.caret && !s.focus_results { caret_style } else { text_style };
            segs.push(seg(c.to_string(), style));
        }
        if s.caret >= text.chars().count() && !s.focus_results {
            segs.push(seg(" ", caret_style));
        }
        if s.input.is_empty() && !s.focus_results {
            segs[0].style = caret_style.fg(t.dim);
        }
        draw_segs(buf, x + used as u16, y, w - used, &segs);

        let rows: Vec<Row> = s
            .results
            .iter()
            .zip(bookmarked)
            .map(|(f, bookmarked)| {
                let mut label = vec![seg(f.station.title.as_str(), t.text())];
                if bookmarked {
                    label.push(seg(format!(" {}", g.star), t.dim()));
                }
                if !f.meta.is_empty() {
                    label.push(seg(format!(" ({})", f.meta), t.dim()));
                }
                Row { label, ..Row::default() }
            })
            .collect();
        let empty = s.status.as_ref().map(|(text, color)| seg(text.as_str(), t.fg(*color)));
        let pane = Pane { title: Vec::new(), focused: s.focus_results, rows: &rows, empty };
        let rows_area = Rect { x: area.x + 1, y: area.y + 3, width: area.width - 2, height: area.height.saturating_sub(4) };
        list::draw_rows(&self.look, buf, rows_area, &pane, &mut s.list);
    }

    pub(super) fn search_hints(&self) -> Vec<(&'static str, &'static str)> {
        let online = matches!(&self.modal, Some(Modal::Search(s)) if s.online);
        if online {
            vec![("enter", "search"), ("↓", "results"), ("^F", "search local"), ("^B", "bookmark"), ("esc", "close")]
        } else {
            vec![("enter", "show all"), ("↓", "results"), ("^F", "search online"), ("^B", "bookmark"), ("esc", "close")]
        }
    }
}

#[cfg(test)]
mod tests {
    use super::super::tests::test_app;
    use super::*;
    use crate::config::Config;

    fn st(title: &str, tags: &[&str]) -> Station {
        Station { title: title.into(), url: format!("http://{title}"), tags: tags.iter().map(|t| t.to_string()).collect() }
    }

    #[test]
    fn stations_search() {
        let stations = [
            st("SomaFM: Groove Salad", &["Downtempo", "Electronic"]),
            st("SomaFM: Bossa Beyond", &["Jazz", "Lounge"]),
            st("Linn Jazz", &["Jazz"]),
        ];
        let titles = |q: &str| search_stations(&stations, q).into_iter().map(|s| s.title).collect::<Vec<_>>();
        assert_eq!(titles("soma"), ["SomaFM: Groove Salad", "SomaFM: Bossa Beyond"]);
        assert_eq!(titles("SOMA jazz"), ["SomaFM: Bossa Beyond"]);
        assert_eq!(titles("jazz"), ["SomaFM: Bossa Beyond", "Linn Jazz"]);
        assert_eq!(titles("electro soma"), ["SomaFM: Groove Salad"]);
        assert!(titles("polka").is_empty());
        assert!(titles("  ").is_empty());
    }

    #[test]
    fn search_named_like_tag() {
        let (mut a, _dir) = test_app();
        let found = vec![Station { title: "Found".into(), url: "http://found".into(), tags: vec![] }];
        a.open_search("Jazz", found, true);
        let rows = a.tag_rows();
        assert!(rows.contains(&TagRef::new("Jazz")) && rows.contains(&TagRef::search("Jazz")), "{rows:?}");

        a.open_tag(TagRef::new("Jazz"));
        assert!(a.listed.len() >= 2 && !a.listed.iter().any(|s| s.url == "http://found"));
        a.open_tag(TagRef::search("Jazz"));
        assert_eq!(a.listed.len(), 1);
        assert_eq!(a.listed[0].url, "http://found");
        assert!(a.stations_title().contains("Jazz (online)"), "{}", a.stations_title());
    }

    #[test]
    fn local_search_modal() {
        let (mut a, _dir) = test_app();
        a.show_search_modal(false);
        for c in "groove".chars() {
            a.on_search_key(KeyEvent::from(KeyCode::Char(c)));
        }
        let Some(Modal::Search(s)) = &a.modal else { panic!("no modal") };
        assert!(s.results.len() >= 2, "{}", s.results.len());
        a.on_search_key(KeyEvent::from(KeyCode::Down));
        a.on_search_key(KeyEvent::from(KeyCode::Enter));
        assert!(a.modal.is_none());
        assert_eq!(a.page, Page::Main);
        assert_eq!(a.tag, Some(TagRef::search("groove")));
        assert!(!a.player.snapshot().url.is_empty());
    }

    #[test]
    fn session_from_search_falls_back() {
        let (mut a, dir) = test_app();
        let s = a.stations[0].clone();
        a.open_search("x", vec![s.clone()], false);
        a.toggle_play_manual(s.clone());
        a.save_session();
        let c = Config::load_from(dir.join("config.yaml"));
        assert_eq!((c.tag.as_str(), c.station.as_str()), (s.tags[0].as_str(), s.url.as_str()));
    }
}
