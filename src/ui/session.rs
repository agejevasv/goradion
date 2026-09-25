//! The last station, its tag and the volume, kept in the config between runs.

use super::{ALL_STATIONS_TAG, App, BOOKMARKS_TAG, TagRef};
use crate::log;
use crate::stations::Station;

impl App {
    /// Leaves the default view when the tag or station is gone, e.g. after an
    /// update of the built-in stations.
    pub(super) fn restore_session(&mut self) {
        self.player.set_volume(self.config.volume);
        let tag = TagRef::new(&self.config.tag);
        let Some(station) = self.find_station(&tag, &self.config.station) else { return };
        if tag.name == ALL_STATIONS_TAG {
            return;
        }
        self.open_tag(tag);
        self.select_station(&station.url);
        if self.config.autoplay {
            self.toggle_play(station);
        }
    }

    /// Falls back to the station's own tag or Bookmarks for a station started
    /// from All Stations, which has no row in the tags list.
    pub(super) fn save_session(&mut self) {
        self.config.volume = self.player.snapshot().volume;
        let mut keys = vec!["volume"];
        if let Some((station, from)) = self.playing.clone() {
            keys.extend(["tag", "station"]);
            self.config.tag.clear();
            self.config.station.clear();
            let mut candidates: Vec<TagRef> = from.into_iter().collect();
            if let Some(s) = self.find_station(&TagRef::new(ALL_STATIONS_TAG), &station.url) {
                candidates.extend(s.tags.first().map(|t| TagRef::new(t)));
            }
            candidates.push(TagRef::new(BOOKMARKS_TAG));
            for tag in candidates.into_iter().filter(|t| t.name != ALL_STATIONS_TAG) {
                if self.find_station(&tag, &station.url).is_some() {
                    self.config.tag = tag.name;
                    self.config.station = station.url;
                    break;
                }
            }
        }
        if let Err(e) = self.config.save(&keys) {
            log!("config: {e}");
        }
    }

    pub(super) fn tag_exists(&self, tag: &TagRef) -> bool {
        if tag.search {
            return self.last_search.as_ref().is_some_and(|l| !tag.name.is_empty() && l.query == tag.name);
        }
        tag.name == BOOKMARKS_TAG || tag.name == ALL_STATIONS_TAG || self.tags.contains(&tag.name)
    }

    fn find_station(&self, tag: &TagRef, url: &str) -> Option<Station> {
        if url.is_empty() || tag.name.is_empty() || tag.search || !self.tag_exists(tag) {
            return None;
        }
        self.stations_for_tag(Some(tag)).into_iter().find(|s| s.url == url)
    }
}

#[cfg(test)]
mod tests {
    use super::super::Page;
    use super::super::tests::{test_app, test_app_with};
    use super::*;
    use crate::config::Config;

    fn with_tag<'a>(stations: &'a [Station], tag: &str) -> Vec<&'a Station> {
        stations.iter().filter(|s| s.tags.iter().any(|t| t == tag)).collect()
    }

    #[test]
    fn saved() {
        let (mut a, dir) = test_app();
        a.open_tag(TagRef::new("Jazz"));
        a.player.set_volume(35);
        let jazz = a.listed[1].clone();
        a.toggle_play(jazz.clone());
        a.save_session();
        let c = Config::load_from(dir.join("config.yaml"));
        assert_eq!((c.volume, c.tag.as_str(), c.station.as_str(), c.autoplay), (35, "Jazz", jazz.url.as_str(), true));
    }

    #[test]
    fn autoplay_kept_when_off() {
        let (mut a, dir) = test_app_with("autoplay: false # quiet\n");
        a.save_session();
        let text = std::fs::read_to_string(dir.join("config.yaml")).unwrap();
        assert!(text.contains("autoplay: false # quiet\n"), "{text}");
    }

    #[test]
    fn from_all_stations_uses_own_tag() {
        let (mut a, dir) = test_app();
        a.open_tag(TagRef::new(ALL_STATIONS_TAG));
        let s = a.listed[2].clone();
        a.toggle_play(s.clone());
        a.save_session();
        let c = Config::load_from(dir.join("config.yaml"));
        assert_eq!((c.tag.as_str(), c.station.as_str()), (s.tags[0].as_str(), s.url.as_str()));
    }

    #[test]
    fn restored() {
        for autoplay in [false, true] {
            let stations = crate::stations::load("").unwrap();
            let jazz = with_tag(&stations, "Jazz")[1].clone();
            let mut text = format!("theme: nord\nvolume: 40\ntag: Jazz\nstation: {}\n", jazz.url);
            if !autoplay {
                text.push_str("autoplay: false\n");
            }
            let (a, _dir) = test_app_with(&text);
            let row = a.station_rows()[a.stations_state.cursor];
            let under_cursor = match row {
                super::super::StationRow::Station(i) => a.listed[i].url.clone(),
                _ => String::new(),
            };
            let playing = a.playing.as_ref().map(|(s, _)| s.url.clone()).unwrap_or_default();
            assert_eq!(a.tag.as_ref().map(|t| t.name.as_str()), Some("Jazz"));
            assert_eq!(a.page, Page::Main);
            assert_eq!(under_cursor, jazz.url);
            assert_eq!(a.player.snapshot().volume, 40);
            assert_eq!(playing, if autoplay { jazz.url.clone() } else { String::new() });
        }
    }

    /// The session is restored before the first draw picks the layout; the
    /// wide one has no back row above the stations.
    #[test]
    fn restored_cursor_survives_layout() {
        let stations = crate::stations::load("").unwrap();
        let jazz = with_tag(&stations, "Jazz")[2].clone();
        let (mut a, _dir) = test_app_with(&format!("tag: Jazz\nstation: {}\nautoplay: false\n", jazz.url));
        let under_cursor = |a: &App| match a.station_rows()[a.stations_state.cursor] {
            super::super::StationRow::Station(i) => a.listed[i].url.clone(),
            other => format!("{other:?}"),
        };
        for width in [120, 80, 120] {
            a.sync_layout(width);
            assert_eq!(under_cursor(&a), jazz.url, "width {width}");
        }
    }

    #[test]
    fn gone_shows_default() {
        for text in [
            "tag: No Such Tag\nstation: https://example.com/\n",
            "tag: All Stations\nstation: https://streams.fluxfm.de/xjazz/mp3-320/audio/\n",
            "tag: Jazz\nstation: https://example.com/gone\n",
            "tag: Jazz\nstation: https://streams.fluxfm.de/chillout/mp3-320/streams.fluxfm.de/play.pls\n",
        ] {
            let (a, _dir) = test_app_with(text);
            assert_eq!(a.page, Page::Tags, "{text}");
            assert!(a.playing.is_none(), "{text}");
        }
    }

    #[test]
    fn without_playing_keeps_station() {
        let (mut a, dir) = test_app_with("tag: Gone\nstation: https://example.com/\n");
        a.save_session();
        let text = std::fs::read_to_string(dir.join("config.yaml")).unwrap();
        assert!(text.starts_with("tag: Gone\nstation: https://example.com/\n"), "{text}");
    }
}
