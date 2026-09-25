//! The sound card side: a cpal stream fed by the mixer, which plays the
//! queues that station sessions fill.

use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::sync::mpsc;
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::Duration;

use cpal::traits::{DeviceTrait, HostTrait, StreamTrait};
use cpal::{ErrorKind, FromSample, SampleFormat, SizedSample, StreamConfig};
use rtrb::{Producer, RingBuffer};

use super::meter::{Meter, Readings};
use super::mixer::{Mixer, PREBUFFER_SECS};
use crate::log;

/// Seconds of decoded audio a session may queue. Servers send a burst on
/// connect; keeping it bridges network hiccups.
const QUEUE_SECS: usize = 10;
/// Per-frame step of the gain towards its target: about 20 ms to settle.
const GAIN_SMOOTHING: f32 = 0.001;

pub struct Output {
    shared: Arc<Shared>,
    _stop: Option<mpsc::Sender<()>>,
}

struct Shared {
    /// The mixer, and the device rate its queues hold audio at.
    mixer: Mutex<Mixer>,
    gain: AtomicU32,
    /// A second gain for fades that leave the volume alone, 0 to 1.
    fade: AtomicU32,
    readings: Readings,
}

impl Shared {
    fn new(rate: u32) -> Arc<Shared> {
        Arc::new(Shared {
            mixer: Mutex::new(Mixer::new(rate)),
            gain: AtomicU32::new(0f32.to_bits()),
            fade: AtomicU32::new(1f32.to_bits()),
            readings: Readings::default(),
        })
    }

    /// A new rate drops the queues, as their audio is at the old one.
    fn set_rate(&self, rate: u32) {
        let mut mixer = self.mixer.lock().unwrap();
        if mixer.rate() != rate {
            log!("audio output: the rate changed from {} Hz to {rate} Hz", mixer.rate());
            *mixer = Mixer::new(rate);
            self.readings.clear();
        }
    }
}

impl Output {
    /// Opens the default output device. The stream lives on a thread of its
    /// own, as cpal streams can't move between threads on every platform.
    pub fn open() -> Result<Output, String> {
        let (ready_tx, ready_rx) = mpsc::channel();
        let (stop_tx, stop_rx) = mpsc::channel::<()>();
        thread::Builder::new()
            .name("audio-output".into())
            .spawn(move || {
                let lost = Arc::new(AtomicBool::new(false));
                let (mut stream, _, shared) = match start_stream(None, lost.clone()) {
                    Ok(started) => started,
                    Err(e) => {
                        ready_tx.send(Err(e)).ok();
                        return;
                    }
                };
                ready_tx.send(Ok(shared.clone())).ok();
                keep_playing(&mut stream, &shared, &lost, &stop_rx);
            })
            .map_err(|e| e.to_string())?;
        let shared = ready_rx.recv().map_err(|e| e.to_string())??;
        Ok(Output { shared, _stop: Some(stop_tx) })
    }

    /// An output without a device, whose queues are never played.
    #[cfg(test)]
    pub fn null() -> Output {
        Output { shared: Shared::new(48000), _stop: None }
    }

    /// Starts a new queue at the device rate, which takes over from the one
    /// playing once it has its prebuffer. A new device rate lets go of it.
    pub fn new_queue(&self) -> Queue {
        let mut mixer = self.shared.mixer.lock().unwrap();
        let rate = mixer.rate();
        let (producer, consumer) = RingBuffer::new(rate as usize * 2 * QUEUE_SECS);
        mixer.switch_to(consumer);
        Queue { producer, rate, prebuffer: (rate as f64 * 2.0 * PREBUFFER_SECS) as usize }
    }

    pub fn silence(&self) {
        self.shared.mixer.lock().unwrap().clear();
        self.shared.readings.clear();
    }

    /// Stops the station being switched from.
    pub fn drop_outgoing(&self) {
        self.shared.mixer.lock().unwrap().drop_outgoing();
    }

    /// Drops the queue tuned in unless it started; reports whether it plays
    /// on until the next one starts.
    pub fn keep_if_started(&self) -> bool {
        self.shared.mixer.lock().unwrap().keep_if_started()
    }

    /// Volume 0-100 on mpv's cubic curve.
    pub fn set_volume(&self, volume: i32) {
        let gain = (volume.clamp(0, 100) as f32 / 100.0).powi(3);
        self.shared.gain.store(gain.to_bits(), Ordering::Relaxed);
    }

    pub fn set_fade(&self, fade: f32) {
        self.shared.fade.store(fade.clamp(0.0, 1.0).to_bits(), Ordering::Relaxed);
    }

    /// What reopening the device at rate does.
    #[cfg(test)]
    pub fn reopen_at(&self, rate: u32) {
        self.shared.set_rate(rate);
    }

    #[cfg(test)]
    pub fn fade(&self) -> f32 {
        f32::from_bits(self.shared.fade.load(Ordering::Relaxed))
    }

    pub fn readings(&self) -> &Readings {
        &self.shared.readings
    }
}

pub struct Queue {
    pub producer: Producer<f32>,
    /// Of the audio the queue takes.
    pub rate: u32,
    pub prebuffer: usize,
}

/// Holds the stream until stop, rebuilding it on the default device when the
/// device goes away, such as Bluetooth headphones switched off. At the same
/// rate the queue stays, so playback goes on where it was.
fn keep_playing(
    stream: &mut Option<cpal::Stream>,
    shared: &Arc<Shared>,
    lost: &Arc<AtomicBool>,
    stop: &mpsc::Receiver<()>,
) {
    loop {
        match stop.recv_timeout(Duration::from_millis(500)) {
            Err(mpsc::RecvTimeoutError::Timeout) => {}
            _ => return,
        }
        if !lost.load(Ordering::Acquire) {
            continue;
        }
        *stream = None;
        lost.store(false, Ordering::Release);
        match start_stream(Some(shared.clone()), lost.clone()) {
            Ok((s, rate, _)) => {
                log!("audio output: reopened at {rate} Hz");
                *stream = s;
            }
            Err(e) => {
                log!("audio output: {e}; retrying");
                lost.store(true, Ordering::Release);
            }
        }
    }
}

/// Opens the default device. The first time, shared is made for its rate;
/// after that, a device at another rate drops the queues, whose stations
/// reconnect at the new rate.
fn start_stream(
    shared: Option<Arc<Shared>>,
    lost: Arc<AtomicBool>,
) -> Result<(Option<cpal::Stream>, u32, Arc<Shared>), String> {
    let host = cpal::default_host();
    let device = host.default_output_device().ok_or("no audio output device")?;
    let supported = device.default_output_config().map_err(|e| e.to_string())?;
    let format = supported.sample_format();
    let config: StreamConfig = supported.into();
    log!("audio output: {:?} {:?} {} Hz, {} channels", device.id().ok(), format, config.sample_rate, config.channels);
    let shared = shared.unwrap_or_else(|| Shared::new(config.sample_rate));
    shared.set_rate(config.sample_rate);
    let stream = match format {
        SampleFormat::F32 => build::<f32>(&device, &config, shared.clone(), lost),
        SampleFormat::I16 => build::<i16>(&device, &config, shared.clone(), lost),
        SampleFormat::U16 => build::<u16>(&device, &config, shared.clone(), lost),
        SampleFormat::I24 => build::<cpal::I24>(&device, &config, shared.clone(), lost),
        SampleFormat::I32 => build::<i32>(&device, &config, shared.clone(), lost),
        other => return Err(format!("unsupported output sample format {other}")),
    }?;
    stream.play().map_err(|e| e.to_string())?;
    Ok((Some(stream), config.sample_rate, shared))
}

fn build<T>(
    device: &cpal::Device,
    config: &StreamConfig,
    shared: Arc<Shared>,
    lost: Arc<AtomicBool>,
) -> Result<cpal::Stream, String>
where
    T: SizedSample + FromSample<f32>,
{
    let channels = config.channels as usize;
    let mut meter = Meter::new(config.sample_rate);
    let mut gain = 0f32;
    let mut mixed: Vec<f32> = Vec::new();
    let callback = move |data: &mut [T], _: &cpal::OutputCallbackInfo| {
        let target =
            f32::from_bits(shared.gain.load(Ordering::Relaxed)) * f32::from_bits(shared.fade.load(Ordering::Relaxed));
        let frames = data.len() / channels;
        // Sized once for the device's block; this only allocates if it grows.
        mixed.resize(frames * 2, 0.0);
        let audible = if let Ok(mut mixer) = shared.mixer.try_lock() {
            mixer.mix(&mut mixed)
        } else {
            mixed.fill(0.0);
            0
        };
        for (i, (frame, &[l, r])) in data.chunks_exact_mut(channels).zip(mixed.as_chunks::<2>().0).enumerate() {
            if i < audible {
                meter.feed(l, r, &shared.readings);
            }
            gain += (target - gain) * GAIN_SMOOTHING;
            // Overs, from loud masters or a crossfade, must clip: 24-bit
            // samples would wrap around.
            let (l, r) = ((l * gain).clamp(-1.0, 1.0), (r * gain).clamp(-1.0, 1.0));
            match frame {
                [mono] => *mono = T::from_sample(f32::midpoint(l, r)),
                [left, right, rest @ ..] => {
                    *left = T::from_sample(l);
                    *right = T::from_sample(r);
                    rest.fill(T::from_sample(0.0));
                }
                [] => {}
            }
        }
    };
    // A broken stream repeats its error until it is dropped, so only the
    // first one is logged.
    let on_error = move |e: cpal::Error| {
        if matches!(e.kind(), ErrorKind::DeviceChanged | ErrorKind::Xrun | ErrorKind::RealtimeDenied) {
            log!("audio output: {e}");
        } else if !lost.swap(true, Ordering::AcqRel) {
            log!("audio output: {e}; reopening");
        }
    };
    device.build_output_stream(*config, callback, on_error, None).map_err(|e| e.to_string())
}
