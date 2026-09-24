use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::mpsc::{Receiver, SyncSender, TrySendError, sync_channel};
use std::sync::{Arc, Mutex, MutexGuard};
use std::thread;
use std::time::{Duration, SystemTime};

use audioadapter_buffers::direct::InterleavedSlice;
use rubato::{Fft, FixedSync, Resampler};

use super::decode::{self, DecodeError, Event, Sink};
use super::meter::BAND_COUNT;
use super::output::{Output, Queue};
use super::source::{self, OpenError};
use crate::log;

pub const DEFAULT_VOLUME: i32 = 80;
pub const VOLUME_STEP: i32 = 5;
const HISTORY_SIZE: usize = 20;
const MAX_RETRY_DELAY: Duration = Duration::from_secs(30);
const RESAMPLE_CHUNK: usize = 1024;

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum State {
    #[default]
    Idle,
    Buffering,
    Playing,
    Stopped,
    /// The stream broke and is retried.
    Failed,
    /// The stream can't be played; no retries.
    Unsupported,
}

impl State {
    fn text(self) -> &'static str {
        match self {
            State::Idle => "",
            State::Buffering => "Buffering...",
            State::Playing => "Playing",
            State::Stopped => "Stopped",
            State::Failed => "Network or stream issues",
            State::Unsupported => "Can't play this stream",
        }
    }
}

#[derive(Clone, Debug)]
pub struct Track {
    pub song: String,
}

#[derive(Clone, Debug, Default)]
pub struct Info {
    pub state: State,
    pub status: String,
    pub station: String,
    pub song: String,
    pub prev_song: String,
    pub url: String,
    pub volume: i32,
    pub bitrate: u32,
    /// Codec and sample rate, such as "mp3 44100 Hz".
    pub format: String,
    /// Replaced, never modified in place, so snapshots can share it.
    pub history: Arc<Vec<Track>>,
}

impl Info {
    fn set_state(&mut self, state: State, reason: &str) {
        self.state = state;
        self.status = state.text().to_string();
        if !reason.is_empty() {
            self.status = format!("{}: {reason}", self.status);
        }
    }
}

/// Plays one station at a time. Its methods are safe to call from any thread
/// and never block on the network.
#[derive(Clone)]
pub struct Player {
    inner: Arc<Inner>,
}

struct Inner {
    output: Output,
    online: bool,
    state: Mutex<PlayerState>,
    listeners: Mutex<Vec<SyncSender<()>>>,
}

struct PlayerState {
    info: Info,
    session: Option<Session>,
}

struct Session {
    id: u64,
    cancel: Arc<AtomicBool>,
    retries: u32,
}

impl Player {
    pub fn new() -> Result<Player, String> {
        Ok(Self::with_output(Output::open()?, true))
    }

    /// A player that only tracks state, never touching the network or a
    /// sound card.
    #[cfg(test)]
    pub fn offline() -> Player {
        Self::with_output(Output::null(), false)
    }

    fn with_output(output: Output, online: bool) -> Player {
        output.set_volume(DEFAULT_VOLUME);
        let info = Info { volume: DEFAULT_VOLUME, ..Info::default() };
        Player {
            inner: Arc::new(Inner {
                output,
                online,
                state: Mutex::new(PlayerState { info, session: None }),
                listeners: Mutex::new(Vec::new()),
            }),
        }
    }

    pub fn snapshot(&self) -> Info {
        self.inner.lock().info.clone()
    }

    /// Receives a message after every change; changes in a row may share one.
    pub fn subscribe(&self) -> Receiver<()> {
        let (tx, rx) = sync_channel(1);
        self.inner.listeners.lock().unwrap().push(tx);
        rx
    }

    /// Plays url, or stops it when it is the one playing.
    pub fn play(&self, station: &str, url: &str) {
        let mut st = self.inner.lock();
        if url == st.info.url {
            self.inner.stop_locked(&mut st);
            return;
        }
        if let Some(old) = st.session.take() {
            old.cancel.store(true, Ordering::Relaxed);
        }
        static NEXT_ID: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(1);
        let id = NEXT_ID.fetch_add(1, Ordering::Relaxed);
        let cancel = Arc::new(AtomicBool::new(false));
        st.session = Some(Session { id, cancel: cancel.clone(), retries: 0 });

        let info = &mut st.info;
        info.station = station.to_string();
        info.url = url.to_string();
        info.set_state(State::Buffering, "");
        info.bitrate = 0;
        info.format.clear();
        info.song.clear();
        info.prev_song.clear();
        drop(st);
        self.inner.notify();

        log!("loading {url}");
        if !self.inner.online {
            return;
        }
        let inner = self.inner.clone();
        let url = url.to_string();
        thread::Builder::new()
            .name("stream".into())
            .spawn(move || inner.run_session(id, &url, &cancel))
            .expect("spawning a thread");
    }

    pub fn stop(&self) {
        let mut st = self.inner.lock();
        if !st.info.url.is_empty() {
            self.inner.stop_locked(&mut st);
        }
    }

    pub fn change_volume(&self, delta: i32) {
        let volume = self.inner.lock().info.volume + delta;
        self.set_volume(volume);
    }

    /// Clamps volume to 0-100.
    pub fn set_volume(&self, volume: i32) {
        let volume = volume.clamp(0, 100);
        let mut st = self.inner.lock();
        if st.info.volume == volume {
            return;
        }
        st.info.volume = volume;
        self.inner.output.set_volume(volume);
        drop(st);
        self.inner.notify();
    }

    /// The level between 0 and 1, and when it was measured.
    pub fn level(&self) -> Option<(f64, SystemTime)> {
        self.inner.output.readings().level()
    }

    /// Band levels between 0 and 1, bass first, and when they were measured.
    pub fn spectrum(&self) -> Option<([f64; BAND_COUNT], SystemTime)> {
        self.inner.output.readings().spectrum()
    }
}

impl Inner {
    fn lock(&self) -> MutexGuard<'_, PlayerState> {
        self.state.lock().unwrap()
    }

    fn notify(&self) {
        self.listeners.lock().unwrap().retain(|tx| !matches!(tx.try_send(()), Err(TrySendError::Disconnected(_))));
    }

    fn stop_locked(&self, st: &mut PlayerState) {
        log!("stopping {}", st.info.url);
        if let Some(s) = st.session.take() {
            s.cancel.store(true, Ordering::Relaxed);
        }
        self.output.silence();
        let info = &mut st.info;
        info.set_state(State::Stopped, "");
        info.url.clear();
        info.song.clear();
        info.prev_song.clear();
        info.bitrate = 0;
        info.format.clear();
        self.notify();
    }

    /// Runs change when session id is still the current one.
    fn update(&self, id: u64, change: impl FnOnce(&mut PlayerState)) {
        let mut st = self.lock();
        if st.session.as_ref().is_some_and(|s| s.id == id) {
            change(&mut st);
            drop(st);
            self.notify();
        }
    }

    fn run_session(self: Arc<Self>, id: u64, url: &str, cancel: &AtomicBool) {
        loop {
            let result = self.play_once(id, url, cancel);
            if cancel.load(Ordering::Relaxed) {
                return;
            }
            let (reason, retry) = match result {
                Ok(()) => return,
                Err(PlayError::Open(OpenError::Unsupported(what))) => (what.to_string(), false),
                Err(PlayError::Decode(DecodeError::Unsupported(what))) => (what, false),
                Err(PlayError::Decode(DecodeError::Ended)) => ("eof".to_string(), true),
                Err(e) => (e.to_string(), true),
            };
            log!("{url}: {reason}");

            let mut delay = Duration::ZERO;
            self.update(id, |st| {
                if retry {
                    let s = st.session.as_mut().unwrap();
                    delay = (Duration::from_secs(1) * 2u32.saturating_pow(s.retries)).min(MAX_RETRY_DELAY);
                    s.retries += 1;
                }
                st.info.set_state(if retry { State::Failed } else { State::Unsupported }, &reason);
                st.info.song.clear();
            });
            if !retry {
                return;
            }
            let until = std::time::Instant::now() + delay;
            while std::time::Instant::now() < until {
                if cancel.load(Ordering::Relaxed) {
                    return;
                }
                thread::sleep(Duration::from_millis(100));
            }
            self.update(id, |st| {
                st.info.set_state(State::Buffering, "");
                st.info.prev_song.clear();
            });
        }
    }

    fn play_once(self: &Arc<Self>, id: u64, url: &str, cancel: &AtomicBool) -> Result<(), PlayError> {
        let titles = self.clone();
        let source = source::open(url, Box::new(move |title| titles.set_song(id, title)))
            .map_err(PlayError::Open)?;
        if cancel.load(Ordering::Relaxed) {
            return Ok(());
        }
        let started = self.clone();
        let mut sink = QueueSink::new(
            self.output.new_queue(),
            self.output.rate,
            cancel,
            Box::new(move || started.set_song(id, String::new())),
        );
        let mut on_event = |event| match event {
            Event::Format(f) => self.update(id, |st| st.info.format = f),
            Event::Bitrate(br) => self.update(id, |st| st.info.bitrate = br),
            Event::Title(t) => self.set_song(id, t),
        };
        decode::run(source, cancel, &mut sink, &mut on_event).map_err(PlayError::Decode)
    }

    /// Marks a buffering stream as playing, and records a new song.
    fn set_song(&self, id: u64, song: String) {
        self.update(id, |st| {
            let info = &mut st.info;
            if info.state == State::Buffering {
                info.set_state(State::Playing, "");
                info.song.clear();
                st.session.as_mut().unwrap().retries = 0;
            }
            if !song.is_empty() && song != info.prev_song {
                info.set_state(State::Playing, "");
                info.song = song.clone();
                info.prev_song = song.clone();
                let mut history: Vec<Track> = info.history.iter().rev().take(HISTORY_SIZE - 1).rev().cloned().collect();
                history.push(Track { song });
                info.history = Arc::new(history);
            }
        });
    }
}

#[derive(Debug)]
enum PlayError {
    Open(OpenError),
    Decode(DecodeError),
}

impl std::fmt::Display for PlayError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            PlayError::Open(e) => e.fmt(f),
            PlayError::Decode(e) => e.fmt(f),
        }
    }
}

/// Resamples to the device rate and queues the audio, waiting while the
/// queue is full.
struct QueueSink<'a> {
    queue: Queue,
    out_rate: u32,
    cancel: &'a AtomicBool,
    on_start: Option<Box<dyn FnOnce() + Send>>,
    written: usize,
    resampler: Option<(u32, Fft<f32>)>,
    pending: Vec<f32>,
    resampled: Vec<f32>,
}

impl<'a> QueueSink<'a> {
    fn new(queue: Queue, out_rate: u32, cancel: &'a AtomicBool, on_start: Box<dyn FnOnce() + Send>) -> Self {
        QueueSink {
            queue,
            out_rate,
            cancel,
            on_start: Some(on_start),
            written: 0,
            resampler: None,
            pending: Vec::new(),
            resampled: Vec::new(),
        }
    }

    fn push(&mut self, mut samples: &[f32]) -> bool {
        while !samples.is_empty() {
            if self.cancel.load(Ordering::Relaxed) {
                return false;
            }
            let n = self.queue.producer.slots().min(samples.len()) & !1;
            if n == 0 {
                thread::sleep(Duration::from_millis(10));
                continue;
            }
            let chunk = self.queue.producer.write_chunk_uninit(n).expect("slots were counted");
            let written = chunk.fill_from_iter(samples[..n].iter().copied());
            samples = &samples[written..];
            self.written += written;
        }
        if self.written >= self.queue.prebuffer {
            if let Some(f) = self.on_start.take() {
                f();
            }
        }
        true
    }

    fn resample(&mut self, frames: &[f32], rate: u32) -> bool {
        if self.resampler.as_ref().is_none_or(|(r, _)| *r != rate) {
            match Fft::new(rate as usize, self.out_rate as usize, RESAMPLE_CHUNK, 2, FixedSync::Input) {
                Ok(r) => self.resampler = Some((rate, r)),
                Err(e) => {
                    log!("resampler {rate} -> {}: {e}", self.out_rate);
                    return false;
                }
            }
            self.pending.clear();
        }
        self.pending.extend_from_slice(frames);
        let mut consumed = 0;
        while self.pending.len() - consumed >= RESAMPLE_CHUNK * 2 {
            let (_, resampler) = self.resampler.as_mut().unwrap();
            let out_frames = resampler.output_frames_next();
            self.resampled.resize(out_frames * 2, 0.0);
            let input = &self.pending[consumed..consumed + RESAMPLE_CHUNK * 2];
            let result = InterleavedSlice::new(input, 2, RESAMPLE_CHUNK).map_err(|e| e.to_string()).and_then(|input| {
                let mut output =
                    InterleavedSlice::new_mut(&mut self.resampled, 2, out_frames).map_err(|e| e.to_string())?;
                resampler.process_into_buffer(&input, &mut output, None).map_err(|e| e.to_string())
            });
            let (read, wrote) = match result {
                Ok(n) => n,
                Err(e) => {
                    log!("resampling: {e}");
                    return false;
                }
            };
            consumed += read * 2;
            let out = std::mem::take(&mut self.resampled);
            let ok = self.push(&out[..wrote * 2]);
            self.resampled = out;
            if !ok {
                return false;
            }
        }
        self.pending.drain(..consumed);
        true
    }
}

impl Sink for QueueSink<'_> {
    fn write(&mut self, frames: &[f32], rate: u32) -> bool {
        if rate == self.out_rate {
            self.push(frames)
        } else {
            self.resample(frames, rate)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::{Read, Write};
    use std::net::TcpListener;
    use std::time::Instant;

    /// Serves body to every connection, counting them.
    fn serve(head: &'static str, body: Vec<u8>) -> (String, Arc<std::sync::atomic::AtomicUsize>) {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let url = format!("http://{}/stream", listener.local_addr().unwrap());
        let count = Arc::new(std::sync::atomic::AtomicUsize::new(0));
        let seen = count.clone();
        thread::spawn(move || {
            for conn in listener.incoming() {
                let Ok(mut conn) = conn else { continue };
                seen.fetch_add(1, Ordering::Relaxed);
                let mut req = [0u8; 1024];
                let _ = conn.read(&mut req);
                let _ = conn.write_all(head.as_bytes());
                let _ = conn.write_all(&body);
            }
        });
        (url, count)
    }

    fn wait_for(p: &Player, what: &str, cond: impl Fn(&Info) -> bool) -> Info {
        let deadline = Instant::now() + Duration::from_secs(10);
        loop {
            let inf = p.snapshot();
            if cond(&inf) {
                return inf;
            }
            assert!(Instant::now() < deadline, "{what}: {inf:?}");
            thread::sleep(Duration::from_millis(20));
        }
    }

    fn online() -> Player {
        Player::with_output(Output::null(), true)
    }

    #[test]
    fn broken_stream_is_retried() {
        let (url, connections) = serve("HTTP/1.0 200 OK\r\nContent-Type: audio/mpeg\r\n\r\n", vec![0x55; 2048]);
        let p = online();
        p.play("Broken", &url);
        let inf = wait_for(&p, "failed", |i| i.state == State::Failed);
        assert!(inf.status.starts_with("Network or stream issues"), "{}", inf.status);
        // The first retry comes after a second.
        wait_for(&p, "retried", |_| connections.load(Ordering::Relaxed) >= 2);
        p.stop();
        let inf = p.snapshot();
        assert_eq!((inf.state, inf.url.as_str()), (State::Stopped, ""));
    }

    #[test]
    fn hls_is_not_retried() {
        let (url, connections) = serve(
            "HTTP/1.0 200 OK\r\nContent-Type: application/vnd.apple.mpegurl\r\n\r\n",
            b"#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:6\nseg1.ts\n".to_vec(),
        );
        let p = online();
        p.play("HLS", &url);
        let inf = wait_for(&p, "unsupported", |i| i.state == State::Unsupported);
        assert!(inf.status.contains("HLS"), "{}", inf.status);
        thread::sleep(Duration::from_millis(1500));
        assert_eq!(connections.load(Ordering::Relaxed), 1);
    }

    #[test]
    fn same_url_toggles_and_history_is_capped() {
        let p = Player::offline();
        p.play("A", "http://a");
        assert_eq!(p.snapshot().state, State::Buffering);
        let id = p.inner.lock().session.as_ref().unwrap().id;
        for i in 0..HISTORY_SIZE + 5 {
            p.inner.set_song(id, format!("Song {i}"));
        }
        let inf = p.snapshot();
        assert_eq!(inf.state, State::Playing);
        assert_eq!(inf.history.len(), HISTORY_SIZE);
        assert_eq!(inf.history.last().unwrap().song, format!("Song {}", HISTORY_SIZE + 4));
        p.play("A", "http://a");
        assert_eq!(p.snapshot().state, State::Stopped);
    }
}
