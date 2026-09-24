//! The now-playing card under the lists.

use std::time::{Duration, SystemTime};

use ratatui::buffer::Buffer;
use ratatui::layout::Rect;
use ratatui::style::{Color, Modifier};

use super::Look;
use super::text::{self, Seg, clock, fit_segs, gauge, marquee, seg, segs_width, spread};
use super::theme::mix;
use crate::audio::meter::BAND_COUNT;
use crate::audio::player::{Info, State, Track};

pub const HEIGHT: u16 = 5; // borders and three rows
const GLOW_PERIOD: Duration = Duration::from_secs(2);
const VOLUME_FLASH: Duration = Duration::from_millis(1500);
const VU_FALL_PER_SEC: f64 = 1.4;
const VU_PEAK_HOLD: Duration = Duration::from_secs(1);
const VU_STALE: Duration = Duration::from_millis(700);

pub trait MeterSource {
    fn level(&self) -> Option<(f64, SystemTime)>;
    fn spectrum(&self) -> Option<([f64; BAND_COUNT], SystemTime)>;
}

impl MeterSource for crate::audio::Player {
    fn level(&self) -> Option<(f64, SystemTime)> {
        crate::audio::Player::level(self)
    }

    fn spectrum(&self) -> Option<([f64; BAND_COUNT], SystemTime)> {
        crate::audio::Player::spectrum(self)
    }
}

/// What the card shows of the app around the player.
#[derive(Default)]
pub struct Extras {
    pub bookmarked: bool,
    /// Remaining and full interval.
    pub shuffle: Option<(Duration, Duration)>,
    /// Remaining and the fade at its end.
    pub sleep: Option<(Duration, Duration)>,
}

#[derive(Default)]
pub struct Card {
    info: Info,
    has_info: bool,
    /// False with --no-vu.
    pub vu: bool,

    playing_since: Option<SystemTime>,
    song_since: Option<SystemTime>,
    flash_until: Option<SystemTime>,
    sleep_flash: Option<SystemTime>,

    level: f64,
    peak: f64,
    peak_at: Option<SystemTime>,
    bands: [f64; BAND_COUNT],
    level_tick: Option<SystemTime>,

    /// Where the volume gauge and the sleep label were drawn, for the mouse.
    pub gauge_at: Option<(u16, u16, u16)>,
    pub sleep_at: Option<(u16, u16, u16)>,
    pub area: Rect,
}

struct Frame {
    title: Vec<Seg>,
    sleep: Vec<Seg>,
    rows: Vec<Vec<Seg>>,
    gauge_x: usize,
    gauge_w: usize,
}

fn since(now: SystemTime, t: SystemTime) -> Duration {
    now.duration_since(t).unwrap_or_default()
}

fn before(now: SystemTime, t: Option<SystemTime>) -> bool {
    t.is_some_and(|t| now < t)
}

impl Card {
    pub fn new(vu: bool) -> Self {
        Card { vu, ..Card::default() }
    }

    pub fn update(&mut self, inf: Info, now: SystemTime) {
        let prev = &self.info;
        if self.has_info && inf.volume != prev.volume {
            self.flash(now);
        }
        let prev = &self.info;
        if inf.url != prev.url || inf.station != prev.station || matches!(inf.state, State::Buffering | State::Stopped)
        {
            self.playing_since = None;
        }
        if inf.song != prev.song {
            self.song_since = Some(now);
        }
        self.info = inf;
        self.has_info = true;
        if self.state() == State::Playing && self.playing_since.is_none() {
            self.playing_since = Some(now);
        }
    }

    pub fn flash(&mut self, now: SystemTime) {
        self.flash_until = Some(now + VOLUME_FLASH);
    }

    pub fn flash_sleep(&mut self, now: SystemTime) {
        self.sleep_flash = Some(now + VOLUME_FLASH);
    }

    pub fn state(&self) -> State {
        let inf = &self.info;
        if !self.has_info || inf.station.is_empty() {
            State::Idle
        } else if inf.url.is_empty() {
            State::Stopped
        } else {
            inf.state
        }
    }

    /// The station URL shown as playing, with its state; empty when none.
    pub fn playing_url(&self) -> (&str, State) {
        match self.state() {
            st @ (State::Idle | State::Stopped) => ("", st),
            st => (&self.info.url, st),
        }
    }

    pub fn advance(&mut self, now: SystemTime, src: &dyn MeterSource) {
        let mut dt = self.level_tick.map_or(0.05, |t| now.duration_since(t).map_or(0.05, |d| d.as_secs_f64()));
        if dt > 0.5 {
            dt = 0.05;
        }
        self.level_tick = Some(now);
        let playing = self.state() == State::Playing;
        let fresh = |at: SystemTime| playing && since(now, at) < VU_STALE;

        let target = src.level().filter(|&(_, at)| fresh(at)).map_or(0.0, |(l, _)| l);
        self.level = follow(self.level, target, dt);
        if self.level >= self.peak {
            self.peak = self.level;
            self.peak_at = Some(now);
        } else if self.peak_at.is_none_or(|t| since(now, t) > VU_PEAK_HOLD) {
            self.peak = self.level.max(self.peak - VU_FALL_PER_SEC * dt);
        }

        let bands = src.spectrum().filter(|&(_, at)| fresh(at)).map_or([0.0; BAND_COUNT], |(b, _)| b);
        for (current, target) in self.bands.iter_mut().zip(bands) {
            *current = follow(*current, target, dt);
        }
    }

    /// Whether the card changes over time, so it needs redrawing.
    pub fn animating(&self, now: SystemTime, extras: &Extras) -> bool {
        matches!(self.state(), State::Buffering | State::Playing | State::Failed)
            || before(now, self.flash_until)
            || before(now, self.sleep_flash)
            || extras.sleep.is_some()
            || extras.shuffle.is_some()
            || self.level > 0.0
            || self.peak > 0.0
            || self.bands.iter().any(|&b| b > 0.0)
    }

    fn previous_track(&self) -> Option<&Track> {
        let playing = self.state() == State::Playing;
        self.info.history.iter().rev().find(|t| !playing || t.song != self.info.song)
    }

    fn render(&self, look: &Look, now: SystemTime, w: usize, extras: &Extras) -> Frame {
        let (t, g) = (&look.t, look.g);
        let inf = &self.info;
        let station = inf.station.as_str();
        let st = self.state();
        let strong = t.text().add_modifier(Modifier::BOLD);
        let danger = t.fg(t.danger);

        let (title, mut left) = match st {
            State::Idle => (seg(" Now playing ", t.dim()), vec![seg("Nothing playing", t.text())]),
            State::Stopped => {
                (seg(" Stopped ", t.dim()), vec![seg(format!("{} ", g.stop), t.dim()), seg(station, t.text())])
            }
            State::Buffering => (
                seg(" Tuning in ", t.fg(t.warn)),
                vec![seg(format!("{} ", g.spinner_frame(now)), t.fg(t.warn)), seg(station, strong)],
            ),
            State::Failed => (
                seg(" Signal lost ", danger),
                vec![seg(format!("{} ", g.fail), danger.add_modifier(Modifier::BOLD)), seg(station, strong)],
            ),
            State::Unsupported => (
                seg(" Can't play ", danger),
                vec![seg(format!("{} ", g.fail), danger.add_modifier(Modifier::BOLD)), seg(station, strong)],
            ),
            State::Playing => (
                seg(" Now playing ", t.accent().add_modifier(Modifier::BOLD)),
                vec![seg(format!("{} ", g.play), t.accent()), seg(station, strong)],
            ),
        };
        if extras.bookmarked && !matches!(st, State::Idle | State::Stopped) {
            left.push(seg(format!(" {}", g.star), t.dim()));
        }
        // The dot glows only while there is sound.
        let dot = if st == State::Playing { t.fg(live_glow(look, now)) } else { t.dim() };
        let title = vec![seg(format!(" {}", g.live), dot), title];

        let mut right = Vec::new();
        if matches!(st, State::Playing | State::Buffering) {
            if self.vu {
                right.extend(self.meter(look));
            }
            if inf.bitrate > 0 {
                if !right.is_empty() {
                    right.push(seg("  ", t.text()));
                }
                right.push(seg(format!("{} kb/s", inf.bitrate), t.dim()));
            }
        }
        let mut rows = vec![spread(look, &left, &right, w)];

        let (mut left, mut right) = (Vec::new(), Vec::new());
        match st {
            State::Idle | State::Stopped => left.push(seg("Pick a station, or press * for a random one.", t.dim())),
            State::Buffering => left.push(seg(format!("Buffering{}", g.ellipsis), t.dim())),
            State::Failed => {
                left.push(seg(inf.status.as_str(), danger));
                left.push(seg(format!(" {} retrying", g.dot), t.dim()));
            }
            State::Unsupported => left.push(seg(inf.status.as_str(), danger)),
            State::Playing => {
                if let Some(start) = self.playing_since {
                    right.push(seg(format!("{} on air", clock(since(now, start))), t.dim()));
                }
                if inf.song.is_empty() {
                    left.push(seg(format!("{} ", g.song), t.dim()));
                    left.push(seg("No track information", t.dim()));
                } else {
                    let song = t.fg(t.song);
                    let note = seg(format!("{} ", g.song), song);
                    let mut room = w.saturating_sub(segs_width(std::slice::from_ref(&note)));
                    let rw = segs_width(&right);
                    if rw > 0 {
                        room = room.saturating_sub(rw + 2);
                    }
                    let elapsed = self.song_since.map_or(Duration::ZERO, |s| since(now, s));
                    left.push(note);
                    left.push(seg(marquee(&inf.song, room, elapsed), song));
                }
            }
        }
        rows.push(spread(look, &left, &right, w));

        let mut frame = Frame { title, sleep: self.sleep_label(look, now, extras), rows, gauge_x: 0, gauge_w: 0 };
        let gauges = self.gauges(look, now, w, extras, &mut frame);
        frame.rows.push(gauges);
        frame
    }

    /// Shows "off" briefly after the timer is turned off.
    fn sleep_label(&self, look: &Look, now: SystemTime, extras: &Extras) -> Vec<Seg> {
        let t = &look.t;
        let flashing = before(now, self.sleep_flash);
        let (text, mut style) = match extras.sleep {
            Some((remaining, fade)) => {
                let style = if remaining <= fade { t.fg(t.warn).add_modifier(Modifier::BOLD) } else { t.fg(t.warn) };
                (clock(remaining + Duration::from_millis(999)), style)
            }
            None if flashing => ("off".to_string(), t.fg(t.warn)),
            None => return Vec::new(),
        };
        if flashing {
            style = t.fg(t.bright).add_modifier(Modifier::BOLD);
        }
        vec![seg(format!(" {} {text} ", look.g.sleep), style)]
    }

    fn gauges(&self, look: &Look, now: SystemTime, w: usize, extras: &Extras, f: &mut Frame) -> Vec<Seg> {
        let t = &look.t;
        let (fill, pct) = if before(now, self.flash_until) {
            let bright = t.fg(t.bright).add_modifier(Modifier::BOLD);
            (bright, bright)
        } else {
            (t.accent(), t.text())
        };

        const VOL_LABEL: &str = "vol ";
        let half = if extras.shuffle.is_some() { w.saturating_sub(3) / 2 } else { w };
        let vol_gauge = half.saturating_sub(VOL_LABEL.len() + 5).clamp(4, 30);
        let mut row = vec![seg(VOL_LABEL, t.dim())];
        f.gauge_x = VOL_LABEL.len();
        f.gauge_w = vol_gauge;
        row.extend(gauge(look, vol_gauge, self.info.volume as f64 / 100.0, fill, t.dim()));
        row.push(seg(format!(" {}%", self.info.volume), pct));

        if let Some((remaining, interval)) = extras.shuffle {
            let label =
                if look.g.shuffle.is_empty() { "shuffle ".to_string() } else { format!("{} shuffle ", look.g.shuffle) };
            let time_text = format!(" {}", clock(remaining + Duration::from_millis(999)));
            let label_w = text::width(&label);
            let shuffle_gauge = half.saturating_sub(label_w + time_text.len()).clamp(4, 30);
            let elapsed = 1.0 - remaining.as_secs_f64() / interval.as_secs_f64();
            let mut right = vec![seg(label, t.fg(t.warn))];
            right.extend(gauge(look, shuffle_gauge, elapsed, t.fg(t.warn), t.dim()));
            right.push(seg(time_text, t.text()));
            return spread(look, &row, &right, w);
        }
        if let Some(track) = self.previous_track() {
            const LABEL: &str = "earlier  ";
            let room = w.saturating_sub(segs_width(&row) + 2);
            if room >= LABEL.len() + 8 {
                let earlier = fit_segs(look, &[seg(LABEL, t.dim()), seg(track.song.as_str(), t.dim())], room);
                return spread(look, &row, &earlier, w);
            }
        }
        fit_segs(look, &row, w)
    }

    /// The spectrum, one cell per band with the bass on the left. It shares
    /// the live colour with the card's dot: both show sound.
    fn meter(&self, look: &Look) -> Vec<Seg> {
        const STEPS: f64 = 8.0;
        let (t, g) = (&look.t, look.g);
        self.bands
            .iter()
            .map(|&level| {
                let step = ((level * STEPS).round() as usize).min(STEPS as usize);
                if step == 0 {
                    seg(g.bands[0], t.dim())
                } else {
                    seg(g.bands[(step - 1) * g.bands.len() / STEPS as usize], t.fg(t.live))
                }
            })
            .collect()
    }

    pub fn draw(&mut self, look: &Look, buf: &mut Buffer, area: Rect, now: SystemTime, extras: &Extras) {
        self.area = area;
        self.gauge_at = None;
        self.sleep_at = None;
        super::draw_box(look, buf, area, look.t.dim(), &[]);
        let w = area.width.saturating_sub(4) as usize;
        if w == 0 || area.height < 3 {
            return;
        }
        let f = self.render(look, now, w, extras);
        let x = area.x + 2;
        text::draw_segs(buf, x, area.y, w, &f.title);
        let sw = segs_width(&f.sleep);
        if sw > 0 && segs_width(&f.title) + sw < w {
            let sx = area.right() - 2 - sw as u16;
            text::draw_segs(buf, sx, area.y, sw, &f.sleep);
            self.sleep_at = Some((sx, area.y, sw as u16));
        }
        for (i, row) in f.rows.iter().enumerate().take(area.height as usize - 2) {
            text::draw_segs(buf, x, area.y + 1 + i as u16, w, row);
        }
        if area.height >= 5 {
            self.gauge_at = Some((x + f.gauge_x as u16, area.y + 3, f.gauge_w as u16));
        }
    }

    /// The volume that a click at column x of the gauge sets, in steps of 5.
    pub fn volume_at(&self, x: u16, y: u16) -> Option<i32> {
        let (gx, gy, gw) = self.gauge_at?;
        if y != gy || gw == 0 || x + 1 < gx || x > gx + gw {
            return None;
        }
        let fraction = (x as f64 - gx as f64 + 0.5) / gw as f64;
        let step = crate::audio::player::VOLUME_STEP as f64;
        Some(((fraction * 100.0 / step).round() * step).clamp(0.0, 100.0) as i32)
    }

    pub fn on_sleep_label(&self, x: u16, y: u16) -> bool {
        self.sleep_at.is_some_and(|(sx, sy, sw)| y == sy && x >= sx && x < sx + sw)
    }
}

/// Jumps up to a louder target and falls slowly towards a quieter one.
fn follow(current: f64, target: f64, dt: f64) -> f64 {
    if target >= current { target } else { target.max(current - VU_FALL_PER_SEC * dt) }
}

/// Fades the dot from the live colour to dim and back once per GLOW_PERIOD.
/// The terminal's own colours can't be blended, so there it blinks.
fn live_glow(look: &Look, now: SystemTime) -> Color {
    let t = &look.t;
    let ms = now.duration_since(SystemTime::UNIX_EPOCH).map_or(0, |d| d.as_millis());
    let phase = (ms % GLOW_PERIOD.as_millis()) as f64 / GLOW_PERIOD.as_millis() as f64;
    let glow = 0.5 + 0.5 * (2.0 * std::f64::consts::PI * phase).cos();
    if !t.paints_bg() {
        return if glow < 0.5 { t.dim } else { t.live };
    }
    mix(t.dim, t.live, glow)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ui::{glyphs, theme};
    use std::sync::Arc;

    struct Fake {
        level: Option<(f64, SystemTime)>,
        bands: Option<([f64; BAND_COUNT], SystemTime)>,
    }

    impl MeterSource for Fake {
        fn level(&self) -> Option<(f64, SystemTime)> {
            self.level
        }
        fn spectrum(&self) -> Option<([f64; BAND_COUNT], SystemTime)> {
            self.bands
        }
    }

    fn look() -> Look {
        Look { t: theme::find("nord").unwrap(), g: &glyphs::UNICODE }
    }

    fn row_text(row: &[Seg]) -> String {
        row.iter().map(|s| s.text.as_str()).collect()
    }

    fn playing(song: &str) -> Info {
        Info {
            state: State::Playing,
            station: "Groove".into(),
            url: "http://x".into(),
            song: song.into(),
            volume: 80,
            bitrate: 128,
            ..Info::default()
        }
    }

    #[test]
    fn spectrum_follows_fresh_readings() {
        let now = SystemTime::now();
        let mut card = Card::new(true);
        card.update(playing("A - B"), now);
        let mut bands = [0.0; BAND_COUNT];
        bands[0] = 1.0;
        bands[11] = 0.5;
        card.advance(now, &Fake { level: Some((0.5, now)), bands: Some((bands, now)) });
        let f = card.render(&look(), now, 60, &Extras::default());
        let first = row_text(&f.rows[0]);
        assert!(first.contains("█▁▁▁▁▁▁▁▁▁▁▄"), "{first}");
        assert!(first.ends_with("128 kb/s"), "{first}");

        // Stale readings fall away.
        let later = now + Duration::from_secs(2);
        card.advance(later, &Fake { level: Some((0.5, now)), bands: Some((bands, now)) });
        card.advance(later + Duration::from_millis(400), &Fake { level: None, bands: None });
        assert!(card.bands.iter().all(|&b| b < 1.0));
    }

    #[test]
    fn earlier_hint_shows_previous_song() {
        let now = SystemTime::now();
        let mut card = Card::new(true);
        let mut inf = playing("New - Song");
        let track = |song: &str| Track { song: song.into() };
        inf.history = Arc::new(vec![track("Old - Song"), track("New - Song")]);
        card.update(inf, now);
        let f = card.render(&look(), now, 70, &Extras::default());
        let gauges = row_text(&f.rows[2]);
        assert!(gauges.starts_with("vol "), "{gauges}");
        assert!(gauges.ends_with("earlier  Old - Song"), "{gauges}");
    }

    #[test]
    fn volume_click() {
        let mut card = Card::new(true);
        card.gauge_at = Some((10, 3, 20));
        assert_eq!(card.volume_at(9, 3), Some(0));
        assert_eq!(card.volume_at(19, 3), Some(50));
        assert_eq!(card.volume_at(30, 3), Some(100));
        assert_eq!(card.volume_at(19, 2), None);
    }

    #[test]
    fn live_glow_blinks_on_terminal_theme() {
        let terminal = Look { t: theme::THEMES[0], g: &glyphs::UNICODE };
        let at = |ms| SystemTime::UNIX_EPOCH + Duration::from_millis(ms);
        assert_eq!(live_glow(&terminal, at(0)), terminal.t.live);
        assert_eq!(live_glow(&terminal, at(1000)), terminal.t.dim);
        let nord = look();
        assert_eq!(live_glow(&nord, at(0)), nord.t.live);
        assert_eq!(live_glow(&nord, at(1000)), nord.t.dim);
    }
}
