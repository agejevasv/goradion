use std::io::Read;

use crate::audio::http;
use crate::log;

const BUILT_IN: &str = include_str!("../stations.csv");

#[derive(Clone, Debug, PartialEq)]
pub struct Station {
    pub title: String,
    pub url: String,
    pub tags: Vec<String>,
}

/// Reads the stations CSV from a file or an http(s) URL; an empty source means
/// the built-in list, which also stands in for a URL that can't be fetched.
pub fn load(source: &str) -> Result<Vec<Station>, String> {
    if source.is_empty() {
        return parse(BUILT_IN.as_bytes());
    }
    if source.starts_with("http://") || source.starts_with("https://") {
        let mut body = Vec::new();
        match http::get(source, &[]).and_then(|mut r| r.body.read_to_end(&mut body)) {
            Ok(_) => return parse(&body[..]),
            Err(e) => {
                log!("stations: {e}, using the built-in list");
                return parse(BUILT_IN.as_bytes());
            }
        }
    }
    let file = std::fs::File::open(source).map_err(|e| format!("{source}: {e}"))?;
    parse(file)
}

/// Reads "title,url[,tag;tag...]" rows and skips rows without a URL.
fn parse(r: impl Read) -> Result<Vec<Station>, String> {
    let mut reader = csv::ReaderBuilder::new().has_headers(false).flexible(true).from_reader(r);
    let mut stations = Vec::new();
    for rec in reader.records() {
        let rec = rec.map_err(|e| format!("can't parse stations CSV: {e}"))?;
        let url = rec.get(1).unwrap_or("").trim();
        if url.is_empty() {
            continue;
        }
        let tags =
            rec.get(2).unwrap_or("").split(';').map(str::trim).filter(|t| !t.is_empty()).map(String::from).collect();
        stations.push(Station { title: rec[0].trim().to_string(), url: url.to_string(), tags });
    }
    Ok(stations)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_rows() {
        let csv = "A,http://a,Jazz; Lounge\n\"B, the station\",http://b\nno url\nC,,Rock\n";
        let got = parse(csv.as_bytes()).unwrap();
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].tags, ["Jazz", "Lounge"]);
        assert_eq!(got[1].title, "B, the station");
        assert!(got[1].tags.is_empty());
    }

    #[test]
    fn built_in_list() {
        assert!(load("").unwrap().len() > 100);
    }
}
