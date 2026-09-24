//! The phone remote's HTTP server. Requests that read or change the app are
//! sent to the UI loop as Calls and answered with the resulting state.

use std::collections::HashMap;
use std::io::Read;
use std::net::UdpSocket;
use std::sync::mpsc::{self, Receiver, Sender};
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::Duration;

use serde::{Deserialize, Serialize};
use tiny_http::{Header, Method, Request, Response};

use crate::log;
use crate::radiobrowser;
use crate::stations::Station;

const PAGE: &str = include_str!("../../internal/radio/remote.html");
const TOKEN_ALPHABET: &[u8] = b"abcdefghijkmnpqrstuvwxyz23456789";
pub const TOKEN_LENGTH: usize = 6;
const MAX_REQUEST_BYTES: u64 = 64 << 10;
const REPLY_TIMEOUT: Duration = Duration::from_secs(10);

#[derive(Debug)]
pub enum Action {
    State,
    Tags,
    Tag,
    Play,
    Bookmark,
    Stop,
    Random,
    Volume,
    Shuffle,
    Sleep,
    /// Picks a result of the last search served, which it carries.
    SelectSearch(LastSearch),
}

#[derive(Deserialize, Default, Debug)]
#[serde(default)]
pub struct ActionRequest {
    pub tag: String,
    /// The tag is the last search.
    pub search: bool,
    pub url: String,
    pub volume: Option<i32>,
    pub minutes: i64,
    pub query: String,
    pub online: bool,
}

pub struct Call {
    pub action: Action,
    pub req: ActionRequest,
    pub reply: Sender<Result<State, ApiError>>,
}

#[derive(Debug)]
pub struct ApiError {
    pub status: u16,
    pub message: String,
}

impl ApiError {
    pub fn bad(message: impl Into<String>) -> Self {
        ApiError { status: 400, message: message.into() }
    }

    pub fn not_found(message: impl Into<String>) -> Self {
        ApiError { status: 404, message: message.into() }
    }
}

#[derive(Serialize, Deserialize, Default, Debug, Clone)]
pub struct Tag {
    pub name: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub kind: String,
    #[serde(default, skip_serializing_if = "std::ops::Not::not")]
    pub online: bool,
}

#[derive(Serialize, Deserialize, Default, Debug, Clone)]
pub struct StationView {
    pub title: String,
    pub url: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub meta: String,
    #[serde(default)]
    pub playing: bool,
    #[serde(default)]
    pub bookmarked: bool,
}

#[derive(Serialize, Deserialize, Default, Debug, Clone)]
pub struct PlayerView {
    pub status: String,
    pub station: String,
    pub song: String,
    pub url: String,
    pub volume: i32,
    pub bitrate: u32,
    pub bookmarked: bool,
}

#[derive(Serialize, Deserialize, Default, Debug, Clone)]
pub struct ShuffleView {
    pub active: bool,
    pub remaining: u64,
    pub interval: u64,
}

#[derive(Serialize, Deserialize, Default, Debug, Clone)]
pub struct SleepView {
    pub active: bool,
    pub remaining: u64,
}

#[derive(Serialize, Deserialize, Default, Debug, Clone)]
pub struct State {
    pub version: String,
    pub page: String,
    pub tag: String,
    pub tags: Vec<Tag>,
    pub stations: Vec<StationView>,
    pub player: PlayerView,
    pub shuffle: ShuffleView,
    pub sleep: SleepView,
}

#[derive(Clone, Debug, Default)]
pub struct LastSearch {
    pub query: String,
    pub online: bool,
    pub stations: Vec<Station>,
}

struct Shared {
    /// Empty: no access code is asked for.
    token: String,
    stations: Vec<Station>,
    calls: Mutex<Sender<Call>>,
    last_search: Mutex<LastSearch>,
}

/// Lives until dropped, which stops it.
pub struct Server {
    http: Arc<tiny_http::Server>,
    shared: Arc<Shared>,
    pub port: u16,
    pub ip: String,
    thread: Option<thread::JoinHandle<()>>,
}

impl Server {
    /// Listens on all interfaces at port, or a free port when it is taken. A
    /// None key makes a random code.
    pub fn start(port: u16, key: Option<&str>, stations: Vec<Station>) -> std::io::Result<(Server, Receiver<Call>)> {
        let http = tiny_http::Server::http(("0.0.0.0", port))
            .or_else(|e| {
                log!("remote: port {port} unavailable ({e}), picking a free one");
                tiny_http::Server::http(("0.0.0.0", 0))
            })
            .map_err(std::io::Error::other)?;
        let port = http.server_addr().to_ip().map_or(port, |a| a.port());
        let token = key.map_or_else(|| new_token(TOKEN_LENGTH), |k| k.trim().to_string());
        let (tx, rx) = mpsc::channel();
        let shared =
            Arc::new(Shared { token, stations, calls: Mutex::new(tx), last_search: Mutex::new(LastSearch::default()) });
        let http = Arc::new(http);
        let (serve_http, serve_shared) = (http.clone(), shared.clone());
        let thread = thread::Builder::new().name("remote".into()).spawn(move || {
            for req in serve_http.incoming_requests() {
                let shared = serve_shared.clone();
                let _ = thread::Builder::new().name("remote-request".into()).spawn(move || handle(req, &shared));
            }
        })?;
        let server = Server { http, shared, port, ip: lan_ip(), thread: Some(thread) };
        log!("remote: listening on {}", server.url());
        Ok((server, rx))
    }

    /// What a user types into the phone browser.
    pub fn address(&self) -> String {
        format!("http://{}:{}", self.ip, self.port)
    }

    /// What the QR code carries, key included.
    pub fn url(&self) -> String {
        if self.shared.token.is_empty() {
            return self.address();
        }
        let key: String = url::form_urlencoded::byte_serialize(self.shared.token.as_bytes()).collect();
        format!("{}/?k={key}", self.address())
    }

    pub fn key(&self) -> &str {
        &self.shared.token
    }
}

impl Drop for Server {
    fn drop(&mut self) {
        self.http.unblock();
        if let Some(t) = self.thread.take() {
            let _ = t.join();
        }
        log!("remote: server stopped");
    }
}

fn handle(req: Request, shared: &Shared) {
    let method = req.method().clone();
    let url = url::Url::parse(&format!("http://localhost{}", req.url())).ok();
    let path = url.as_ref().map_or("/".to_string(), |u| u.path().to_string());
    let query: HashMap<String, String> = url.map(|u| u.query_pairs().into_owned().collect()).unwrap_or_default();

    let response = match (&method, path.as_str()) {
        (Method::Get, "/") => html(PAGE),
        (_, "/favicon.ico") => Response::from_string("").with_status_code(204),
        (_, p) if p.starts_with("/api/") => {
            if !authorized(shared, &req, &query) {
                text(403, "Forbidden: wrong access code. The code is shown by goradion when you press Ctrl+P.")
            } else {
                api(req, shared, method, &path, &query);
                return;
            }
        }
        _ => text(404, "404 page not found"),
    };
    let _ = req.respond(response);
}

/// The key comes as the "k" query parameter (QR link) or the X-Key header
/// (page script). Case is ignored: phone keyboards capitalize.
fn authorized(shared: &Shared, req: &Request, query: &HashMap<String, String>) -> bool {
    if shared.token.is_empty() {
        return true;
    }
    let header = req.headers().iter().find(|h| h.field.equiv("X-Key")).map(|h| h.value.as_str().to_string());
    let key = query.get("k").filter(|k| !k.is_empty()).cloned().or(header).unwrap_or_default();
    constant_time_eq(key.trim().to_lowercase().as_bytes(), shared.token.to_lowercase().as_bytes())
}

fn constant_time_eq(a: &[u8], b: &[u8]) -> bool {
    a.len() == b.len() && a.iter().zip(b).fold(0u8, |acc, (x, y)| acc | (x ^ y)) == 0
}

fn api(mut req: Request, shared: &Shared, method: Method, path: &str, query: &HashMap<String, String>) {
    let action = match (method, path) {
        (Method::Get, "/api/state") => Action::State,
        (Method::Get, "/api/search") => {
            let _ = req.respond(search(shared, query));
            return;
        }
        (Method::Post, "/api/tags") => Action::Tags,
        (Method::Post, "/api/tag") => Action::Tag,
        (Method::Post, "/api/play") => Action::Play,
        (Method::Post, "/api/bookmark") => Action::Bookmark,
        (Method::Post, "/api/stop") => Action::Stop,
        (Method::Post, "/api/random") => Action::Random,
        (Method::Post, "/api/volume") => Action::Volume,
        (Method::Post, "/api/shuffle") => Action::Shuffle,
        (Method::Post, "/api/sleep") => Action::Sleep,
        (Method::Post, "/api/search/select") => Action::SelectSearch(shared.last_search.lock().unwrap().clone()),
        _ => {
            let _ = req.respond(text(404, "404 page not found"));
            return;
        }
    };
    let mut body = String::new();
    if let Err(e) = req.as_reader().take(MAX_REQUEST_BYTES).read_to_string(&mut body) {
        let _ = req.respond(text(400, &format!("bad request: {e}")));
        return;
    }
    let parsed = if body.trim().is_empty() { Ok(ActionRequest::default()) } else { serde_json::from_str(&body) };
    let action_req = match parsed {
        Ok(r) => r,
        Err(e) => {
            let _ = req.respond(text(400, &format!("bad request: {e}")));
            return;
        }
    };

    let (tx, rx) = mpsc::channel();
    let sent = shared.calls.lock().unwrap().send(Call { action, req: action_req, reply: tx }).is_ok();
    let response = match rx.recv_timeout(REPLY_TIMEOUT) {
        Ok(Ok(state)) if sent => json(&state),
        Ok(Err(e)) => text(e.status, &e.message),
        _ => text(503, "goradion is exiting"),
    };
    let _ = req.respond(response);
}

fn search(shared: &Shared, query: &HashMap<String, String>) -> Response<std::io::Cursor<Vec<u8>>> {
    let q = query.get("q").map_or("", |s| s.trim()).to_string();
    let online = query.get("online").is_some_and(|v| v == "1");
    let (stations, results): (Vec<Station>, Vec<StationView>) = if !q.is_empty() && online {
        match radiobrowser::search(&q) {
            Ok(found) => found
                .into_iter()
                .map(|f| {
                    let meta = crate::ui::online_meta(&f);
                    let view = StationView {
                        title: f.station.title.clone(),
                        url: f.station.url.clone(),
                        meta,
                        ..StationView::default()
                    };
                    (f.station, view)
                })
                .unzip(),
            Err(e) => return text(502, &e),
        }
    } else {
        crate::ui::search_stations(&shared.stations, &q)
            .into_iter()
            .map(|s| {
                let view = StationView { title: s.title.clone(), url: s.url.clone(), ..StationView::default() };
                (s, view)
            })
            .unzip()
    };
    *shared.last_search.lock().unwrap() = LastSearch { query: q.clone(), online, stations };
    json(&serde_json::json!({ "query": q, "online": online, "results": results }))
}

fn header(name: &str, value: &str) -> Header {
    Header::from_bytes(name.as_bytes(), value.as_bytes()).expect("valid header")
}

fn html(body: &str) -> Response<std::io::Cursor<Vec<u8>>> {
    Response::from_string(body).with_header(header("Content-Type", "text/html; charset=utf-8"))
}

fn json(v: &impl Serialize) -> Response<std::io::Cursor<Vec<u8>>> {
    let body = serde_json::to_string(v).unwrap_or_else(|_| "{}".into());
    Response::from_string(body)
        .with_header(header("Content-Type", "application/json"))
        .with_header(header("Cache-Control", "no-store"))
}

fn text(status: u16, message: &str) -> Response<std::io::Cursor<Vec<u8>>> {
    Response::from_string(format!("{message}\n"))
        .with_status_code(status)
        .with_header(header("Content-Type", "text/plain; charset=utf-8"))
}

pub fn new_token(n: usize) -> String {
    use rand::RngExt;
    let mut rng = rand::rng();
    (0..n).map(|_| TOKEN_ALPHABET[rng.random_range(0..TOKEN_ALPHABET.len())] as char).collect()
}

/// The default-route IPv4 address, else loopback. Connecting a UDP socket
/// only resolves the route; nothing is sent.
pub fn lan_ip() -> String {
    UdpSocket::bind("0.0.0.0:0")
        .and_then(|s| s.connect("8.8.8.8:80").map(|_| s))
        .and_then(|s| s.local_addr())
        .ok()
        .map(|a| a.ip())
        .filter(|ip| !ip.is_loopback() && !ip.is_unspecified())
        .map_or_else(|| "127.0.0.1".to_string(), |ip| ip.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn tokens() {
        let mut seen = std::collections::HashSet::new();
        for _ in 0..50 {
            let t = new_token(TOKEN_LENGTH);
            assert_eq!(t.len(), TOKEN_LENGTH);
            assert!(seen.insert(t));
        }
        assert!(!lan_ip().is_empty());
    }

    #[test]
    fn key_in_url() {
        let (server, _rx) = Server::start(0, Some("Correct Horse [battery] staple"), Vec::new()).unwrap();
        assert!(server.url().ends_with("/?k=Correct+Horse+%5Bbattery%5D+staple"), "{}", server.url());
        let (server, _rx) = Server::start(0, Some(""), Vec::new()).unwrap();
        assert_eq!(server.url(), server.address());
    }
}
