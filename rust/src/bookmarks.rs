//! The stations the user marked, in the order added. They may come from the
//! stations list or from an online search.

use std::collections::HashMap;
use std::path::PathBuf;

use serde::{Deserialize, Serialize};

use crate::files;
use crate::log;
use crate::stations::Station;

/// Fields may be missing from a hand-edited file, as the Go version allowed.
#[derive(Serialize, Deserialize, Clone, Default)]
#[serde(default)]
struct Bookmark {
    url: String,
    title: String,
}

pub struct Bookmarks {
    path: PathBuf,
    items: Vec<Bookmark>,
    /// The stations list by URL.
    known: HashMap<String, Station>,
}

impl Bookmarks {
    pub fn load(stations: &[Station]) -> Self {
        Self::load_from(files::config_dir().join("bookmarks.json"), stations)
    }

    pub fn load_from(path: PathBuf, stations: &[Station]) -> Self {
        let known = stations.iter().map(|s| (s.url.clone(), s.clone())).collect();
        let mut items: Vec<Bookmark> = match std::fs::read(&path) {
            Ok(data) => serde_json::from_slice(&data).unwrap_or_else(|e| {
                log!("bookmarks: {e}");
                Vec::new()
            }),
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => Vec::new(),
            Err(e) => {
                log!("bookmarks: {e}");
                Vec::new()
            }
        };
        items.retain(|b| !b.url.is_empty());
        Bookmarks { path, items, known }
    }

    pub fn has(&self, url: &str) -> bool {
        !url.is_empty() && self.items.iter().any(|b| b.url == url)
    }

    /// Adds the station, or removes it when bookmarked, and saves the file.
    /// Reports whether the station is bookmarked now.
    pub fn toggle(&mut self, s: &Station) -> bool {
        if s.url.is_empty() {
            return false;
        }
        let n = self.items.len();
        self.items.retain(|b| b.url != s.url);
        let added = self.items.len() == n;
        if added {
            self.items.push(Bookmark { url: s.url.clone(), title: s.title.clone() });
        }
        self.save();
        added
    }

    fn save(&self) {
        let result = serde_json::to_vec_pretty(&self.items)
            .map_err(std::io::Error::other)
            .and_then(|data| files::write_atomic(&self.path, &data));
        if let Err(e) = result {
            log!("bookmarks: {e}");
        }
    }

    /// Those in the stations list show its title, which may have changed
    /// since they were added.
    pub fn list(&self) -> Vec<Station> {
        self.items
            .iter()
            .map(|b| {
                self.known.get(&b.url).cloned().unwrap_or_else(|| Station {
                    title: b.title.clone(),
                    url: b.url.clone(),
                    tags: Vec::new(),
                })
            })
            .collect()
    }

    pub fn is_empty(&self) -> bool {
        self.items.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn toggle_and_reload() {
        let dir = std::env::temp_dir().join(format!("goradion-bm-{}", std::process::id()));
        let path = dir.join("bookmarks.json");
        let a = Station { title: "A".into(), url: "http://a".into(), tags: vec!["Jazz".into()] };
        let online = Station { title: "Online".into(), url: "http://o".into(), tags: vec![] };
        let mut b = Bookmarks::load_from(path.clone(), std::slice::from_ref(&a));
        assert!(b.is_empty());
        assert!(b.toggle(&a));
        assert!(b.toggle(&online));
        assert!(b.has("http://a"));

        let renamed = Station { title: "A renamed".into(), ..a.clone() };
        let mut b = Bookmarks::load_from(path.clone(), &[renamed]);
        let titles: Vec<_> = b.list().into_iter().map(|s| s.title).collect();
        assert_eq!(titles, ["A renamed", "Online"]);
        assert!(!b.toggle(&a));
        assert!(!b.has("http://a"));
        std::fs::remove_dir_all(dir).unwrap();
    }

    #[test]
    fn hand_edited_entries() {
        let dir = std::env::temp_dir().join(format!("goradion-bm-edited-{}", std::process::id()));
        let path = dir.join("bookmarks.json");
        std::fs::create_dir_all(&dir).unwrap();
        std::fs::write(&path, r#"[{"url": "http://a"}, {"title": "No URL"}, {"url": "http://b", "title": "B"}]"#)
            .unwrap();
        let b = Bookmarks::load_from(path, &[]);
        let urls: Vec<_> = b.list().into_iter().map(|s| s.url).collect();
        assert_eq!(urls, ["http://a", "http://b"]);
        std::fs::remove_dir_all(dir).unwrap();
    }
}
