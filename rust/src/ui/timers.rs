//! Shuffle and the sleep timer, advanced by the UI loop's ticks.

use std::time::{Duration, Instant};

use super::App;
use crate::audio::player::State;

pub const DEFAULT_SHUFFLE_INTERVAL: Duration = Duration::from_secs(5 * 60);
const SHUFFLE_FADE: Duration = Duration::from_secs(2);
/// How long shuffle waits for a station to play before fading in anyway.
const PLAYBACK_TIMEOUT: Duration = Duration::from_secs(30);

pub const SLEEP_STEPS: [Duration; 5] = [
    Duration::from_secs(15 * 60),
    Duration::from_secs(30 * 60),
    Duration::from_secs(45 * 60),
    Duration::from_secs(60 * 60),
    Duration::from_secs(90 * 60),
];
pub const SLEEP_FADE: Duration = Duration::from_secs(60);

#[derive(Clone, Copy, Debug)]
struct Fade {
    from: i32,
    to: i32,
    start: Instant,
    duration: Duration,
}

impl Fade {
    fn new(from: i32, to: i32, start: Instant, duration: Duration) -> Self {
        Fade { from, to, start, duration }
    }

    /// The volume at now, and whether the fade is over.
    fn at(&self, now: Instant) -> (i32, bool) {
        let t = now.saturating_duration_since(self.start).as_secs_f64() / self.duration.as_secs_f64().max(1e-9);
        if t >= 1.0 {
            return (self.to, true);
        }
        (self.from + ((self.to - self.from) as f64 * t).round() as i32, false)
    }
}

#[derive(Debug)]
enum Phase {
    Waiting,
    FadingOut(Fade),
    /// The station picked is tuning in, silent.
    Tuning {
        url: String,
        since: Instant,
    },
    FadingIn(Fade),
}

/// Plays a random station of the list every interval, fading between them.
#[derive(Debug)]
pub struct Shuffle {
    pub active: bool,
    pub interval: Duration,
    fade: Duration,
    timeout: Duration,
    /// When the current interval began.
    start: Instant,
    phase: Phase,
    /// The volume to put back when stopped mid-fade.
    restore: Option<i32>,
}

impl Default for Shuffle {
    fn default() -> Self {
        Shuffle {
            active: false,
            interval: DEFAULT_SHUFFLE_INTERVAL,
            fade: SHUFFLE_FADE,
            timeout: PLAYBACK_TIMEOUT,
            start: Instant::now(),
            phase: Phase::Waiting,
            restore: None,
        }
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
    /// The volume to put back once the fade is over or cancelled.
    restore: Option<i32>,
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
        self.restart_shuffle(now);
    }

    pub(super) fn stop_shuffle(&mut self) {
        if !self.shuffle.active {
            return;
        }
        self.shuffle.active = false;
        self.end_shuffle_fade();
    }

    pub(super) fn set_shuffle_interval(&mut self, minutes: u64, now: Instant) {
        self.shuffle.interval = Duration::from_secs(minutes * 60);
        if self.shuffle.active {
            self.restart_shuffle(now);
        }
    }

    fn restart_shuffle(&mut self, now: Instant) {
        self.end_shuffle_fade();
        self.shuffle.start = now;
    }

    fn end_shuffle_fade(&mut self) {
        self.shuffle.phase = Phase::Waiting;
        if let Some(volume) = self.shuffle.restore.take() {
            self.player.set_volume(volume);
        }
    }

    /// Fades out, plays a random station and fades back in once it plays, or
    /// after the timeout. The next interval starts when the station is picked.
    fn tick_shuffle(&mut self, now: Instant) {
        if !self.shuffle.active {
            return;
        }
        let due = now.saturating_duration_since(self.shuffle.start) >= self.shuffle.interval;
        let next = match std::mem::replace(&mut self.shuffle.phase, Phase::Waiting) {
            Phase::Waiting if due => {
                let volume = self.player.snapshot().volume;
                self.shuffle.restore = Some(volume);
                Phase::FadingOut(Fade::new(volume, 0, now, self.shuffle.fade))
            }
            Phase::FadingOut(fade) => {
                let (volume, done) = fade.at(now);
                self.player.set_volume(volume);
                if done {
                    self.shuffle.start = now;
                    let url = self.play_random().unwrap_or_default();
                    Phase::Tuning { url, since: now }
                } else {
                    Phase::FadingOut(fade)
                }
            }
            Phase::Tuning { url, since } => {
                let inf = self.player.snapshot();
                let tuned = inf.url != url || inf.state == State::Playing || !inf.song.is_empty();
                if tuned || now.saturating_duration_since(since) >= self.shuffle.timeout {
                    let to = self.shuffle.restore.unwrap_or(inf.volume);
                    Phase::FadingIn(Fade::new(inf.volume, to, now, self.shuffle.fade))
                } else {
                    Phase::Tuning { url, since }
                }
            }
            Phase::FadingIn(fade) => {
                let (volume, done) = fade.at(now);
                self.player.set_volume(volume);
                if done {
                    self.shuffle.restore = None;
                    Phase::Waiting
                } else {
                    Phase::FadingIn(fade)
                }
            }
            waiting @ Phase::Waiting => waiting,
        };
        self.shuffle.phase = next;
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
        if let Some(volume) = self.sleep.restore.take() {
            self.player.set_volume(volume);
        }
    }

    /// Fades out over the last minute, then stops and puts the volume back.
    fn tick_sleep(&mut self, now: Instant) {
        if !self.sleep.active() {
            return;
        }
        let remaining = self.sleep.remaining(now);
        let fade = self.sleep.fade();
        if remaining > fade {
            return;
        }
        if self.sleep.restore.is_none() {
            // Stopping the shuffle first puts back a volume it faded.
            self.stop_shuffle();
            self.sleep.restore = Some(self.player.snapshot().volume);
        }
        let from = self.sleep.restore.unwrap_or_default();
        let t = remaining.as_secs_f64() / fade.as_secs_f64().max(1e-9);
        self.player.set_volume((from as f64 * t).round() as i32);
        if remaining.is_zero() {
            self.stop();
        }
    }
}

#[cfg(test)]
mod tests {
    use super::super::tests::test_app;
    use super::super::{StationRow, TagRef};
    use super::*;
    use crate::audio::player::DEFAULT_VOLUME;

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

        // The offline player never plays, so the station fades in after the
        // timeout.
        a.shuffle.fade = Duration::from_millis(10);
        a.shuffle.timeout = Duration::from_millis(300);
        let mut now = t0 + a.shuffle.interval;
        a.tick(now);
        now += Duration::from_millis(10);
        a.tick(now);
        assert_ne!(url(&a), first);
        assert_eq!(volume(&a), 0, "silent while tuning in");
        let picked = now;
        assert_eq!(a.shuffle.remaining(picked), a.shuffle.interval);
        now += Duration::from_millis(300);
        a.tick(now);
        now += Duration::from_millis(10);
        a.tick(now);
        assert_eq!(volume(&a), DEFAULT_VOLUME);

        // Picking a station ends the shuffle and restores the volume at once.
        now = picked + a.shuffle.interval;
        a.tick(now);
        a.tick(now + Duration::from_millis(5));
        assert!(volume(&a) < DEFAULT_VOLUME);
        a.toggle_play_manual(a.listed[0].clone());
        assert_eq!(volume(&a), DEFAULT_VOLUME);
        assert!(!a.shuffle.active, "playing a station must end the shuffle");
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
        assert_eq!(volume(&a), DEFAULT_VOLUME / 2);
        a.tick(end);
        assert_eq!(url(&a), "", "still playing");
        assert_eq!(volume(&a), DEFAULT_VOLUME);
        assert!(!a.sleep.active());
    }

    #[test]
    fn sleep_cancel_restores_volume() {
        let (mut a, _dir) = test_app();
        a.open_tag(TagRef::new(super::super::ALL_STATIONS_TAG));
        a.toggle_play_manual(a.listed[0].clone());
        let now = Instant::now();
        a.cycle_sleep(now);
        a.tick((now + SLEEP_STEPS[0]).checked_sub(SLEEP_FADE / 4).unwrap());
        assert!(volume(&a) < DEFAULT_VOLUME);
        a.cycle_sleep(now); // 30 minutes: starts over
        assert_eq!(volume(&a), DEFAULT_VOLUME);
        assert!(!url(&a).is_empty());
    }
}
