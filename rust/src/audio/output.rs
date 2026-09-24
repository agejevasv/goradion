//! The sound card side: a cpal stream fed from a ring buffer that each
//! station session replaces.

use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::sync::mpsc;
use std::sync::{Arc, Mutex};
use std::thread;

use cpal::traits::{DeviceTrait, HostTrait, StreamTrait};
use cpal::{FromSample, SampleFormat, SizedSample, StreamConfig};
use rtrb::{Consumer, Producer, RingBuffer};

use super::meter::{Meter, Readings};
use crate::log;

/// Seconds of decoded audio a session may queue. Servers send a burst on
/// connect; keeping it bridges network hiccups.
const QUEUE_SECS: usize = 10;
/// Seconds queued before playback starts.
const PREBUFFER_SECS: f64 = 0.3;
/// Per-frame step of the gain towards its target: about 20 ms to settle.
const GAIN_SMOOTHING: f32 = 0.001;

pub struct Output {
    pub rate: u32,
    shared: Arc<Shared>,
    _stop: Option<mpsc::Sender<()>>,
}

struct Shared {
    consumer: Mutex<Option<Consumer<f32>>>,
    primed: AtomicBool,
    gain: AtomicU32,
    readings: Readings,
}

impl Output {
    /// Opens the default output device. The stream lives on a thread of its
    /// own, as cpal streams can't move between threads on every platform.
    pub fn open() -> Result<Output, String> {
        let shared = Arc::new(Shared {
            consumer: Mutex::new(None),
            primed: AtomicBool::new(false),
            gain: AtomicU32::new(0f32.to_bits()),
            readings: Readings::default(),
        });
        let (ready_tx, ready_rx) = mpsc::channel();
        let (stop_tx, stop_rx) = mpsc::channel::<()>();
        let thread_shared = shared.clone();
        thread::Builder::new()
            .name("audio-output".into())
            .spawn(move || match start_stream(thread_shared) {
                Ok((stream, rate)) => {
                    ready_tx.send(Ok(rate)).ok();
                    let _ = stop_rx.recv();
                    drop(stream);
                }
                Err(e) => {
                    ready_tx.send(Err(e)).ok();
                }
            })
            .map_err(|e| e.to_string())?;
        let rate = ready_rx.recv().map_err(|e| e.to_string())??;
        Ok(Output { rate, shared, _stop: Some(stop_tx) })
    }

    /// An output without a device, whose queues are never played.
    #[cfg(test)]
    pub fn null() -> Output {
        let shared = Arc::new(Shared {
            consumer: Mutex::new(None),
            primed: AtomicBool::new(false),
            gain: AtomicU32::new(0f32.to_bits()),
            readings: Readings::default(),
        });
        Output { rate: 48000, shared, _stop: None }
    }

    /// Starts a new queue, dropping whatever the previous one held.
    pub fn new_queue(&self) -> Queue {
        let (producer, consumer) = RingBuffer::new(self.rate as usize * 2 * QUEUE_SECS);
        self.shared.readings.clear();
        self.shared.primed.store(false, Ordering::Release);
        *self.shared.consumer.lock().unwrap() = Some(consumer);
        Queue { producer, prebuffer: (self.rate as f64 * 2.0 * PREBUFFER_SECS) as usize }
    }

    pub fn silence(&self) {
        *self.shared.consumer.lock().unwrap() = None;
        self.shared.readings.clear();
    }

    /// Volume 0-100 on mpv's cubic curve.
    pub fn set_volume(&self, volume: i32) {
        let gain = (volume.clamp(0, 100) as f32 / 100.0).powi(3);
        self.shared.gain.store(gain.to_bits(), Ordering::Relaxed);
    }

    pub fn readings(&self) -> &Readings {
        &self.shared.readings
    }
}

pub struct Queue {
    pub producer: Producer<f32>,
    pub prebuffer: usize,
}

fn start_stream(shared: Arc<Shared>) -> Result<(cpal::Stream, u32), String> {
    let host = cpal::default_host();
    let device = host.default_output_device().ok_or("no audio output device")?;
    let supported = device.default_output_config().map_err(|e| e.to_string())?;
    let format = supported.sample_format();
    let config: StreamConfig = supported.into();
    log!("audio output: {:?} {:?} {} Hz, {} channels", device.id().ok(), format, config.sample_rate, config.channels);
    let stream = match format {
        SampleFormat::F32 => build::<f32>(&device, &config, shared),
        SampleFormat::I16 => build::<i16>(&device, &config, shared),
        SampleFormat::U16 => build::<u16>(&device, &config, shared),
        SampleFormat::I32 => build::<i32>(&device, &config, shared),
        other => return Err(format!("unsupported output sample format {other}")),
    }?;
    stream.play().map_err(|e| e.to_string())?;
    Ok((stream, config.sample_rate))
}

fn build<T>(device: &cpal::Device, config: &StreamConfig, shared: Arc<Shared>) -> Result<cpal::Stream, String>
where
    T: SizedSample + FromSample<f32>,
{
    let channels = config.channels as usize;
    let rate = config.sample_rate;
    let prebuffer = (rate as f64 * 2.0 * PREBUFFER_SECS) as usize;
    let mut meter = Meter::new(rate);
    let mut gain = 0f32;
    let callback = move |data: &mut [T], _: &cpal::OutputCallbackInfo| {
        let target = f32::from_bits(shared.gain.load(Ordering::Relaxed));
        let frames = data.len() / channels;
        let mut guard = shared.consumer.try_lock().ok();
        let consumer = guard.as_mut().and_then(|g| g.as_mut());

        let mut chunk = None;
        if let Some(c) = consumer {
            let slots = c.slots();
            if !shared.primed.load(Ordering::Acquire) && slots >= prebuffer {
                shared.primed.store(true, Ordering::Release);
            }
            if shared.primed.load(Ordering::Acquire) {
                let n = slots.min(frames * 2) & !1;
                chunk = c.read_chunk(n).ok();
            }
        }
        let (first, second) = chunk.as_ref().map_or((&[][..], &[][..]), |c| c.as_slices());
        let mut samples = first.iter().chain(second).copied();

        for frame in data.chunks_exact_mut(channels) {
            let (l, r) = match (samples.next(), samples.next()) {
                (Some(l), Some(r)) => {
                    meter.feed(l, r, &shared.readings);
                    (l, r)
                }
                _ => (0.0, 0.0),
            };
            gain += (target - gain) * GAIN_SMOOTHING;
            let (l, r) = (l * gain, r * gain);
            match frame {
                [mono] => *mono = T::from_sample(0.5 * (l + r)),
                [left, right, rest @ ..] => {
                    *left = T::from_sample(l);
                    *right = T::from_sample(r);
                    rest.fill(T::from_sample(0.0));
                }
                [] => {}
            }
        }
        if let Some(c) = chunk {
            c.commit_all();
        }
    };
    device
        .build_output_stream(config.clone(), callback, |e| log!("audio output: {e}"), None)
        .map_err(|e| e.to_string())
}
