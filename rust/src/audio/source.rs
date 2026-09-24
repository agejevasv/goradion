//! Opens a station URL: follows playlists and strips ICY metadata.

use std::io::{self, BufRead, Read};

use super::http;

const MAX_PLAYLIST: u64 = 64 * 1024;
const MAX_PLAYLIST_DEPTH: usize = 3;

pub type TitleFn = Box<dyn FnMut(String) + Send>;

pub struct Source {
    pub reader: Box<dyn Read + Send>,
    pub mime: Option<String>,
    /// `icy-br`, in kbit/s.
    pub bitrate: Option<u32>,
}

#[derive(Debug)]
pub enum OpenError {
    Io(io::Error),
    Unsupported(&'static str),
}

impl std::fmt::Display for OpenError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            OpenError::Io(e) => e.fmt(f),
            OpenError::Unsupported(what) => f.write_str(what),
        }
    }
}

impl From<io::Error> for OpenError {
    fn from(e: io::Error) -> Self {
        OpenError::Io(e)
    }
}

pub fn open(url: &str, on_title: TitleFn) -> Result<Source, OpenError> {
    let mut url = url.to_string();
    for _ in 0..MAX_PLAYLIST_DEPTH {
        let mut resp = http::get(&url, &[("Icy-MetaData", "1")])?;
        let mime = resp.header("content-type").map(|v| v.split(';').next().unwrap_or("").trim().to_ascii_lowercase());

        if is_playlist(mime.as_deref(), resp.body.fill_buf()?) {
            let mut text = String::new();
            let mut raw = Vec::new();
            (&mut resp.body).take(MAX_PLAYLIST).read_to_end(&mut raw)?;
            text.push_str(&String::from_utf8_lossy(&raw));
            if text.contains("#EXT-X-") {
                return Err(OpenError::Unsupported("HLS streams are not supported yet"));
            }
            let entry = first_entry(&text).ok_or(OpenError::Unsupported("empty playlist"))?;
            url = resp.url.join(&entry).map(String::from).unwrap_or(entry);
            continue;
        }

        let metaint = resp.header("icy-metaint").and_then(|v| v.parse::<usize>().ok());
        let bitrate = resp.header("icy-br").and_then(parse_icy_bitrate);
        let reader: Box<dyn Read + Send> = match metaint {
            Some(n) if n > 0 => Box::new(IcyReader::new(resp.body, n, on_title)),
            _ => Box::new(resp.body),
        };
        return Ok(Source { reader, mime, bitrate });
    }
    Err(OpenError::Unsupported("playlist nested too deeply"))
}

fn is_playlist(mime: Option<&str>, head: &[u8]) -> bool {
    const TYPES: [&str; 7] = [
        "audio/x-scpls",
        "audio/scpls",
        "application/pls+xml",
        "audio/x-mpegurl",
        "audio/mpegurl",
        "application/x-mpegurl",
        "application/vnd.apple.mpegurl",
    ];
    if mime.is_some_and(|m| TYPES.contains(&m)) {
        return true;
    }
    let head = String::from_utf8_lossy(&head[..head.len().min(64)]);
    let head = head.trim_start_matches('\u{feff}').trim_start().to_ascii_lowercase();
    ["[playlist]", "#extm3u", "http://", "https://"].iter().any(|p| head.starts_with(p))
}

/// The first stream of a PLS or M3U playlist.
fn first_entry(text: &str) -> Option<String> {
    let lines = text.lines().map(str::trim);
    for line in lines.clone() {
        if let Some((key, value)) = line.split_once('=')
            && key.to_ascii_lowercase().starts_with("file")
            && !value.trim().is_empty()
        {
            return Some(value.trim().to_string());
        }
    }
    lines
        .filter(|l| !l.is_empty() && !l.starts_with('#') && !l.starts_with('['))
        .find(|l| !l.contains('=') || l.contains("://"))
        .map(String::from)
}

/// `icy-br` is sometimes a list such as "128,128".
fn parse_icy_bitrate(v: &str) -> Option<u32> {
    v.split(',').next()?.trim().parse().ok().filter(|&br| br > 0)
}

/// Removes the metadata blocks that the server puts after every `metaint`
/// bytes of audio, and reports their stream titles.
struct IcyReader<R> {
    inner: R,
    metaint: usize,
    until_meta: usize,
    on_title: TitleFn,
}

impl<R: Read> IcyReader<R> {
    fn new(inner: R, metaint: usize, on_title: TitleFn) -> Self {
        IcyReader { inner, metaint, until_meta: metaint, on_title }
    }
}

impl<R: Read> Read for IcyReader<R> {
    fn read(&mut self, buf: &mut [u8]) -> io::Result<usize> {
        if self.until_meta == 0 {
            let mut len = [0u8];
            self.inner.read_exact(&mut len)?;
            let mut meta = vec![0; len[0] as usize * 16];
            self.inner.read_exact(&mut meta)?;
            if let Some(title) = stream_title(&meta) {
                (self.on_title)(title);
            }
            self.until_meta = self.metaint;
        }
        let n = buf.len().min(self.until_meta);
        let n = self.inner.read(&mut buf[..n])?;
        self.until_meta -= n;
        Ok(n)
    }
}

fn stream_title(meta: &[u8]) -> Option<String> {
    let end = meta.iter().rposition(|&b| b != 0).map_or(0, |i| i + 1);
    let text = decode_text(&meta[..end]);
    let start = text.find("StreamTitle='")? + "StreamTitle='".len();
    let rest = &text[start..];
    let end = rest.find("';").or_else(|| rest.rfind('\''))?;
    Some(rest[..end].trim().to_string())
}

/// UTF-8, or else Latin-1, which older servers send.
pub fn decode_text(bytes: &[u8]) -> String {
    match std::str::from_utf8(bytes) {
        Ok(s) => s.to_string(),
        Err(_) => bytes.iter().map(|&b| b as char).collect(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::{Arc, Mutex};

    #[test]
    fn playlists() {
        let pls = "[playlist]\nNumberOfEntries=2\nFile1=http://a/stream\nTitle1=A\nFile2=http://b\n";
        assert_eq!(first_entry(pls).as_deref(), Some("http://a/stream"));
        let m3u = "#EXTM3U\n#EXTINF:-1,A\n\nhttps://a/b.mp3\n";
        assert_eq!(first_entry(m3u).as_deref(), Some("https://a/b.mp3"));
        assert!(is_playlist(Some("text/plain"), b"[playlist]\n"));
        assert!(is_playlist(Some("audio/x-mpegurl"), b""));
        assert!(!is_playlist(Some("audio/mpeg"), b"\xff\xfb\x90"));
    }

    #[test]
    fn icy_blocks_are_stripped() {
        let mut raw = b"abcd".to_vec();
        let meta = b"StreamTitle='Artist - Song';StreamUrl='';";
        raw.push(meta.len().div_ceil(16) as u8);
        raw.extend_from_slice(meta);
        raw.resize(raw.len() + (16 - meta.len() % 16) % 16, 0);
        raw.extend_from_slice(b"efgh");
        raw.push(0);
        raw.extend_from_slice(b"ij");

        let titles = Arc::new(Mutex::new(Vec::new()));
        let sink = titles.clone();
        let mut r = IcyReader::new(&raw[..], 4, Box::new(move |t| sink.lock().unwrap().push(t)));
        let mut audio = String::new();
        r.read_to_string(&mut audio).unwrap();
        assert_eq!(audio, "abcdefghij");
        assert_eq!(*titles.lock().unwrap(), vec!["Artist - Song".to_string()]);
    }

    #[test]
    fn latin1_titles() {
        assert_eq!(stream_title(b"StreamTitle='Caf\xe9';\0\0").as_deref(), Some("Café"));
    }
}
