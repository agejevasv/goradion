//! Plays the queue of the station tuned in, crossfading from the previous one.
//! The previous station keeps playing until the new one has its prebuffer, so
//! switching has no gap however long the new one takes to connect.

use std::f32::consts::FRAC_PI_2;

use rtrb::Consumer;
use rtrb::chunks::ReadChunk;

/// Seconds queued before a station starts.
pub const PREBUFFER_SECS: f64 = 0.3;
const CROSSFADE_SECS: f64 = 1.0;

pub struct Mixer {
    rate: u32,
    current: Option<Consumer<f32>>,
    /// The station switched from, playing out.
    outgoing: Option<Consumer<f32>>,
    /// The current queue reached its prebuffer.
    primed: bool,
    /// Frames of the crossfade played.
    faded: usize,
    fade_len: usize,
    /// In samples.
    prebuffer: usize,
}

impl Mixer {
    pub fn new(rate: u32) -> Self {
        Mixer {
            rate,
            current: None,
            outgoing: None,
            primed: false,
            faded: 0,
            fade_len: (rate as f64 * CROSSFADE_SECS) as usize,
            prebuffer: (rate as f64 * 2.0 * PREBUFFER_SECS) as usize,
        }
    }

    /// Of the audio in the queues.
    pub fn rate(&self) -> u32 {
        self.rate
    }

    /// Tunes in to a new queue. What is audible now plays on until it has
    /// its prebuffer: a current queue that never started is dropped instead.
    pub fn switch_to(&mut self, queue: Consumer<f32>) {
        if self.primed || self.outgoing.is_none() {
            self.outgoing = self.current.take();
        }
        self.current = Some(queue);
        self.primed = false;
        self.faded = 0;
    }

    pub fn clear(&mut self) {
        self.current = None;
        self.outgoing = None;
        self.primed = false;
    }

    /// Stops the station switched from at once.
    pub fn drop_outgoing(&mut self) {
        self.outgoing = None;
    }

    /// Fills out, interleaved stereo; returns the frames that carry audio.
    /// Dropping a queue tells its producer, which then stops.
    pub fn mix(&mut self, out: &mut [f32]) -> usize {
        out.fill(0.0);
        let frames = out.len() / 2;
        if !self.primed && self.current.as_ref().is_some_and(|c| c.slots() >= self.prebuffer) {
            self.primed = true;
        }

        let new = if self.primed { take(&mut self.current, frames) } else { None };
        let old = take(&mut self.outgoing, frames);
        let (new_a, new_b) = new.as_ref().map_or((&[][..], &[][..]), ReadChunk::as_slices);
        let (old_a, old_b) = old.as_ref().map_or((&[][..], &[][..]), ReadChunk::as_slices);
        let at = |a: &[f32], b: &[f32], i: usize| {
            if i < a.len() { a[i] } else { b.get(i - a.len()).copied().unwrap_or(0.0) }
        };

        let fading = old.is_some();
        for (i, frame) in out.as_chunks_mut::<2>().0.iter_mut().enumerate() {
            let (new_gain, old_gain) = match (fading, self.primed) {
                (false, _) => (1.0, 0.0),
                (true, false) => (0.0, 1.0),
                (true, true) => {
                    // Equal power, so the loudness holds through the fade.
                    let x = (self.faded as f32 / self.fade_len.max(1) as f32).min(1.0) * FRAC_PI_2;
                    self.faded += 1;
                    (x.sin(), x.cos())
                }
            };
            for (ch, sample) in frame.iter_mut().enumerate() {
                let k = i * 2 + ch;
                *sample = at(new_a, new_b, k) * new_gain + at(old_a, old_b, k) * old_gain;
            }
        }
        let len = |c: &Option<ReadChunk<'_, f32>>| c.as_ref().map_or(0, ReadChunk::len);
        let audible = len(&new).max(len(&old)) / 2;
        if let Some(c) = new {
            c.commit_all();
        }
        if let Some(c) = old {
            c.commit_all();
        }
        if self.primed && self.faded >= self.fade_len {
            self.outgoing = None;
        }
        audible
    }
}

/// Up to frames of the queue's audio.
fn take(queue: &mut Option<Consumer<f32>>, frames: usize) -> Option<ReadChunk<'_, f32>> {
    let c = queue.as_mut()?;
    let n = c.slots().min(frames * 2) & !1;
    c.read_chunk(n).ok()
}

#[cfg(test)]
// Outside the fade the gains are exactly 1 and 0, so samples pass unchanged.
#[allow(clippy::float_cmp)]
mod tests {
    use super::*;
    use rtrb::{Producer, RingBuffer};

    const RATE: u32 = 1000;

    fn queue() -> (Producer<f32>, Consumer<f32>) {
        RingBuffer::new(RATE as usize * 2 * 10)
    }

    fn fill(p: &mut Producer<f32>, value: f32, frames: usize) {
        for _ in 0..frames * 2 {
            p.push(value).unwrap();
        }
    }

    /// Mixes n frames and returns the left channel.
    fn run(m: &mut Mixer, frames: usize) -> Vec<f32> {
        let mut out = vec![0.0; frames * 2];
        m.mix(&mut out);
        out.iter().step_by(2).copied().collect()
    }

    #[test]
    fn old_plays_until_new_is_primed_then_crossfades() {
        let mut m = Mixer::new(RATE);
        let (mut old, c) = queue();
        m.switch_to(c);
        fill(&mut old, 1.0, 2000);
        assert!(run(&mut m, 100).iter().all(|&s| s == 1.0));

        let (mut new, c) = queue();
        m.switch_to(c);
        // Not primed yet: the old station goes on alone.
        fill(&mut new, -1.0, 100);
        assert!(run(&mut m, 100).iter().all(|&s| s == 1.0));

        fill(&mut new, -1.0, 1500);
        let fade = run(&mut m, 1000);
        assert!((fade[0] - 1.0).abs() < 0.01, "{}", fade[0]);
        assert!(fade[500].abs() < 0.01, "equal power midpoint: {}", fade[500]);
        assert!((fade[999] + 1.0).abs() < 0.01, "{}", fade[999]);
        assert!(run(&mut m, 10).iter().all(|&s| s == -1.0));
        assert!(old.is_abandoned(), "the old station must be told to stop");
        assert!(!new.is_abandoned());
    }

    #[test]
    fn switching_again_before_start_keeps_what_is_audible() {
        let mut m = Mixer::new(RATE);
        let (mut a, c) = queue();
        m.switch_to(c);
        fill(&mut a, 1.0, 5000);
        run(&mut m, 10);
        let (b, c) = queue();
        m.switch_to(c);
        let (mut c_producer, c) = queue();
        m.switch_to(c);
        assert!(b.is_abandoned(), "B never started, so it goes");
        assert!(!a.is_abandoned());
        assert!(run(&mut m, 10).iter().all(|&s| s == 1.0));
        fill(&mut c_producer, -1.0, 2000);
        run(&mut m, 1100);
        assert!(a.is_abandoned());
    }

    #[test]
    fn clear_stops_everything() {
        let mut m = Mixer::new(RATE);
        let (mut a, c) = queue();
        m.switch_to(c);
        fill(&mut a, 1.0, 500);
        run(&mut m, 10);
        let (b, c) = queue();
        m.switch_to(c);
        m.clear();
        assert!(a.is_abandoned() && b.is_abandoned());
        assert!(run(&mut m, 10).iter().all(|&s| s == 0.0));
    }
}
