//! Station search on radio-browser.info.

use std::io::Read;

use serde::Deserialize;

use crate::audio::http;
use crate::stations::Station;

const MAX_NAME_LEN: usize = 50;
const FALLBACK_SERVER: &str = "de1.api.radio-browser.info";

#[derive(Clone, Debug, PartialEq)]
pub struct OnlineStation {
    pub station: Station,
    pub country_code: String,
    pub bitrate: u32,
}

#[derive(Deserialize, Default)]
#[serde(default)]
struct Row {
    name: String,
    url: String,
    url_resolved: String,
    countrycode: String,
    tags: String,
    bitrate: u32,
}

#[derive(Deserialize)]
struct Server {
    name: String,
}

fn get_json<T: serde::de::DeserializeOwned>(url: &str) -> Result<T, String> {
    let mut resp = http::get(url, &[("Accept", "application/json")]).map_err(|e| e.to_string())?;
    let mut body = Vec::new();
    resp.body.read_to_end(&mut body).map_err(|e| e.to_string())?;
    serde_json::from_slice(&body).map_err(|e| e.to_string())
}

/// A random mirror, as radio-browser asks clients to spread the load.
fn server() -> String {
    use rand::seq::IndexedRandom;
    get_json::<Vec<Server>>("https://all.api.radio-browser.info/json/servers")
        .ok()
        .and_then(|servers| servers.choose(&mut rand::rng()).map(|s| s.name.clone()))
        .unwrap_or_else(|| FALLBACK_SERVER.to_string())
}

pub fn search(query: &str) -> Result<Vec<OnlineStation>, String> {
    let q: String = url::form_urlencoded::Serializer::new(String::new())
        .append_pair("name", query)
        .append_pair("hidebroken", "true")
        .append_pair("order", "clickcount")
        .append_pair("reverse", "true")
        .append_pair("limit", "50")
        .finish();
    let rows: Vec<Row> = get_json(&format!("https://{}/json/stations/search?{q}", server()))?;
    Ok(parse(rows))
}

/// Drops rows without a usable name or URL, and repeated stream URLs: the
/// database holds duplicate entries for the same stream. Rows arrive sorted by
/// click count, so the first one wins.
fn parse(rows: Vec<Row>) -> Vec<OnlineStation> {
    let mut seen = std::collections::HashSet::new();
    let mut out = Vec::new();
    for r in rows {
        let url = if r.url_resolved.is_empty() { r.url } else { r.url_resolved };
        if url.is_empty() || seen.contains(&url) {
            continue;
        }
        let title = clean_name(&r.name);
        if title.is_empty() {
            continue;
        }
        seen.insert(url.clone());
        let tags = r.tags.split(',').map(str::trim).filter(|t| !t.is_empty()).map(String::from).collect();
        out.push(OnlineStation { station: Station { title, url, tags }, country_code: r.countrycode, bitrate: r.bitrate });
    }
    out
}

fn clean_name(name: &str) -> String {
    let mut name = name.trim();
    for sep in [" - ", " | ", " || ", " · ", " – ", " — ", " -> "] {
        if let Some(i) = name.find(sep).filter(|&i| i >= 10) {
            name = &name[..i];
            break;
        }
    }
    let kept: String = name.chars().filter(|c| c.is_ascii_alphanumeric() || ".,'`\"&/ ".contains(*c)).collect();
    let mut name = kept.split_whitespace().collect::<Vec<_>>().join(" ");
    name = name.trim_end_matches([',', '.', ' ']).to_string();
    if name.len() > MAX_NAME_LEN {
        name.truncate(MAX_NAME_LEN);
        if let Some(i) = name.rfind(' ').filter(|&i| i > MAX_NAME_LEN / 2) {
            name.truncate(i);
        }
        name = name.trim_end_matches([',', '.', ' ']).to_string();
    }
    name
}

#[cfg(test)]
mod tests {
    use super::*;

    fn row(name: &str, url: &str, resolved: &str, tags: &str, bitrate: u32) -> Row {
        Row { name: name.into(), url: url.into(), url_resolved: resolved.into(), tags: tags.into(), bitrate, ..Row::default() }
    }

    #[test]
    fn dedupes() {
        let got = parse(vec![
            row("Top Bachata Radio", "https://x/topbachata", "https://x/topbachata", "", 128),
            row("Top Bachata Radio", "https://x/topbachata", "https://x/topbachata", "", 64),
            row("Bachata Mix", "https://x/mix", "", "bachata, latin", 0),
            row("", "https://x/noname", "", "", 0),
            row("No URL", "", "", "", 0),
            row("Same stream, other name", "https://x/mix", "", "", 0),
        ]);
        assert_eq!(got.len(), 2, "{got:?}");
        assert_eq!((got[0].station.title.as_str(), got[0].bitrate), ("Top Bachata Radio", 128));
        assert_eq!((got[1].station.url.as_str(), got[1].station.tags.len()), ("https://x/mix", 2));
    }

    #[test]
    fn names() {
        assert_eq!(clean_name("  Radio Paradise - Main Mix (320k) "), "Radio Paradise");
        assert_eq!(clean_name("Jazz ★ FM!!"), "Jazz FM");
        assert_eq!(clean_name("A - B"), "A B");
        let long = "Word ".repeat(20);
        assert!(clean_name(&long).len() <= MAX_NAME_LEN);
    }
}
