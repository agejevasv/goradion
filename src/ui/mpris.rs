//! MPRIS on the app's side: the calls from D-Bus, which run on the UI loop,
//! and the state they see.

use std::sync::mpsc::Sender;

use super::{App, Page, StationRow, TagRef, Wake};
use crate::audio::player::State;
use crate::log;
use crate::mpris::{Command, Mpris, Status};
use crate::stations::Station;

/// Joins the session bus on a thread of its own: a bus that doesn't answer
/// would keep the TUI from starting. The loop gets the result as a Wake.
pub(super) fn start(wake: Sender<Wake>) {
    let spawned = std::thread::Builder::new().name("mpris".into()).spawn(move || {
        let commands = wake.clone();
        match Mpris::start(move |c| {
            let _ = commands.send(Wake::Mpris(c));
        }) {
            Ok(m) => {
                let _ = wake.send(Wake::MprisStarted(m));
            }
            Err(e) => log!("mpris: {e}"),
        }
    });
    if let Err(e) = spawned {
        log!("mpris: {e}");
    }
}

impl App {
    pub(super) fn on_mpris(&mut self, command: Command) {
        match command {
            Command::Play => self.mpris_play(),
            Command::Pause | Command::Stop => self.stop(),
            Command::PlayPause => {
                if self.mpris_playing() {
                    self.stop();
                } else {
                    self.mpris_play();
                }
            }
            Command::Next => self.play_adjacent(1),
            Command::Previous => self.play_adjacent(-1),
            Command::Volume(v) => self.player.set_volume(v),
            Command::Quit => self.quit = true,
        }
    }

    pub(super) fn sync_mpris(&self) {
        if let Some(mpris) = &self.mpris {
            mpris.update(self.mpris_status());
        }
    }

    fn mpris_status(&self) -> Status {
        let inf = self.player.snapshot();
        let resume = self.mpris_resume();
        let can_play = resume.is_some();
        let (station, url) = if inf.url.is_empty() {
            resume.map(|(s, _)| (s.title, s.url)).unwrap_or_default()
        } else {
            (inf.station, inf.url)
        };
        Status {
            playing: self.mpris_playing(),
            station,
            url,
            song: inf.song,
            volume: inf.volume,
            can_play,
            can_skip: self.adjacent_station(1).is_some(),
        }
    }

    /// A failed stream is retried, so it counts as playing.
    fn mpris_playing(&self) -> bool {
        matches!(self.player.snapshot().state, State::Buffering | State::Playing | State::Failed)
    }

    /// The station last started, else the one under the cursor, and the list
    /// it is in.
    fn mpris_resume(&self) -> Option<(Station, Option<TagRef>)> {
        if let Some(playing) = &self.playing {
            return Some(playing.clone());
        }
        match self.station_rows().get(self.stations_state.cursor) {
            Some(&StationRow::Station(i)) if self.page == Page::Main => {
                Some((self.listed[i].clone(), self.tag.clone()))
            }
            _ => None,
        }
    }

    fn mpris_play(&mut self) {
        if self.mpris_playing() {
            return;
        }
        let Some((station, from)) = self.mpris_resume() else { return };
        // A stream that can't be played keeps its URL, and playing it again
        // would stop it.
        self.player.stop();
        self.play_from(station, from);
    }

    /// The station step rows away from the one last started, in the list it
    /// was started from; the ends wrap.
    fn adjacent_station(&self, step: isize) -> Option<Station> {
        let (current, tag) = self.playing.as_ref()?;
        // A newer search replaced the results it was started from.
        if tag.as_ref().is_some_and(|t| !self.tag_exists(t)) {
            return None;
        }
        let list = self.stations_for_tag(tag.as_ref());
        let i = list.iter().position(|s| s.url == current.url)?;
        let next = (i as isize + step).rem_euclid(list.len() as isize) as usize;
        Some(list[next].clone()).filter(|s| s.url != current.url)
    }

    fn play_adjacent(&mut self, step: isize) {
        let Some(station) = self.adjacent_station(step) else { return };
        let from = self.playing.as_ref().and_then(|(_, t)| t.clone());
        self.play_from(station, from);
    }

    /// Plays station as a station of the list from, which Next and Previous
    /// then step through; `toggle_play` takes the list shown.
    fn play_from(&mut self, station: Station, from: Option<TagRef>) {
        if self.tag == from {
            self.select_station(&station.url);
        }
        self.toggle_play_manual(station);
        if let Some((_, tag)) = &mut self.playing {
            *tag = from;
        }
    }
}

#[cfg(test)]
mod tests {
    use super::super::tests::test_app;
    use super::*;

    fn url(a: &App) -> String {
        a.player.snapshot().url
    }

    #[test]
    fn next_and_previous_stay_in_the_list_played_from() {
        let (mut a, _dir) = test_app();
        a.open_tag(TagRef::new("Jazz"));
        let jazz = a.listed.clone();
        a.toggle_play(jazz[0].clone());
        a.open_tag(TagRef::new("Rock"));
        a.on_mpris(Command::Next);
        assert_eq!(url(&a), jazz[1].url);
        a.on_mpris(Command::Previous);
        a.on_mpris(Command::Previous);
        assert_eq!(url(&a), jazz[jazz.len() - 1].url);
    }

    #[test]
    fn play_pause() {
        let (mut a, _dir) = test_app();
        a.on_mpris(Command::PlayPause);
        assert!(!a.mpris_playing(), "nothing to play");
        a.open_tag(TagRef::new("Jazz"));
        let jazz = a.listed.clone();
        a.select_station(&jazz[0].url);
        a.on_mpris(Command::PlayPause);
        assert_eq!(url(&a), jazz[0].url);
        a.on_mpris(Command::Play);
        assert_eq!(url(&a), jazz[0].url, "Play keeps playing");
        a.on_mpris(Command::PlayPause);
        assert_eq!(url(&a), "");
        let stopped = a.mpris_status();
        assert_eq!((stopped.playing, stopped.can_play, stopped.url.as_str()), (false, true, jazz[0].url.as_str()));
        a.open_tag(TagRef::new("Rock"));
        a.on_mpris(Command::Play);
        assert_eq!(url(&a), jazz[0].url, "resumes the station last played");
        a.on_mpris(Command::Next);
        assert_eq!(url(&a), jazz[1].url, "in the list it was started from");
    }

    #[test]
    fn a_one_station_list_has_no_next() {
        let (mut a, _dir) = test_app();
        let s = a.stations[0].clone();
        a.toggle_bookmark_of(&s);
        a.open_tag(TagRef::new(super::super::BOOKMARKS_TAG));
        a.toggle_play(s.clone());
        assert!(!a.mpris_status().can_skip);
        a.on_mpris(Command::Next);
        assert_eq!(url(&a), s.url);
    }

    #[test]
    fn a_newer_search_has_no_next() {
        let (mut a, _dir) = test_app();
        let s = a.stations[..3].to_vec();
        a.open_search("one", s[..2].to_vec(), false);
        a.toggle_play(s[0].clone());
        assert!(a.mpris_status().can_skip);
        a.open_search("two", s[1..].to_vec(), false);
        assert!(!a.mpris_status().can_skip);
        a.on_mpris(Command::Next);
        assert_eq!(url(&a), s[0].url);
    }
}
