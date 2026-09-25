//! Shuffle and the sleep timer, advanced by the UI loop's ticks.

use std::time::{Duration, Instant};

use super::App;

pub const DEFAULT_SHUFFLE_INTERVAL: Duration = Duration::from_secs(5 * 60);

pub const SLEEP_STEPS: [Duration; 5] = [
    Duration::from_secs(15 * 60),
    Duration::from_secs(30 * 60),
    Duration::from_secs(45 * 60),
    Duration::from_secs(60 * 60),
    Duration::from_secs(90 * 60),
];
pub const SLEEP_FADE: Duration = Duration::from_secs(60);

/// Plays a random station of the list every interval; the player crossfades.
#[derive(Debug)]
pub struct Shuffle {
    pub active: bool,
    pub interval: Duration,
    /// When the current interval began.
    start: Instant,
}

impl Default for Shuffle {
    fn default() -> Self {
        Shuffle { active: false, interval: DEFAULT_SHUFFLE_INTERVAL, start: Instant::now() }
    }
}

impl Shuffle {
    pub fn remaining(&self, now: Instant) -> Duration {
        self.interval.saturating_sub(now.saturating_duration_since(self.start))
    }
}

#[derive(Debug, Default)]
pub struct Sleep {
    /// Zero when off.
    pub total: Duration,
    end: Option<Instant>,
    fade: Option<Duration>,
}

impl Sleep {
    pub fn active(&self) -> bool {
        !self.total.is_zero()
    }

    pub fn remaining(&self, now: Instant) -> Duration {
        self.end.map_or(Duration::ZERO, |end| end.saturating_duration_since(now))
    }

    pub fn fade(&self) -> Duration {
        self.fade.unwrap_or(SLEEP_FADE)
    }
}

fn next_sleep(current: Duration) -> Duration {
    SLEEP_STEPS.into_iter().find(|&d| d > current).unwrap_or(Duration::ZERO)
}

impl App {
    pub(super) fn tick(&mut self, now: Instant) {
        self.tick_shuffle(now);
        self.tick_sleep(now);
    }

    pub(super) fn toggle_shuffle(&mut self, now: Instant) {
        if self.shuffle.active {
            self.stop_shuffle();
            return;
        }
        self.shuffle.active = true;
        self.play_random();
        self.shuffle.start = now;
    }

    pub(super) fn stop_shuffle(&mut self) {
        self.shuffle.active = false;
    }

    pub(super) fn set_shuffle_interval(&mut self, minutes: u64, now: Instant) {
        self.shuffle.interval = Duration::from_secs(minutes * 60);
        self.shuffle.start = now;
    }

    fn tick_shuffle(&mut self, now: Instant) {
        if self.shuffle.active && self.shuffle.remaining(now).is_zero() {
            self.shuffle.start = now;
            self.play_random();
        }
    }

    /// Steps through `SLEEP_STEPS`, then off. Does nothing while nothing plays.
    pub(super) fn cycle_sleep(&mut self, now: Instant) {
        let next = next_sleep(self.sleep.total);
        if !self.sleep.active() && self.player.snapshot().url.is_empty() {
            return;
        }
        self.cancel_sleep();
        self.card.flash_sleep(std::time::SystemTime::now());
        if next.is_zero() {
            return;
        }
        self.sleep.total = next;
        self.sleep.end = Some(now + next);
    }

    pub(super) fn cancel_sleep(&mut self) {
        self.sleep.total = Duration::ZERO;
        self.sleep.end = None;
        self.player.set_fade(1.0);
    }

    /// Fades out over the last minute, then stops. The volume stays as set,
    /// and can still be changed while it fades.
    fn tick_sleep(&mut self, now: Instant) {
        if !self.sleep.active() {
            return;
        }
        let remaining = self.sleep.remaining(now);
        let fade = self.sleep.fade();
        if remaining > fade {
            return;
        }
        // A new station now would cut into the fade.
        self.stop_shuffle();
        if remaining.is_zero() {
            // Silent before the fade goes back up.
            self.player.stop();
            self.stop();
            return;
        }
        self.player.set_fade((remaining.as_secs_f64() / fade.as_secs_f64().max(1e-9)) as f32);
    }
}

#[cfg(test)]
mod tests {
    use super::super::tests::test_app;
    use super::super::{StationRow, TagRef};
    use super::*;
    use crate::audio::player::{DEFAULT_VOLUME, VOLUME_STEP};

    fn url(a: &App) -> String {
        a.player.snapshot().url
    }

    fn volume(a: &App) -> i32 {
        a.player.snapshot().volume
    }

    #[test]
    fn shuffle() {
        let (mut a, _dir) = test_app();
        a.open_tag(TagRef::new(super::super::ALL_STATIONS_TAG));
        let t0 = Instant::now();
        a.toggle_shuffle(t0);
        let first = url(&a);
        assert!(!first.is_empty() && a.shuffle.active, "shuffle should start with a random station");
        let row = a.station_rows()[a.stations_state.cursor];
        assert!(matches!(row, StationRow::Station(i) if a.listed[i].url == first));

        a.tick(t0 + a.shuffle.interval / 2);
        assert_eq!(url(&a), first);
        let next = t0 + a.shuffle.interval;
        a.tick(next);
        assert_ne!(url(&a), first);
        assert_eq!(volume(&a), DEFAULT_VOLUME, "the player crossfades; the volume stays");
        assert_eq!(a.shuffle.remaining(next), a.shuffle.interval);

        a.toggle_play_manual(a.listed[0].clone());
        assert!(!a.shuffle.active, "playing a station must end the shuffle");
    }

    /// A list of only the station playing, even listed twice, keeps it on.
    #[test]
    fn shuffle_on_the_station_playing() {
        let (mut a, _dir) = test_app();
        let s = a.stations[0].clone();
        a.open_search("x", vec![s.clone(), s.clone()], false);
        a.toggle_play_manual(s.clone());
        let now = Instant::now();
        a.toggle_shuffle(now);
        a.tick(now + a.shuffle.interval);
        assert_eq!(url(&a), s.url);
    }

    #[test]
    fn shuffle_interval() {
        let (mut a, _dir) = test_app();
        let now = Instant::now();
        a.set_shuffle_interval(3, now);
        a.toggle_shuffle(now);
        assert!(a.shuffle.active);
        assert_eq!(a.shuffle.remaining(now + Duration::from_secs(5)), Duration::from_secs(175));
        a.toggle_shuffle(now);
        assert!(!a.shuffle.active);
    }

    #[test]
    fn sleep_cycle() {
        let (mut a, _dir) = test_app();
        let now = Instant::now();
        a.cycle_sleep(now);
        assert!(!a.sleep.active(), "timer set with nothing playing");
        a.open_tag(TagRef::new(super::super::ALL_STATIONS_TAG));
        a.toggle_play_manual(a.listed[0].clone());

        let mut got = Vec::new();
        for _ in 0..=SLEEP_STEPS.len() {
            a.cycle_sleep(now);
            got.push(a.sleep.total);
        }
        let mut want = SLEEP_STEPS.to_vec();
        want.push(Duration::ZERO);
        assert_eq!(got, want);

        a.cycle_sleep(now);
        let playing = a.playing.clone().unwrap().0;
        a.toggle_play_manual(playing);
        assert!(!a.sleep.active(), "stopping must turn the timer off");
    }

    #[test]
    fn sleep_fades_and_stops() {
        let (mut a, _dir) = test_app();
        a.open_tag(TagRef::new(super::super::ALL_STATIONS_TAG));
        a.toggle_play_manual(a.listed[0].clone());
        let now = Instant::now();
        a.toggle_shuffle(now);
        a.cycle_sleep(now);
        let end = now + SLEEP_STEPS[0];
        a.tick(end.checked_sub(SLEEP_FADE / 2).unwrap());
        assert!(!a.shuffle.active, "shuffle still on");
        assert!((a.player.fade() - 0.5).abs() < 1e-3, "fade {}", a.player.fade());
        assert_eq!(volume(&a), DEFAULT_VOLUME, "the volume bar stays");
        a.tick(end);
        assert_eq!(url(&a), "", "still playing");
        assert_eq!(volume(&a), DEFAULT_VOLUME);
        assert!((a.player.fade() - 1.0).abs() < f32::EPSILON, "ready for the next station");
        assert!(!a.sleep.active());
    }

    #[test]
    fn sleep_cancel_ends_the_fade() {
        let (mut a, _dir) = test_app();
        a.open_tag(TagRef::new(super::super::ALL_STATIONS_TAG));
        a.toggle_play_manual(a.listed[0].clone());
        let now = Instant::now();
        a.cycle_sleep(now);
        a.tick((now + SLEEP_STEPS[0]).checked_sub(SLEEP_FADE / 4).unwrap());
        assert!(a.player.fade() < 0.5);
        // The volume can be changed during the fade.
        a.change_volume(-VOLUME_STEP);
        a.tick((now + SLEEP_STEPS[0]).checked_sub(SLEEP_FADE / 5).unwrap());
        assert_eq!(volume(&a), DEFAULT_VOLUME - VOLUME_STEP);
        a.cycle_sleep(now); // 30 minutes: starts over
        assert!((a.player.fade() - 1.0).abs() < f32::EPSILON);
        assert!(!url(&a).is_empty());
    }
}
