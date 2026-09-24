//! The phone remote on the app's side: the API actions, which run on the UI
//! loop, and the Ctrl+P window with the QR code.

use std::time::Instant;

use qrcode::{EcLevel, QrCode};
use ratatui::buffer::Buffer;
use ratatui::layout::Rect;
use ratatui::style::{Color, Modifier, Style};

use super::text::{Seg, draw_segs, fill, seg, width};
use super::{ALL_STATIONS_TAG, App, BOOKMARKS_TAG, Modal, Page, TagRef};
use crate::log;
use crate::remote::{
    Action, ActionRequest, ApiError, PlayerView, Server, ShuffleView, SleepView, State, StationView, Tag,
};

/// The QR spec asks for a quiet zone of 4 modules; 2 keeps the code within an
/// 80x24 terminal and phone scanners cope with it.
const QR_QUIET_ZONE: usize = 2;
const TEXT_WIDTH: usize = 40;

pub struct RemoteSettings {
    pub autostart: bool,
    pub port: u16,
    /// None: a random code per run.
    pub key: Option<String>,
}

impl App {
    /// Idempotent: a running server is kept.
    pub(super) fn start_remote(&mut self) -> Result<(), String> {
        if self.remote.is_some() {
            return Ok(());
        }
        let (server, calls) =
            Server::start(self.remote_settings.port, self.remote_settings.key.as_deref(), self.stations.clone())
                .map_err(|e| e.to_string())?;
        self.remote = Some((server, calls));
        Ok(())
    }

    pub(super) fn toggle_remote_modal(&mut self) {
        if matches!(self.modal, Some(Modal::Remote(_))) {
            self.modal = None;
            return;
        }
        let error = self.start_remote().err();
        if let Some(e) = &error {
            log!("remote: {e}");
        }
        self.modal = Some(Modal::Remote(error));
    }

    /// Answers the phone's calls queued since the last loop.
    pub(super) fn poll_remote(&mut self) {
        loop {
            let Some((_, calls)) = &self.remote else { return };
            let Ok(call) = calls.try_recv() else { return };
            let result = self.remote_action(call.action, &call.req).map(|_| self.remote_state());
            let _ = call.reply.send(result);
        }
    }

    fn remote_action(&mut self, action: Action, q: &ActionRequest) -> Result<(), ApiError> {
        let now = Instant::now();
        match action {
            Action::State => {}
            Action::Tags => {
                self.modal_close_for_remote();
                self.tag = None;
                self.show(Page::Tags);
            }
            Action::Tag => {
                let tag = TagRef { name: q.tag.clone(), search: q.search };
                let empty_bookmarks = tag == TagRef::new(BOOKMARKS_TAG) && self.bookmarks.is_empty();
                if empty_bookmarks || tag == TagRef::new(ALL_STATIONS_TAG) || !self.tag_exists(&tag) {
                    return Err(ApiError::not_found(format!("tag {:?} not found", q.tag)));
                }
                self.open_tag(tag);
            }
            Action::Play => {
                let i = self.listed.iter().position(|s| s.url == q.url);
                let i = i.ok_or_else(|| ApiError::not_found("station not found in the current list"))?;
                let station = self.listed[i].clone();
                self.select_station(&station.url);
                self.toggle_play_manual(station);
            }
            Action::Bookmark => {
                let listed = self.listed.iter().find(|s| !q.url.is_empty() && s.url == q.url).cloned();
                let playing_url = self.player.snapshot().url;
                let playing = self.playing.as_ref().map(|(s, _)| s.clone()).filter(|s| {
                    !playing_url.is_empty() && s.url == playing_url && (q.url.is_empty() || q.url == s.url)
                });
                let station = listed.or(playing).ok_or_else(|| ApiError::not_found("station not found"))?;
                self.bookmarks.toggle(&station);
                self.reload_stations();
            }
            Action::Stop => self.stop(),
            Action::Random => {
                self.stop_shuffle();
                self.play_random().ok_or_else(|| ApiError::bad("no stations to pick from"))?;
            }
            Action::Volume => {
                let volume = q.volume.ok_or_else(|| ApiError::bad("volume required"))?;
                self.player.set_volume(volume);
            }
            Action::Shuffle => match q.minutes {
                0 => self.toggle_shuffle(now),
                m @ 1..=9 => self.set_shuffle_interval(m as u64, now),
                _ => return Err(ApiError::bad("minutes must be 1-9")),
            },
            Action::Sleep => {
                if !self.sleep.active() && self.player.snapshot().url.is_empty() {
                    return Err(ApiError::bad("nothing playing"));
                }
                self.cycle_sleep(now);
            }
            Action::SelectSearch(last) => {
                if last.query.is_empty() || last.query != q.query || last.online != q.online {
                    return Err(ApiError::bad("search results are stale, search again"));
                }
                let selected = last.stations.iter().find(|s| s.url == q.url).cloned();
                let selected = selected.ok_or_else(|| ApiError::not_found("station not found in search results"))?;
                self.open_search(&last.query, last.stations, last.online);
                self.select_station(&selected.url);
                self.toggle_play_manual(selected);
            }
        }
        Ok(())
    }

    /// A search or theme window in the TUI would hide what the phone changed.
    fn modal_close_for_remote(&mut self) {
        if matches!(self.modal, Some(Modal::Search(_))) {
            self.modal = None;
        }
    }

    /// What the TUI shows.
    pub(super) fn remote_state(&self) -> State {
        let now = Instant::now();
        let inf = self.player.snapshot();
        let mut tags = Vec::new();
        if !self.bookmarks.is_empty() {
            tags.push(Tag { name: BOOKMARKS_TAG.into(), kind: "bookmarks".into(), online: false });
        }
        tags.extend(self.tags.iter().map(|t| Tag { name: t.clone(), ..Tag::default() }));
        if let Some(last) = &self.last_search {
            tags.push(Tag { name: last.query.clone(), kind: "search".into(), online: last.online });
        }
        let stations_page = self.page == Page::Main;
        let stations = if stations_page {
            self.listed
                .iter()
                .map(|s| StationView {
                    title: s.title.clone(),
                    url: s.url.clone(),
                    meta: String::new(),
                    playing: s.url == inf.url,
                    bookmarked: self.bookmarks.has(&s.url),
                })
                .collect()
        } else {
            Vec::new()
        };
        State {
            version: crate::version_string(),
            page: if stations_page { "stations" } else { "tags" }.into(),
            tag: self.tag.as_ref().map(|t| t.name.clone()).unwrap_or_default(),
            tags,
            stations,
            player: PlayerView {
                status: inf.status.clone(),
                station: inf.station.clone(),
                song: inf.song.clone(),
                url: inf.url.clone(),
                volume: inf.volume,
                bitrate: inf.bitrate,
                bookmarked: self.bookmarks.has(&inf.url),
            },
            shuffle: ShuffleView {
                active: self.shuffle.active,
                remaining: if self.shuffle.active { self.shuffle.remaining(now).as_secs() } else { 0 },
                interval: self.shuffle.interval.as_secs() / 60,
            },
            sleep: SleepView {
                active: self.sleep.active(),
                remaining: if self.sleep.active() { self.sleep.remaining(now).as_secs() } else { 0 },
            },
        }
    }

    pub(super) fn draw_remote_modal(&mut self, buf: &mut Buffer, screen: Rect) {
        let t = self.look.t;
        let Some(Modal::Remote(error)) = &self.modal else { return };
        let bold = Modifier::BOLD;
        let mut lines: Vec<Vec<Seg>> = Vec::new();
        let mut qr = Vec::new();
        match (error, &self.remote) {
            (Some(_), _) | (None, None) => {
                let e = error.clone().unwrap_or_default();
                lines.push(vec![seg("Could not start the remote control server:", t.fg(t.danger))]);
                lines.push(Vec::new());
                lines.push(vec![seg(e, t.text())]);
            }
            (None, Some((server, _))) => {
                qr = qr_lines(&server.url()).unwrap_or_default();
                lines.push(vec![seg("Control goradion from your phone", t.accent().add_modifier(bold))]);
                lines.push(Vec::new());
                lines.push(vec![seg("Scan the QR code with the phone camera, or open", t.text())]);
                lines.push(Vec::new());
                lines.push(vec![seg(format!("  {}", server.address()), t.fg(t.warn))]);
                lines.push(Vec::new());
                if server.key().is_empty() {
                    lines.push(vec![
                        seg("No access code is set:", t.fg(t.danger)),
                        seg(" anyone on the network can control goradion.", t.text()),
                    ]);
                } else {
                    lines.push(vec![seg("and enter the code", t.text())]);
                    lines.push(Vec::new());
                    lines.push(vec![seg(format!("  {}", server.key()), t.fg(t.warn).add_modifier(bold))]);
                }
                lines.push(Vec::new());
                lines.push(vec![seg("The phone must be on the same network.", t.text())]);
                lines.push(Vec::new());
                lines.push(vec![seg("Esc closes this window.", t.dim())]);
            }
        }
        let lines: Vec<Vec<Seg>> = lines.into_iter().flat_map(|l| wrap(l, TEXT_WIDTH)).collect();

        let qr_w = qr.first().map_or(0, |l| width(l)) as u16;
        let qr_h = qr.len() as u16;
        let width = if qr_w > 0 { qr_w + 2 + TEXT_WIDTH as u16 + 4 } else { TEXT_WIDTH as u16 + 4 };
        let height = (qr_h.max(lines.len() as u16).max(12) + 2).min(screen.height);
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
        super::draw_box(&self.look, buf, area, t.dim(), &[seg(" Remote control ", t.text())]);
        let inner_h = area.height.saturating_sub(2) as usize;
        // Explicit black on white, so the code scans on any theme.
        let qr_style = Style::new().fg(Color::Rgb(0, 0, 0)).bg(Color::Rgb(255, 255, 255));
        for (i, line) in qr.iter().enumerate().take(inner_h) {
            draw_segs(buf, area.x + 2, area.y + 1 + i as u16, qr_w as usize, &[seg(line.as_str(), qr_style)]);
        }
        let text_x = if qr_w > 0 { area.x + 2 + qr_w + 2 } else { area.x + 2 };
        let text_w = area.right().saturating_sub(text_x + 2) as usize;
        for (i, line) in lines.iter().enumerate().take(inner_h) {
            draw_segs(buf, text_x, area.y + 1 + i as u16, text_w, line);
        }
    }
}

/// Wraps a line of segments at spaces to width cells.
fn wrap(line: Vec<Seg>, w: usize) -> Vec<Vec<Seg>> {
    let mut out = vec![Vec::new()];
    let mut used = 0;
    for s in line {
        for word in s.text.split_inclusive(' ') {
            let ww = width(word.trim_end());
            if used > 0 && used + ww > w {
                out.push(Vec::new());
                used = 0;
            }
            out.last_mut().unwrap().push(seg(word, s.style));
            used += width(word);
        }
    }
    out
}

/// A QR code as half blocks, two modules per text row.
pub fn qr_lines(content: &str) -> Option<Vec<String>> {
    let code = QrCode::with_error_correction_level(content, EcLevel::L).ok()?;
    let size = code.width();
    let colors = code.to_colors();
    let w = size + 2 * QR_QUIET_ZONE;
    let dark = |x: usize, y: usize| {
        let (x, y) = (x.checked_sub(QR_QUIET_ZONE)?, y.checked_sub(QR_QUIET_ZONE)?);
        (x < size && y < size).then(|| colors[y * size + x] == qrcode::Color::Dark)
    };
    let dark = |x, y| dark(x, y).unwrap_or(false);
    Some(
        (0..w)
            .step_by(2)
            .map(|y| {
                (0..w)
                    .map(|x| match (dark(x, y), dark(x, y + 1)) {
                        (true, true) => '█',
                        (true, false) => '▀',
                        (false, true) => '▄',
                        (false, false) => ' ',
                    })
                    .collect()
            })
            .collect(),
    )
}

#[cfg(test)]
mod tests {
    use super::super::tests::test_app;
    use super::*;
    use std::io::{Read, Write};
    use std::sync::{Arc, Mutex};

    #[test]
    fn qr_fits_small_terminal() {
        let lines = qr_lines("http://192.168.100.100:7373/?k=abc123").unwrap();
        let w = width(&lines[0]);
        assert_eq!(lines.len(), w.div_ceil(2));
        assert!(lines.iter().all(|l| width(l) == w));
        assert!(w <= 40 && lines.len() <= 20, "{w}x{}", lines.len());
    }

    /// Sends a request and returns the status and body.
    fn request(port: u16, method: &str, path: &str, key: &str, body: &str) -> (u16, String) {
        let mut s = std::net::TcpStream::connect(("127.0.0.1", port)).unwrap();
        write!(
            s,
            "{method} {path} HTTP/1.1\r\nHost: x\r\nX-Key: {key}\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
            body.len()
        )
        .unwrap();
        let mut resp = String::new();
        s.read_to_string(&mut resp).unwrap();
        let status = resp.split_whitespace().nth(1).unwrap().parse().unwrap();
        let body = resp.split_once("\r\n\r\n").map_or("", |(_, b)| b).to_string();
        (status, body)
    }

    fn state(port: u16, method: &str, path: &str, key: &str, body: &str) -> (u16, State) {
        let (status, body) = request(port, method, path, key, body);
        let st = if status == 200 { serde_json::from_str(&body).unwrap() } else { State::default() };
        (status, st)
    }

    #[test]
    fn remote_control() {
        let (mut a, _dir) = test_app();
        a.remote_settings.port = 0;
        a.toggle_remote_modal();
        assert!(matches!(a.modal, Some(Modal::Remote(None))));
        let (port, key) = {
            let (server, _) = a.remote.as_ref().unwrap();
            (server.port, server.key().to_string())
        };
        assert_eq!(key.len(), crate::remote::TOKEN_LENGTH);
        a.toggle_remote_modal();
        assert!(a.modal.is_none());
        a.toggle_remote_modal();
        assert_eq!(a.remote.as_ref().unwrap().0.port, port, "Ctrl+P must not start a second server");
        a.modal = None;
        let first_tag = a.tags[0].clone();

        // The UI loop, answering calls.
        let app = Arc::new(Mutex::new(a));
        let running = Arc::new(std::sync::atomic::AtomicBool::new(true));
        let pump = {
            let (app, running) = (app.clone(), running.clone());
            std::thread::spawn(move || {
                while running.load(std::sync::atomic::Ordering::Relaxed) {
                    app.lock().unwrap().poll_remote();
                    std::thread::sleep(std::time::Duration::from_millis(2));
                }
            })
        };

        for (path, k, want) in [
            (format!("/?k={key}"), "", 200),
            ("/".to_string(), "", 200),
            ("/api/state".to_string(), key.as_str(), 200),
            ("/api/state".to_string(), &*format!(" {}", key.to_uppercase()), 200),
            ("/api/state".to_string(), "", 403),
            ("/api/state?k=wrong".to_string(), "", 403),
        ] {
            let (status, body) = request(port, "GET", &path, k, "");
            assert_eq!(status, want, "GET {path} key={k:?}");
            if want == 200 && path.starts_with("/?") {
                assert!(body.contains("goradion"));
            }
        }

        let call = |method: &str, path: &str, body: &str| state(port, method, path, &key, body);
        let (_, st) = call("GET", "/api/state", "");
        assert_eq!((st.page.as_str(), st.tags[0].name.as_str()), ("tags", first_tag.as_str()));
        assert_ne!(call("POST", "/api/tag", r#"{"tag":"All Stations"}"#).0, 200);

        let (code, st) = call("POST", "/api/tag", r#"{"tag":"Jazz"}"#);
        assert_eq!((code, st.page.as_str(), st.tag.as_str()), (200, "stations", "Jazz"));
        assert!(!st.stations.is_empty());
        assert_eq!(app.lock().unwrap().tag, Some(TagRef::new("Jazz")));
        assert_eq!(call("POST", "/api/tag", r#"{"tag":"Nope"}"#).0, 404);

        let target = st.stations[1].clone();
        let (code, st) = call("POST", "/api/play", &format!(r#"{{"url":"{}"}}"#, target.url));
        assert_eq!((code, st.player.url.as_str()), (200, target.url.as_str()));
        assert!(st.stations[1].playing && !st.stations[0].playing);
        assert_eq!(call("POST", "/api/play", r#"{"url":"http://nope"}"#).0, 404);

        let (_, st) = call("GET", "/api/state", "");
        assert_ne!(st.tags[0].kind, "bookmarks", "bookmarks tag shown with no bookmarks");
        let (code, st) = call("POST", "/api/bookmark", "");
        assert!(code == 200 && st.player.bookmarked && st.stations[1].bookmarked && !st.stations[0].bookmarked);
        let (code, st) = call("POST", "/api/bookmark", &format!(r#"{{"url":"{}"}}"#, st.stations[0].url));
        assert!(code == 200 && st.stations[0].bookmarked);
        assert_eq!(call("POST", "/api/bookmark", r#"{"url":"http://nope"}"#).0, 404);
        assert_eq!(st.tags[0].kind, "bookmarks");
        let (code, st) = call("POST", "/api/tag", r#"{"tag":"Bookmarks"}"#);
        assert!(
            code == 200 && st.stations.len() == 2 && st.stations[0].title == target.title && st.stations[0].playing
        );
        let (code, st) = call("POST", "/api/bookmark", &format!(r#"{{"url":"{}"}}"#, st.stations[1].url));
        assert!(code == 200 && st.stations.len() == 1);

        let (_, st) = call("POST", "/api/stop", "");
        assert_eq!((st.player.url.as_str(), st.player.status.as_str()), ("", "Stopped"));
        let (_, st) = call("POST", "/api/play", &format!(r#"{{"url":"{}"}}"#, target.url));
        assert_eq!(st.player.url, target.url);

        assert_eq!(call("POST", "/api/volume", r#"{"volume":40}"#).1.player.volume, 40);
        assert_eq!(call("POST", "/api/volume", r#"{"volume":150}"#).1.player.volume, 100);
        assert_eq!(call("POST", "/api/volume", "{}").0, 400);

        call("POST", "/api/tag", r#"{"tag":"Jazz"}"#);
        let (_, st) = call("POST", "/api/random", "");
        assert!(!st.player.url.is_empty() && st.player.url != target.url);

        let (_, st) = call("POST", "/api/shuffle", r#"{"minutes":3}"#);
        assert!(st.shuffle.interval == 3 && !st.shuffle.active);
        let (_, st) = call("POST", "/api/shuffle", "");
        assert!(st.shuffle.active && st.shuffle.remaining > 0 && st.shuffle.remaining <= 180);
        assert!(!call("POST", "/api/shuffle", "").1.shuffle.active);
        assert_eq!(call("POST", "/api/shuffle", r#"{"minutes":12}"#).0, 400);

        // Stop first: the random pick above may be the station selected below.
        call("POST", "/api/stop", "");
        let (_, body) = request(port, "GET", &format!("/api/search?q=soma+jazz&k={key}"), "", "");
        let sr: serde_json::Value = serde_json::from_str(&body).unwrap();
        let results = sr["results"].as_array().unwrap();
        assert!(!results.is_empty() && sr["online"] == false);
        let url = results[0]["url"].as_str().unwrap();
        let (code, st) =
            call("POST", "/api/search/select", &format!(r#"{{"query":"soma jazz","online":false,"url":"{url}"}}"#));
        assert!(code == 200 && st.tag == "soma jazz" && st.player.url == url && st.stations.len() == results.len());
        let stale = format!(r#"{{"query":"other","online":false,"url":"{url}"}}"#);
        assert_eq!(call("POST", "/api/search/select", &stale).0, 400);
        let (_, st) = call("GET", "/api/state", "");
        let last = st.tags.last().unwrap();
        assert!(last.kind == "search" && last.name == "soma jazz" && !last.online);

        let (_, st) = call("POST", "/api/tags", "");
        assert_eq!(st.page, "tags");
        assert_eq!(app.lock().unwrap().page, Page::Tags);

        running.store(false, std::sync::atomic::Ordering::Relaxed);
        pump.join().unwrap();
        app.lock().unwrap().remote = None;
        assert!(std::net::TcpStream::connect(("127.0.0.1", port)).is_err(), "the server must stop");
    }

    #[test]
    fn startup_key() {
        for (key, good, bad) in [
            (
                Some("Correct Horse [battery] staple"),
                vec!["Correct Horse [battery] staple", "correct horse [battery] staple"],
                vec!["", "correct"],
            ),
            (Some(""), vec!["", "anything"], vec![]),
        ] {
            let (server, _rx) = Server::start(0, key, Vec::new()).unwrap();
            // Nobody answers the calls, but authorization comes first.
            for k in bad {
                assert_eq!(request(server.port, "GET", "/api/search?q=x", k, "").0, 403, "{k:?}");
            }
            for k in good {
                assert_eq!(request(server.port, "GET", "/api/search?q=x", k, "").0, 200, "{k:?}");
            }
        }
    }
}
