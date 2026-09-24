//! The terminal UI.

mod card;
pub mod glyphs;
mod help;
mod list;
mod remote;
mod search;
mod session;
mod text;
pub mod theme;
mod theme_modal;
mod timers;

use std::collections::HashMap;
use std::io;
use std::time::{Duration, Instant, SystemTime};

use rand::RngExt;
use ratatui::DefaultTerminal;
use ratatui::buffer::Buffer;
use ratatui::crossterm::event::{
    self, DisableMouseCapture, EnableMouseCapture, Event, KeyCode, KeyEvent, KeyEventKind, KeyModifiers, MouseButton,
    MouseEvent, MouseEventKind,
};
use ratatui::crossterm::execute;
use ratatui::layout::{Position, Rect};
use ratatui::style::Style;

use crate::audio::Player;
use crate::audio::player::{State, VOLUME_STEP};
use crate::bookmarks::Bookmarks;
use crate::config::Config;
use crate::stations::Station;
use card::{Card, Extras};
use glyphs::Glyphs;
use list::{ListState, Pane, Row, matches};
use text::{Seg, draw_segs, seg, segs_width};
use theme::Theme;

pub use remote::RemoteSettings;
pub(crate) use search::{online_meta, search_stations};

const WIDE_MIN_WIDTH: u16 = 100;
const TAGS_PANE_MIN_WIDTH: u16 = 26;
const TAGS_PANE_MAX_WIDTH: u16 = 40;
const FRAME_INTERVAL: Duration = Duration::from_millis(50);
const IDLE_INTERVAL: Duration = Duration::from_millis(250);
/// How often phone requests are answered when nothing else wakes the loop.
const REMOTE_INTERVAL: Duration = Duration::from_millis(100);

const BOOKMARKS_TAG: &str = "Bookmarks";
const ALL_STATIONS_TAG: &str = "All Stations";

pub struct Look {
    pub t: Theme,
    pub g: &'static Glyphs,
}

pub struct Options {
    pub ascii: bool,
    pub vu: bool,
    /// -r: Some(None) keeps the configured key.
    pub remote: Option<Option<String>>,
    /// -p
    pub remote_port: Option<u16>,
}

enum Modal {
    Theme(ListState),
    Search(Box<search::Search>),
    /// The error when the server could not start.
    Remote(Option<String>),
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
enum Page {
    Tags,
    Main,
    Help,
}

/// Names a list of stations: a tag, All Stations, Bookmarks, or the last
/// search, which may be named like any of them.
#[derive(Clone, PartialEq, Eq, Debug)]
struct TagRef {
    name: String,
    search: bool,
}

impl TagRef {
    fn new(name: &str) -> Self {
        TagRef { name: name.to_string(), search: false }
    }

    fn search(query: &str) -> Self {
        TagRef { name: query.to_string(), search: true }
    }
}

/// The last search shown in the stations list.
struct LastSearch {
    query: String,
    stations: Vec<Station>,
    online: bool,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
enum StationRow {
    /// Back to the tags, in the narrow layout.
    Back,
    Random,
    Station(usize),
}

pub struct App {
    player: Player,
    stations: Vec<Station>,
    tags: Vec<String>,
    tag_counts: HashMap<String, usize>,
    bookmarks: Bookmarks,
    config: Config,
    look: Look,
    card: Card,

    page: Page,
    help_from: Page,
    help_scroll: usize,
    wide: bool,
    layout_ready: bool,

    /// The tag shown in the stations list; None shows all stations.
    tag: Option<TagRef>,
    listed: Vec<Station>,
    tags_state: ListState,
    stations_state: ListState,
    last_search: Option<LastSearch>,
    /// The station last started, and the tag it was started from.
    playing: Option<(Station, Option<TagRef>)>,

    shuffle: timers::Shuffle,
    sleep: timers::Sleep,
    modal: Option<Modal>,
    modal_area: Rect,
    remote: Option<(crate::remote::Server, std::sync::mpsc::Receiver<crate::remote::Call>)>,
    remote_settings: RemoteSettings,
    /// The terminal's default background as last set by sync_term_bg.
    term_bg: ratatui::style::Color,

    tags_area: Rect,
    stations_area: Rect,
    quit: bool,
}

impl App {
    pub fn new(player: Player, stations: Vec<Station>, bookmarks: Bookmarks, config: Config, opts: &Options) -> Self {
        let mut tags: Vec<String> = stations.iter().flat_map(|s| s.tags.iter().cloned()).collect();
        tags.sort();
        tags.dedup();
        let mut tag_counts = HashMap::new();
        for s in &stations {
            for (i, t) in s.tags.iter().enumerate() {
                // A tag repeated in the CSV lists the station once.
                if !s.tags[..i].contains(t) {
                    *tag_counts.entry(t.clone()).or_default() += 1;
                }
            }
        }
        let theme = theme::find(&config.theme).unwrap_or_else(|| {
            crate::log!("config: unknown theme {:?}, using {:?}", config.theme, theme::DEFAULT_THEME);
            theme::THEMES[0]
        });
        let g = if opts.ascii { &glyphs::ASCII } else { &glyphs::UNICODE };
        let listed = stations.clone();
        let remote_settings = RemoteSettings {
            autostart: config.remote.autostart || opts.remote.is_some(),
            port: opts.remote_port.unwrap_or(config.remote.port),
            key: match &opts.remote {
                Some(Some(key)) => Some(key.clone()),
                _ => config.remote.key.clone(),
            },
        };
        let mut app = App {
            player,
            stations,
            tags,
            tag_counts,
            bookmarks,
            look: Look { t: theme, g },
            card: Card::new(opts.vu),
            page: Page::Tags,
            help_from: Page::Tags,
            help_scroll: 0,
            wide: false,
            layout_ready: false,
            tag: None,
            listed,
            tags_state: ListState::default(),
            stations_state: ListState::default(),
            playing: None,
            last_search: None,
            tags_area: Rect::default(),
            stations_area: Rect::default(),
            quit: false,
            shuffle: timers::Shuffle::default(),
            sleep: timers::Sleep::default(),
            modal: None,
            modal_area: Rect::default(),
            remote: None,
            remote_settings,
            term_bg: ratatui::style::Color::Reset,
            config,
        };
        app.restore_session();
        app
    }

    pub fn run(mut self) -> io::Result<()> {
        if self.remote_settings.autostart
            && let Err(e) = self.start_remote()
        {
            // The window retries and shows the error.
            crate::log!("remote: {e}");
            self.toggle_remote_modal();
        }
        let mut terminal = ratatui::init();
        execute!(io::stdout(), EnableMouseCapture)?;
        let result = self.event_loop(&mut terminal);
        self.look.t = theme::THEMES[0];
        self.sync_term_bg();
        let _ = execute!(io::stdout(), DisableMouseCapture);
        ratatui::restore();
        self.cancel_sleep();
        self.stop_shuffle();
        self.save_session();
        self.remote = None;
        result
    }

    fn event_loop(&mut self, terminal: &mut DefaultTerminal) -> io::Result<()> {
        while !self.quit {
            self.tick(Instant::now());
            self.poll_search();
            self.poll_remote();
            self.sync_term_bg();
            let now = SystemTime::now();
            self.card.update(self.player.snapshot(), now);
            self.card.advance(now, &self.player);
            terminal.draw(|f| {
                let area = f.area();
                self.draw(f.buffer_mut(), area, now);
            })?;
            let busy = self.card.animating(now, &self.extras()) || self.searching();
            let wait = if busy {
                FRAME_INTERVAL
            } else if self.remote.is_some() {
                REMOTE_INTERVAL
            } else {
                IDLE_INTERVAL
            };
            if event::poll(wait)? {
                // Handle everything queued before the next frame.
                loop {
                    match event::read()? {
                        Event::Key(k) if k.kind != KeyEventKind::Release => self.on_key(k),
                        Event::Mouse(m) => self.on_mouse(m),
                        _ => {}
                    }
                    if self.quit || !event::poll(Duration::ZERO)? {
                        break;
                    }
                }
            }
        }
        Ok(())
    }

    fn extras(&self) -> Extras {
        let now = Instant::now();
        Extras {
            bookmarked: self.bookmarks.has(&self.player.snapshot().url),
            shuffle: (self.shuffle.active && !self.shuffle.interval.is_zero())
                .then(|| (self.shuffle.remaining(now), self.shuffle.interval)),
            sleep: self.sleep.active().then(|| (self.sleep.remaining(now), self.sleep.fade())),
        }
    }

    // Lists

    fn wants_bookmarks_row(&self) -> bool {
        !self.bookmarks.is_empty() || self.tag.as_ref().is_some_and(|t| t.name == BOOKMARKS_TAG)
    }

    fn tag_rows(&self) -> Vec<TagRef> {
        let mut rows = Vec::new();
        if self.wants_bookmarks_row() {
            rows.push(TagRef::new(BOOKMARKS_TAG));
        }
        rows.extend(self.tags.iter().map(|t| TagRef::new(t)));
        if let Some(last) = &self.last_search {
            rows.push(TagRef::search(&last.query));
        }
        let filter = &self.tags_state.filter;
        rows.retain(|r| matches(filter, &r.name));
        rows
    }

    fn station_rows(&self) -> Vec<StationRow> {
        let filter = &self.stations_state.filter;
        let stations = self.listed.iter().enumerate().filter(|(_, s)| matches(filter, &s.title));
        let mut rows = Vec::new();
        if filter.is_empty() {
            if self.tag.is_some() && !self.wide {
                rows.push(StationRow::Back);
            }
            if !self.listed.is_empty() {
                rows.push(StationRow::Random);
            }
        }
        rows.extend(stations.map(|(i, _)| StationRow::Station(i)));
        rows
    }

    fn stations_for_tag(&self, tag: Option<&TagRef>) -> Vec<Station> {
        if tag.is_some_and(|t| t.search) {
            return self.last_search.as_ref().map(|l| l.stations.clone()).unwrap_or_default();
        }
        match tag.map(|t| t.name.as_str()) {
            None | Some(ALL_STATIONS_TAG) => self.stations.clone(),
            Some(BOOKMARKS_TAG) => self.bookmarks.list(),
            Some(name) => self.stations.iter().filter(|s| s.tags.iter().any(|t| t == name)).cloned().collect(),
        }
    }

    /// Shows the tag's stations without switching pages; the cursor goes to
    /// the station playing.
    fn load_tag(&mut self, tag: TagRef) {
        self.listed = self.stations_for_tag(Some(&tag));
        self.tag = Some(tag);
        self.stations_state = ListState::default();
        self.select_station(&self.player.snapshot().url);
    }

    fn open_tag(&mut self, tag: TagRef) {
        self.load_tag(tag);
        self.tags_state.filter.clear();
        self.show(Page::Main);
    }

    /// Rebuilds the stations list, keeping the cursor.
    fn reload_stations(&mut self) {
        self.listed = self.stations_for_tag(self.tag.as_ref());
        let len = self.station_rows().len();
        self.stations_state.clamp(len);
    }

    fn select_station(&mut self, url: &str) -> bool {
        if url.is_empty() {
            return false;
        }
        let found =
            self.station_rows().iter().position(|r| matches!(r, StationRow::Station(i) if self.listed[*i].url == url));
        if let Some(i) = found {
            self.stations_state.cursor = i;
        }
        found.is_some()
    }

    fn show(&mut self, page: Page) {
        if page == Page::Help && self.page != Page::Help {
            self.help_from = self.page;
            self.help_scroll = 0;
        }
        self.page = page;
        if self.wide && page == Page::Tags && self.tag.is_none() {
            self.preview_tag_at_cursor();
        } else if self.wide && matches!(page, Page::Tags | Page::Main) {
            self.sync_tag_cursor();
        }
    }

    fn preview_tag_at_cursor(&mut self) {
        if let Some(tag) = self.tag_rows().get(self.tags_state.cursor).cloned()
            && self.tag.as_ref() != Some(&tag)
        {
            self.load_tag(tag);
        }
    }

    fn sync_tag_cursor(&mut self) {
        let tag = self.tag.clone().unwrap_or_else(|| TagRef::new(ALL_STATIONS_TAG));
        if let Some(i) = self.tag_rows().iter().position(|t| *t == tag) {
            self.tags_state.cursor = i;
        }
    }

    // Playback

    /// Plays the station, or stops it when it is the one playing.
    fn toggle_play(&mut self, station: Station) {
        if station.url != self.player.snapshot().url {
            self.playing = Some((station.clone(), self.tag.clone()));
        } else {
            self.cancel_sleep();
        }
        self.player.play(&station.title, &station.url);
    }

    /// toggle_play for a station the user picked, which ends the shuffle.
    fn toggle_play_manual(&mut self, station: Station) {
        self.stop_shuffle();
        self.toggle_play(station);
    }

    fn stop(&mut self) {
        self.cancel_sleep();
        self.stop_shuffle();
        self.player.stop();
    }

    /// Plays a random station of the rows shown, avoiding the one playing
    /// when there is another.
    fn play_random(&mut self) -> Option<String> {
        let rows = self.station_rows();
        let choices: Vec<(usize, usize)> = rows
            .iter()
            .enumerate()
            .filter_map(|(row, r)| match r {
                StationRow::Station(i) => Some((row, *i)),
                _ => None,
            })
            .collect();
        if choices.is_empty() {
            return None;
        }
        let current = self.player.snapshot().url;
        let mut rng = rand::rng();
        let mut pick = choices[rng.random_range(0..choices.len())];
        while choices.len() > 1 && self.listed[pick.1].url == current {
            pick = choices[rng.random_range(0..choices.len())];
        }
        self.stations_state.cursor = pick.0;
        let station = self.listed[pick.1].clone();
        let url = station.url.clone();
        self.toggle_play(station);
        Some(url)
    }

    fn play_bookmark(&mut self, n: usize) {
        if let Some(s) = self.bookmarks.list().into_iter().nth(n) {
            self.toggle_play_manual(s);
        }
    }

    fn change_volume(&mut self, delta: i32) {
        // Flashes the gauge even when the volume is at its limit.
        self.card.flash(SystemTime::now());
        self.player.change_volume(delta);
    }

    /// Bookmarks the station under the cursor, else the one playing, or
    /// removes its bookmark.
    fn toggle_bookmark(&mut self) {
        let under_cursor = match (self.page, self.station_rows().get(self.stations_state.cursor)) {
            _ if self.modal.is_some() => self.search_cursor_station(),
            (Page::Main, Some(StationRow::Station(i))) => Some(self.listed[*i].clone()),
            _ => None,
        };
        let url = self.player.snapshot().url;
        let target = under_cursor
            .or_else(|| self.playing.as_ref().filter(|(s, _)| !url.is_empty() && s.url == url).map(|(s, _)| s.clone()));
        if let Some(s) = target {
            self.bookmarks.toggle(&s);
            if self.tag.as_ref().is_some_and(|t| t.name == BOOKMARKS_TAG) {
                self.reload_stations();
            }
        }
    }

    // Input

    fn focused_state(&mut self) -> &mut ListState {
        if self.page == Page::Tags { &mut self.tags_state } else { &mut self.stations_state }
    }

    fn filter_changed(&mut self) {
        match self.page {
            Page::Tags => {
                self.tags_state.cursor = 0;
                self.tags_state.offset = 0;
                if self.wide {
                    self.preview_tag_at_cursor();
                }
            }
            _ => {
                self.stations_state.cursor = 0;
                self.stations_state.offset = 0;
            }
        }
    }

    fn on_key(&mut self, k: KeyEvent) {
        let ctrl = k.modifiers.contains(KeyModifiers::CONTROL);
        if ctrl && k.code == KeyCode::Char('c') {
            self.quit = true;
            return;
        }
        match self.modal {
            Some(Modal::Theme(_)) => return self.on_theme_key(k),
            Some(Modal::Search(_)) => return self.on_search_key(k),
            Some(Modal::Remote(_)) => {
                match k.code {
                    KeyCode::Esc => self.modal = None,
                    KeyCode::Char('p') if ctrl => self.modal = None,
                    KeyCode::Left => self.change_volume(-VOLUME_STEP),
                    KeyCode::Right => self.change_volume(VOLUME_STEP),
                    _ => {}
                }
                return;
            }
            None => {}
        }
        match k.code {
            KeyCode::Char('t') if ctrl => self.show_theme_modal(),
            KeyCode::Char('f') if ctrl => self.show_search_modal(false),
            KeyCode::Char('s') if ctrl => self.show_search_modal(true),
            KeyCode::Char('p') if ctrl => self.toggle_remote_modal(),
            KeyCode::Char('b') if ctrl => self.toggle_bookmark(),
            KeyCode::Char('r') if ctrl => self.toggle_shuffle(Instant::now()),
            KeyCode::Char('z') if ctrl => self.cycle_sleep(Instant::now()),
            KeyCode::Char(c @ '1'..='9') if k.modifiers.contains(KeyModifiers::ALT) => {
                self.set_shuffle_interval(c as u64 - '0' as u64, Instant::now())
            }
            KeyCode::Char(_) if ctrl || k.modifiers.contains(KeyModifiers::ALT) => {}
            KeyCode::Left => self.change_volume(-VOLUME_STEP),
            KeyCode::Right => self.change_volume(VOLUME_STEP),
            _ if self.page == Page::Help => self.on_help_key(k),
            KeyCode::Esc => self.escape(),
            KeyCode::Tab | KeyCode::BackTab if self.wide => {
                self.show(if self.page == Page::Tags { Page::Main } else { Page::Tags })
            }
            KeyCode::Up => self.move_cursor(-1),
            KeyCode::Down => self.move_cursor(1),
            KeyCode::PageUp => {
                let page = self.focused_state().page();
                self.move_cursor(-page)
            }
            KeyCode::PageDown => {
                let page = self.focused_state().page();
                self.move_cursor(page)
            }
            KeyCode::Home => self.move_cursor(isize::MIN / 2),
            KeyCode::End => self.move_cursor(isize::MAX / 2),
            KeyCode::Enter => self.activate(),
            KeyCode::Backspace => {
                if self.focused_state().filter.pop().is_some() {
                    self.filter_changed();
                }
            }
            KeyCode::Char(c) => self.on_char(c),
            _ => {}
        }
    }

    fn on_help_key(&mut self, k: KeyEvent) {
        match k.code {
            KeyCode::Esc | KeyCode::Char('?') => self.show(self.help_from),
            KeyCode::Up => self.help_scroll = self.help_scroll.saturating_sub(1),
            KeyCode::Down => self.help_scroll += 1,
            KeyCode::PageUp => self.help_scroll = self.help_scroll.saturating_sub(10),
            KeyCode::PageDown => self.help_scroll += 10,
            _ => {}
        }
    }

    /// Letters start a filter of the focused list; while one is typed, every
    /// character goes to it. Otherwise digits and symbols are commands.
    fn on_char(&mut self, c: char) {
        if !self.focused_state().filter.is_empty() {
            self.focused_state().filter.push(c);
            self.filter_changed();
            return;
        }
        match c {
            ' ' => self.activate(),
            '+' | '=' => self.change_volume(VOLUME_STEP),
            '-' | '_' => self.change_volume(-VOLUME_STEP),
            '*' => {
                self.stop_shuffle();
                self.play_random();
            }
            '?' => self.show(Page::Help),
            ':' => self.show_search_modal(false),
            '/' | '#' => self.show(Page::Tags),
            '1'..='9' => self.play_bookmark(c as usize - '1' as usize),
            c if c.is_alphabetic() => {
                self.focused_state().filter.push(c);
                self.filter_changed();
            }
            _ => {}
        }
    }

    fn escape(&mut self) {
        if !self.focused_state().filter.is_empty() {
            self.focused_state().filter.clear();
            if self.page == Page::Tags {
                self.sync_tag_cursor();
            } else {
                let url = self.player.snapshot().url;
                self.select_station(&url);
            }
            return;
        }
        match self.page {
            Page::Tags => self.quit = true,
            _ => {
                if !self.wide {
                    self.tag = None;
                }
                self.show(Page::Tags);
            }
        }
    }

    fn move_cursor(&mut self, delta: isize) {
        if self.page == Page::Tags {
            let len = self.tag_rows().len();
            self.tags_state.move_by(delta, len);
            if self.wide {
                self.preview_tag_at_cursor();
            }
        } else {
            let len = self.station_rows().len();
            self.stations_state.move_by(delta, len);
        }
    }

    fn activate(&mut self) {
        if self.page == Page::Tags {
            if let Some(tag) = self.tag_rows().get(self.tags_state.cursor).cloned() {
                self.open_tag(tag);
            }
            return;
        }
        match self.station_rows().get(self.stations_state.cursor) {
            Some(StationRow::Back) => {
                self.tag = None;
                self.show(Page::Tags);
            }
            Some(StationRow::Random) => {
                self.stop_shuffle();
                self.play_random();
            }
            Some(&StationRow::Station(i)) => self.toggle_play_manual(self.listed[i].clone()),
            None => {}
        }
    }

    fn on_mouse(&mut self, m: MouseEvent) {
        match self.modal {
            Some(Modal::Theme(_)) => return self.on_theme_mouse(m),
            Some(Modal::Search(_)) => return self.on_search_mouse(m),
            Some(Modal::Remote(_)) => return,
            None => {}
        }
        let pos = Position::new(m.column, m.row);
        if self.card.area.contains(pos) {
            match m.kind {
                MouseEventKind::ScrollUp => self.change_volume(VOLUME_STEP),
                MouseEventKind::ScrollDown => self.change_volume(-VOLUME_STEP),
                MouseEventKind::Down(MouseButton::Left) => {
                    if self.sleep.active() && self.card.on_sleep_label(m.column, m.row) {
                        self.cancel_sleep();
                        self.card.flash_sleep(SystemTime::now());
                    }
                    if let Some(volume) = self.card.volume_at(m.column, m.row) {
                        self.card.flash(SystemTime::now());
                        self.player.set_volume(volume);
                    }
                }
                _ => {}
            }
            return;
        }
        let page = if self.tags_area.contains(pos) {
            Page::Tags
        } else if self.stations_area.contains(pos) {
            Page::Main
        } else {
            return;
        };
        let delta = match m.kind {
            MouseEventKind::ScrollUp => -1,
            MouseEventKind::ScrollDown => 1,
            MouseEventKind::Down(MouseButton::Left) => 0,
            _ => return,
        };
        if page == Page::Tags {
            let len = self.tag_rows().len();
            if delta != 0 {
                self.tags_state.scroll(delta, len);
                if self.wide {
                    self.preview_tag_at_cursor();
                }
            } else if let Some(i) = self.tags_state.row_at(m.row, len) {
                self.page = Page::Tags;
                self.tags_state.cursor = i;
                self.activate();
            }
        } else {
            let len = self.station_rows().len();
            if delta != 0 {
                self.stations_state.scroll(delta, len);
            } else if let Some(i) = self.stations_state.row_at(m.row, len) {
                self.show(Page::Main);
                self.stations_state.cursor = i;
                self.activate();
            }
        }
    }

    // Drawing

    fn draw(&mut self, buf: &mut Buffer, area: Rect, now: SystemTime) {
        let wide = area.width >= WIDE_MIN_WIDTH;
        if wide != self.wide || !self.layout_ready {
            self.layout_ready = true;
            self.wide = wide;
            self.reload_stations();
            if wide {
                if self.page == Page::Tags || self.tag.is_none() {
                    self.preview_tag_at_cursor();
                } else {
                    self.sync_tag_cursor();
                }
            }
        }
        if self.look.t.paints_bg() {
            buf.set_style(area, self.look.t.text());
        }

        let card_h = card::HEIGHT.min(area.height);
        let hint_h = 1.min(area.height - card_h);
        let body = Rect { height: area.height - card_h - hint_h, ..area };
        let card_area = Rect { y: body.bottom(), height: card_h, ..area };
        let hint_area = Rect { y: card_area.bottom(), height: hint_h, ..area };

        self.tags_area = Rect::default();
        self.stations_area = Rect::default();
        match self.page {
            Page::Help => help::draw(&self.look, buf, body, &mut self.help_scroll),
            _ if self.wide => {
                let tags_w = (area.width / 4).clamp(TAGS_PANE_MIN_WIDTH, TAGS_PANE_MAX_WIDTH);
                self.tags_area = Rect { width: tags_w, ..body };
                self.stations_area = Rect { x: body.x + tags_w, width: body.width - tags_w, ..body };
            }
            Page::Tags => self.tags_area = body,
            Page::Main => self.stations_area = body,
        }
        if !self.tags_area.is_empty() {
            self.draw_tags(buf, now);
        }
        if !self.stations_area.is_empty() {
            self.draw_stations(buf, now);
        }

        let extras = self.extras();
        self.card.draw(&self.look, buf, card_area, now, &extras);
        if hint_h > 0 {
            let hints = self.hints();
            let segs = hint_segs(&self.look, &hints, hint_area.width.saturating_sub(1) as usize);
            draw_segs(buf, hint_area.x + 1, hint_area.y, hint_area.width.saturating_sub(1) as usize, &segs);
        }
        match self.modal {
            Some(Modal::Theme(_)) => self.draw_theme_modal(buf, body),
            Some(Modal::Search(_)) => self.draw_search_modal(buf, area),
            Some(Modal::Remote(_)) => self.draw_remote_modal(buf, area),
            None => {}
        }
    }

    fn draw_tags(&mut self, buf: &mut Buffer, _now: SystemTime) {
        let t = self.look.t;
        let rows: Vec<Row> = self
            .tag_rows()
            .into_iter()
            .map(|tag| {
                let mut label = vec![seg(tag.name.as_str(), t.text())];
                let count = if tag.search {
                    let last = self.last_search.as_ref();
                    if last.is_some_and(|l| l.online) {
                        label.push(seg(" (online)", t.dim()));
                    }
                    last.map_or(0, |l| l.stations.len())
                } else if tag.name == BOOKMARKS_TAG {
                    self.bookmarks.list().len()
                } else {
                    self.tag_counts.get(&tag.name).copied().unwrap_or(0)
                };
                Row { label, aside: count.to_string(), ..Row::default() }
            })
            .collect();
        let pane = Pane {
            title: vec![seg(" Tags ", t.text())],
            focused: self.page == Page::Tags,
            rows: &rows,
            empty: Some(seg("No match", t.dim())),
        };
        list::draw(&self.look, buf, self.tags_area, &pane, &mut self.tags_state);
    }

    fn draw_stations(&mut self, buf: &mut Buffer, now: SystemTime) {
        let (t, g) = (self.look.t, self.look.g);
        let (playing_url, state) = self.card.playing_url();
        let playing_url = playing_url.to_string();
        let rows: Vec<Row> = self
            .station_rows()
            .into_iter()
            .map(|r| match r {
                StationRow::Back => {
                    let name = self.tag.as_ref().map_or("", |t| t.name.as_str());
                    Row { label: vec![seg(format!("{} {name}", g.back), t.accent())], ..Row::default() }
                }
                StationRow::Random => Row { indent: 2, label: vec![seg("Random", t.text())], ..Row::default() },
                StationRow::Station(i) => {
                    let s = &self.listed[i];
                    let mut label = vec![seg(s.title.as_str(), t.text())];
                    if self.bookmarks.has(&s.url) {
                        label.push(seg(format!(" {}", g.star), t.dim()));
                    }
                    let (mark, tint) = if !playing_url.is_empty() && s.url == playing_url {
                        let (glyph, color) = match state {
                            State::Buffering => (g.spinner_frame(now), t.warn),
                            State::Failed | State::Unsupported => (g.fail, t.danger),
                            _ => (g.play, t.accent),
                        };
                        (seg(glyph, t.fg(color)), Some(color))
                    } else {
                        (seg(" ", t.text()), None)
                    };
                    Row { mark: Some(mark), label, tint, ..Row::default() }
                }
            })
            .collect();

        let title =
            vec![seg(" ", t.text()), seg(g.notes, t.fg(t.song)), seg(format!(" {} ", self.stations_title()), t.text())];
        let empty = if !self.stations_state.filter.is_empty() {
            "No match".to_string()
        } else if self.tag.as_ref().is_some_and(|t| t.name == BOOKMARKS_TAG) {
            format!("No bookmarks yet {} press Ctrl+B on a station to add it", g.dot)
        } else {
            "No stations".to_string()
        };
        let pane = Pane { title, focused: self.page == Page::Main, rows: &rows, empty: Some(seg(empty, t.dim())) };
        list::draw(&self.look, buf, self.stations_area, &pane, &mut self.stations_state);
    }

    fn stations_title(&self) -> String {
        let count = self.listed.len();
        let mut name = self.tag.as_ref().map_or(ALL_STATIONS_TAG.to_string(), |t| t.name.clone());
        if self.tag.as_ref().is_some_and(|t| t.search) && self.last_search.as_ref().is_some_and(|l| l.online) {
            name.push_str(" (online)");
        }
        let unit = if count == 1 { "station" } else { "stations" };
        format!("{name} {} {count} {unit}", self.look.g.dot)
    }

    fn hints(&self) -> Vec<(&'static str, &'static str)> {
        match self.modal {
            Some(Modal::Theme(_)) => return vec![("↑ ↓", "preview"), ("enter", "apply"), ("esc", "cancel")],
            Some(Modal::Search(_)) => return self.search_hints(),
            Some(Modal::Remote(_)) => return vec![("esc", "close")],
            None => {}
        }
        let filtering = match self.page {
            Page::Tags => !self.tags_state.filter.is_empty(),
            Page::Main => !self.stations_state.filter.is_empty(),
            Page::Help => false,
        };
        match self.page {
            Page::Help => vec![("esc", "back"), ("↑ ↓", "scroll")],
            Page::Tags if filtering => vec![("enter", "open"), ("↑ ↓", "move"), ("esc", "clear filter")],
            Page::Main if filtering => vec![("enter", "play"), ("↑ ↓", "move"), ("esc", "clear filter")],
            Page::Main => {
                let mut hints = vec![
                    ("a-z", "filter"),
                    ("^B", "bookmark"),
                    ("^F", "search"),
                    ("^R", "shuffle"),
                    ("^Z", "sleep"),
                    ("^P", "phone"),
                    ("^T", "theme"),
                    ("?", "help"),
                ];
                if !self.wide {
                    hints.push(("esc", "tags"));
                }
                hints
            }
            Page::Tags => vec![
                ("a-z", "filter"),
                ("^F", "search"),
                ("^R", "shuffle"),
                ("^Z", "sleep"),
                ("^P", "phone"),
                ("^T", "theme"),
                ("?", "help"),
                ("esc", "quit"),
            ],
        }
    }
}

/// Keys as keycaps on the cursor's surface; the terminal theme, which has
/// none, keeps them bold.
fn hint_segs(look: &Look, hints: &[(&str, &str)], w: usize) -> Vec<Seg> {
    let t = &look.t;
    let keycaps = t.surface != ratatui::style::Color::Reset;
    let gap = if keycaps { "  " } else { "   " };
    let mut out = Vec::new();
    let mut used = 0;
    for (i, (key, label)) in hints.iter().enumerate() {
        let key = if look.g.ascii { key.replace('↑', "^").replace('↓', "v") } else { key.to_string() };
        let key = if keycaps { seg(format!(" {key} "), t.key().bg(t.surface)) } else { seg(key, t.key()) };
        let part = [key, seg(format!(" {label}"), t.dim())];
        let mut pw = segs_width(&part);
        if i > 0 {
            pw += gap.len();
        }
        if used + pw > w {
            break;
        }
        if i > 0 {
            out.push(seg(gap, t.text()));
        }
        out.extend(part);
        used += pw;
    }
    out
}

/// A box with rounded corners, or ASCII ones, and a title from x + 1.
pub fn draw_box(look: &Look, buf: &mut Buffer, area: Rect, style: Style, title: &[Seg]) {
    if area.width < 2 || area.height < 2 {
        return;
    }
    let [tl, tr, bl, br, h, v] = look.g.border;
    let inner = area.width as usize - 2;
    let line = h.repeat(inner);
    draw_segs(buf, area.x, area.y, area.width as usize, &[seg(tl, style), seg(line.as_str(), style), seg(tr, style)]);
    draw_segs(
        buf,
        area.x,
        area.bottom() - 1,
        area.width as usize,
        &[seg(bl, style), seg(line.as_str(), style), seg(br, style)],
    );
    for y in area.y + 1..area.bottom() - 1 {
        draw_segs(buf, area.x, y, 1, &[seg(v, style)]);
        draw_segs(buf, area.right() - 1, y, 1, &[seg(v, style)]);
    }
    draw_segs(buf, area.x + 1, area.y, inner, title);
}

#[cfg(test)]
pub(crate) mod tests {
    use super::*;
    use std::path::PathBuf;

    /// An app on the built-in stations with an offline player, its config and
    /// bookmarks in a directory of its own.
    pub fn test_app_with(config_text: &str) -> (App, PathBuf) {
        use std::sync::atomic::{AtomicUsize, Ordering};
        static N: AtomicUsize = AtomicUsize::new(0);
        let dir = std::env::temp_dir().join(format!(
            "goradion-app-{}-{}",
            std::process::id(),
            N.fetch_add(1, Ordering::Relaxed)
        ));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        if !config_text.is_empty() {
            std::fs::write(dir.join("config.yaml"), config_text).unwrap();
        }
        let stations = crate::stations::load("").unwrap();
        let bookmarks = Bookmarks::load_from(dir.join("bookmarks.json"), &stations);
        let config = Config::load_from(dir.join("config.yaml"));
        let opts = Options { ascii: false, vu: true, remote: None, remote_port: None };
        (App::new(Player::offline(), stations, bookmarks, config, &opts), dir)
    }

    pub fn test_app() -> (App, PathBuf) {
        test_app_with("")
    }
}
