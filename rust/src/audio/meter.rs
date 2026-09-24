//! Level and spectrum of the audio as it is played. The numbers follow the
//! mpv/FFmpeg filters of the Go version, so the bars look the same.

use std::f64::consts::{LN_2, PI};
use std::sync::Mutex;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

pub const BAND_COUNT: usize = 12;

/// Band centres in Hz, bass first: 40 Hz to 12 kHz in steps of about three
/// quarters of an octave.
const BAND_FREQS: [f64; BAND_COUNT] =
    [40.0, 67.0, 113.0, 190.0, 318.0, 535.0, 898.0, 1508.0, 2533.0, 4254.0, 7145.0, 12000.0];

/// Bars span `SPECTRUM_FLOOR_DB` to 0 dBFS. Band-limited RMS sits well below the
/// overall level, and music carries less energy in the treble, so the tilt
/// lowers the bass and raises the treble by half of `SPECTRUM_TILT_DB` each.
const SPECTRUM_FLOOR_DB: f64 = -50.0;
const SPECTRUM_TILT_DB: f64 = 10.0;
const LEVEL_FLOOR_DB: f64 = -32.0;

/// Frames per reading, about what `FFmpeg`'s astats saw per frame.
const WINDOW: usize = 1024;

#[derive(Default)]
pub struct Readings {
    level_bits: AtomicU64,
    level_at: AtomicU64,
    bands: Mutex<[f64; BAND_COUNT]>,
    bands_at: AtomicU64,
}

impl Readings {
    pub fn level(&self) -> Option<(f64, SystemTime)> {
        let at = self.level_at.load(Ordering::Acquire);
        (at != 0).then(|| (f64::from_bits(self.level_bits.load(Ordering::Relaxed)), from_nanos(at)))
    }

    pub fn spectrum(&self) -> Option<([f64; BAND_COUNT], SystemTime)> {
        let at = self.bands_at.load(Ordering::Acquire);
        (at != 0).then(|| (*self.bands.lock().unwrap(), from_nanos(at)))
    }

    pub fn clear(&self) {
        self.level_at.store(0, Ordering::Release);
        self.bands_at.store(0, Ordering::Release);
    }
}

/// Runs in the audio callback, so it neither allocates nor blocks.
pub struct Meter {
    filters: [Biquad; BAND_COUNT],
    sum_left: f64,
    sum_right: f64,
    band_sums: [f64; BAND_COUNT],
    frames: usize,
    rate: u32,
}

impl Meter {
    pub fn new(rate: u32) -> Self {
        Meter {
            filters: BAND_FREQS.map(|f| Biquad::bandpass(f, rate as f64)),
            sum_left: 0.0,
            sum_right: 0.0,
            band_sums: [0.0; BAND_COUNT],
            frames: 0,
            rate,
        }
    }

    pub fn feed(&mut self, left: f32, right: f32, out: &Readings) {
        let (l, r) = (left as f64, right as f64);
        self.sum_left += l * l;
        self.sum_right += r * r;
        let mono = f64::midpoint(l, r);
        for (sum, filter) in self.band_sums.iter_mut().zip(&mut self.filters) {
            let y = filter.process(mono);
            *sum += y * y;
        }
        self.frames += 1;
        if self.frames == WINDOW {
            self.publish(out);
        }
    }

    fn publish(&mut self, out: &Readings) {
        let n = self.frames as f64;
        let level_db = to_db(self.sum_left.max(self.sum_right) / n);
        let now = now_nanos();
        out.level_bits.store(level(level_db).to_bits(), Ordering::Relaxed);
        out.level_at.store(now, Ordering::Release);
        if let Ok(mut bands) = out.bands.try_lock() {
            for (i, sum) in self.band_sums.iter().enumerate() {
                bands[i] = band_level(i, to_db(sum / n), self.rate);
            }
            out.bands_at.store(now, Ordering::Release);
        }
        self.sum_left = 0.0;
        self.sum_right = 0.0;
        self.band_sums = [0.0; BAND_COUNT];
        self.frames = 0;
    }
}

fn to_db(mean_square: f64) -> f64 {
    10.0 * mean_square.log10()
}

pub fn level(db: f64) -> f64 {
    if db.is_nan() {
        return 0.0;
    }
    (1.0 - db / LEVEL_FLOOR_DB).clamp(0.0, 1.0)
}

/// A band centred at or above half the sample rate reads as silent.
pub fn band_level(band: usize, db: f64, rate: u32) -> f64 {
    if db.is_nan() || 2.0 * BAND_FREQS[band] >= rate as f64 {
        return 0.0;
    }
    let tilt = SPECTRUM_TILT_DB * (band as f64 / (BAND_COUNT - 1) as f64 - 0.5);
    (1.0 - (db + tilt) / SPECTRUM_FLOOR_DB).clamp(0.0, 1.0)
}

/// `FFmpeg`'s `bandpass` with a one-octave width: the RBJ band-pass with 0 dB
/// peak gain.
#[derive(Clone, Copy)]
struct Biquad {
    b0: f64,
    b2: f64,
    a1: f64,
    a2: f64,
    x1: f64,
    x2: f64,
    y1: f64,
    y2: f64,
}

impl Biquad {
    fn bandpass(freq: f64, rate: f64) -> Self {
        let w0 = 2.0 * PI * freq.min(rate * 0.49) / rate;
        let alpha = w0.sin() * (LN_2 / 2.0 * w0 / w0.sin()).sinh();
        let a0 = 1.0 + alpha;
        Biquad {
            b0: alpha / a0,
            b2: -alpha / a0,
            a1: -2.0 * w0.cos() / a0,
            a2: (1.0 - alpha) / a0,
            x1: 0.0,
            x2: 0.0,
            y1: 0.0,
            y2: 0.0,
        }
    }

    fn process(&mut self, x: f64) -> f64 {
        let y = self.b0 * x + self.b2 * self.x2 - self.a1 * self.y1 - self.a2 * self.y2;
        self.x2 = self.x1;
        self.x1 = x;
        self.y2 = self.y1;
        // Flush denormals, which make silence slow.
        self.y1 = if y.abs() < 1e-30 { 0.0 } else { y };
        y
    }
}

fn now_nanos() -> u64 {
    SystemTime::now().duration_since(UNIX_EPOCH).map_or(1, |d| d.as_nanos() as u64)
}

fn from_nanos(n: u64) -> SystemTime {
    UNIX_EPOCH + Duration::from_nanos(n)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn sine_band_readings(freq: f64) -> [f64; BAND_COUNT] {
        let rate = 48000;
        let mut meter = Meter::new(rate);
        let readings = Readings::default();
        for i in 0..rate as usize {
            let s = (2.0 * PI * freq * i as f64 / rate as f64).sin() as f32 * 0.5;
            meter.feed(s, s, &readings);
        }
        readings.spectrum().unwrap().0
    }

    #[test]
    fn sine_lights_its_band() {
        for (band, &freq) in BAND_FREQS.iter().enumerate() {
            let bands = sine_band_readings(freq);
            let loudest = (0..BAND_COUNT).max_by(|&a, &b| bands[a].total_cmp(&bands[b])).unwrap();
            assert_eq!(loudest, band, "{freq} Hz: {bands:?}");
        }
    }

    #[test]
    fn level_of_half_scale_sine() {
        let mut meter = Meter::new(44100);
        let readings = Readings::default();
        for i in 0..WINDOW * 4 {
            let s = (2.0 * PI * 1000.0 * i as f64 / 44100.0).sin() as f32 * 0.5;
            meter.feed(s, s, &readings);
        }
        // -9 dBFS RMS against the -32 dB floor.
        let (lvl, _) = readings.level().unwrap();
        assert!((lvl - (1.0 - 9.03 / 32.0)).abs() < 0.01, "{lvl}");
    }

    #[test]
    #[allow(clippy::float_cmp)] // clamped to exactly 0 or 1
    fn levels_match_go() {
        assert_eq!(level(f64::NAN), 0.0);
        assert_eq!(level(-40.0), 0.0);
        assert_eq!(level(0.0), 1.0);
        assert_eq!(band_level(11, -20.0, 22050), 0.0);
        assert!((band_level(0, -20.0, 44100) - (1.0 - (-25.0 / -50.0))).abs() < 1e-9);
    }
}
