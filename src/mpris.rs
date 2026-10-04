//! MPRIS (Media Player Remote Interfacing Specification) on the D-Bus session
//! bus: media keys, desktop widgets and playerctl control goradion. Calls are
//! sent to the UI loop as Commands; the loop publishes what plays with
//! `Mpris::update`.

// The code zbus generates reads the arguments that Seek and SetPosition ignore.
#![allow(clippy::used_underscore_binding)]

use std::collections::HashMap;
use std::sync::{Arc, Mutex, MutexGuard};

use zbus::blocking::Connection;
use zbus::blocking::connection::Builder;
use zbus::fdo::{self, RequestNameFlags};
use zbus::interface;
use zbus::zvariant::{ObjectPath, Value};

use crate::log;

const PATH: &str = "/org/mpris/MediaPlayer2";
const NAME: &str = "org.mpris.MediaPlayer2.goradion";
const PLAYER_INTERFACE: &str = "org.mpris.MediaPlayer2.Player";
const NO_TRACK: &str = "/org/mpris/MediaPlayer2/TrackList/NoTrack";

#[derive(Clone, Copy, Debug, PartialEq)]
pub enum Command {
    Play,
    /// A stream can't pause: it stops.
    Pause,
    PlayPause,
    Stop,
    Next,
    Previous,
    /// 0-100.
    Volume(i32),
    Quit,
}

/// What the player interface shows.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct Status {
    pub playing: bool,
    /// The station playing, else the one Play starts.
    pub station: String,
    pub url: String,
    pub song: String,
    pub volume: i32,
    pub can_play: bool,
    /// Next and Previous have another station: the list wraps.
    pub can_skip: bool,
}

impl Status {
    /// Paused when Play resumes a station: Waybar keeps showing a stopped
    /// player as playing.
    fn playback_status(&self) -> &'static str {
        match (self.playing, self.can_play) {
            (true, _) => "Playing",
            (false, true) => "Paused",
            (false, false) => "Stopped",
        }
    }
}

type Changes = HashMap<&'static str, Value<'static>>;

#[derive(Default)]
struct Published {
    status: Status,
    /// Changes with the station and the song, as a new track.
    track: u64,
}

impl Published {
    fn metadata(&self) -> Changes {
        let s = &self.status;
        let mut m = HashMap::new();
        if s.url.is_empty() {
            m.insert("mpris:trackid", ObjectPath::from_static_str_unchecked(NO_TRACK).into());
            return m;
        }
        let track = ObjectPath::try_from(format!("/org/goradion/track/{}", self.track)).expect("valid object path");
        m.insert("mpris:trackid", track.into());
        m.insert("xesam:url", s.url.clone().into());
        // Without a song, an empty artist: GNOME shows "Unknown artist" for
        // none.
        let (title, artist) =
            if s.song.is_empty() { (&s.station, String::new()) } else { (&s.song, s.station.clone()) };
        m.insert("xesam:title", title.clone().into());
        m.insert("xesam:artist", vec![artist].into());
        m
    }

    fn volume(&self) -> f64 {
        f64::from(self.status.volume) / 100.0
    }

    /// The properties that status changes; None when it changes none.
    fn apply(&mut self, status: Status) -> Option<Changes> {
        if self.status == status {
            return None;
        }
        let old = std::mem::replace(&mut self.status, status);
        let new_track = self.status.url != old.url || self.status.song != old.song;
        if new_track {
            self.track += 1;
        }
        let s = &self.status;
        let mut changed = HashMap::new();
        if new_track || s.station != old.station {
            changed.insert("Metadata", self.metadata().into());
        }
        if s.playback_status() != old.playback_status() {
            changed.insert("PlaybackStatus", s.playback_status().into());
        }
        if s.playing != old.playing {
            changed.insert("CanPause", s.playing.into());
        }
        if s.volume != old.volume {
            changed.insert("Volume", self.volume().into());
        }
        if s.can_play != old.can_play {
            changed.insert("CanPlay", s.can_play.into());
        }
        if s.can_skip != old.can_skip {
            changed.insert("CanGoNext", s.can_skip.into());
            changed.insert("CanGoPrevious", s.can_skip.into());
        }
        Some(changed)
    }
}

type OnCommand = Arc<dyn Fn(Command) + Send + Sync>;

struct Root {
    on_command: OnCommand,
}

// zbus calls every member through self.
#[allow(clippy::unused_self)]
#[interface(name = "org.mpris.MediaPlayer2")]
impl Root {
    /// A terminal can't be raised.
    fn raise(&self) {}

    fn quit(&self) {
        (self.on_command)(Command::Quit);
    }

    #[zbus(property(emits_changed_signal = "const"))]
    fn can_quit(&self) -> bool {
        true
    }

    #[zbus(property(emits_changed_signal = "const"))]
    fn can_raise(&self) -> bool {
        false
    }

    #[zbus(property(emits_changed_signal = "const"))]
    fn has_track_list(&self) -> bool {
        false
    }

    #[zbus(property(emits_changed_signal = "const"))]
    fn identity(&self) -> &'static str {
        "goradion"
    }

    #[zbus(property(emits_changed_signal = "const"))]
    fn supported_uri_schemes(&self) -> Vec<String> {
        Vec::new()
    }

    #[zbus(property(emits_changed_signal = "const"))]
    fn supported_mime_types(&self) -> Vec<String> {
        Vec::new()
    }
}

struct Player {
    on_command: OnCommand,
    published: Arc<Mutex<Published>>,
}

impl Player {
    fn published(&self) -> MutexGuard<'_, Published> {
        self.published.lock().unwrap()
    }
}

// zbus calls every member through self.
#[allow(clippy::unused_self)]
#[interface(name = "org.mpris.MediaPlayer2.Player")]
impl Player {
    fn next(&self) {
        (self.on_command)(Command::Next);
    }

    fn previous(&self) {
        (self.on_command)(Command::Previous);
    }

    fn pause(&self) {
        (self.on_command)(Command::Pause);
    }

    fn play_pause(&self) {
        (self.on_command)(Command::PlayPause);
    }

    fn stop(&self) {
        (self.on_command)(Command::Stop);
    }

    fn play(&self) {
        (self.on_command)(Command::Play);
    }

    /// Streams can't seek.
    fn seek(&self, _offset: i64) {}

    fn set_position(&self, _track_id: ObjectPath<'_>, _position: i64) {}

    fn open_uri(&self, _uri: &str) -> fdo::Result<()> {
        Err(fdo::Error::NotSupported("goradion plays its own stations".into()))
    }

    #[zbus(property)]
    fn playback_status(&self) -> &'static str {
        self.published().status.playback_status()
    }

    #[zbus(property)]
    fn metadata(&self) -> Changes {
        self.published().metadata()
    }

    #[zbus(property)]
    fn volume(&self) -> f64 {
        self.published().volume()
    }

    #[zbus(property)]
    fn set_volume(&self, volume: f64) {
        let volume = (volume.clamp(0.0, 1.0) * 100.0).round() as i32;
        // zbus emits the property when this returns, before the UI loop has
        // set it: the change would read as the old volume.
        self.published().status.volume = volume;
        (self.on_command)(Command::Volume(volume));
    }

    #[zbus(property(emits_changed_signal = "false"))]
    fn position(&self) -> i64 {
        0
    }

    #[zbus(property(emits_changed_signal = "const"))]
    fn rate(&self) -> f64 {
        1.0
    }

    #[zbus(property(emits_changed_signal = "const"))]
    fn minimum_rate(&self) -> f64 {
        1.0
    }

    #[zbus(property(emits_changed_signal = "const"))]
    fn maximum_rate(&self) -> f64 {
        1.0
    }

    #[zbus(property)]
    fn can_go_next(&self) -> bool {
        self.published().status.can_skip
    }

    #[zbus(property)]
    fn can_go_previous(&self) -> bool {
        self.published().status.can_skip
    }

    #[zbus(property)]
    fn can_play(&self) -> bool {
        self.published().status.can_play
    }

    /// Pause stops, which only a playing stream can do.
    #[zbus(property)]
    fn can_pause(&self) -> bool {
        self.published().status.playing
    }

    #[zbus(property(emits_changed_signal = "const"))]
    fn can_seek(&self) -> bool {
        false
    }

    #[zbus(property(emits_changed_signal = "const"))]
    fn can_control(&self) -> bool {
        true
    }
}

/// Lives until dropped, which leaves the bus.
pub struct Mpris {
    conn: Connection,
    published: Arc<Mutex<Published>>,
}

impl Mpris {
    /// Serves on the session bus; `on_command` gets the calls, on a zbus
    /// thread. Blocks while the bus doesn't answer: zbus has no timeout for
    /// connecting.
    pub fn start(on_command: impl Fn(Command) + Send + Sync + 'static) -> zbus::Result<Mpris> {
        let on_command: OnCommand = Arc::new(on_command);
        let published = Arc::new(Mutex::new(Published::default()));
        let conn = Builder::session()?
            .serve_at(PATH, Root { on_command: on_command.clone() })?
            .serve_at(PATH, Player { on_command, published: published.clone() })?
            .build()?;
        // zbus's default flags let the next instance take the name; it gets
        // the spec's name for a second instance instead.
        let flags = RequestNameFlags::DoNotQueue.into();
        let name = match conn.request_name_with_flags(NAME, flags) {
            Ok(_) => NAME.to_string(),
            Err(zbus::Error::NameTaken) => {
                let name = format!("{NAME}.instance{}", std::process::id());
                conn.request_name_with_flags(name.as_str(), flags)?;
                name
            }
            Err(e) => return Err(e),
        };
        log!("mpris: serving as {name}");
        Ok(Mpris { conn, published })
    }

    /// Emits the properties that changed.
    pub fn update(&self, status: Status) {
        let Some(changed) = self.published.lock().unwrap().apply(status) else { return };
        let body = (PLAYER_INTERFACE, changed, Vec::<&str>::new());
        if let Err(e) =
            self.conn.emit_signal(None::<&str>, PATH, "org.freedesktop.DBus.Properties", "PropertiesChanged", &body)
        {
            log!("mpris: {e}");
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn keys(changes: &Changes) -> Vec<&str> {
        let mut k: Vec<&str> = changes.keys().copied().collect();
        k.sort_unstable();
        k
    }

    #[test]
    fn apply_reports_only_what_changed() {
        let mut p = Published::default();
        let radio = Status {
            playing: true,
            station: "R".into(),
            url: "http://r".into(),
            volume: 50,
            can_play: true,
            ..Status::default()
        };
        let all = ["CanPause", "CanPlay", "Metadata", "PlaybackStatus", "Volume"];
        assert_eq!(keys(&p.apply(radio.clone()).unwrap()), all);
        assert_eq!(p.track, 1);
        assert!(p.apply(radio.clone()).is_none());
        let song = Status { song: "S".into(), ..radio.clone() };
        assert_eq!(keys(&p.apply(song.clone()).unwrap()), ["Metadata"]);
        assert_eq!(p.track, 2, "a new song is a new track");
        assert_eq!(p.metadata()["xesam:title"], Value::from("S"));
        let louder = Status { volume: 60, ..song };
        assert_eq!(keys(&p.apply(louder.clone()).unwrap()), ["Volume"]);
        assert_eq!(p.track, 2);
        let stopped = Status { playing: false, song: String::new(), ..louder };
        assert_eq!(keys(&p.apply(stopped).unwrap()), ["CanPause", "Metadata", "PlaybackStatus"]);
        assert_eq!(p.status.playback_status(), "Paused", "Play resumes it");
        assert_eq!(p.metadata()["xesam:title"], Value::from("R"));
        assert_eq!(p.metadata()["xesam:artist"], Value::from(vec![""]));
    }
}
