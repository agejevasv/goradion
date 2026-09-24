//! Demuxes and decodes a stream into interleaved stereo f32.

use std::sync::atomic::{AtomicBool, Ordering};

use symphonia::core::codecs::audio::{AudioDecoder, AudioDecoderOptions};
use symphonia::core::errors::Error as SymError;
use symphonia::core::formats::probe::Hint;
use symphonia::core::formats::{FormatOptions, FormatReader, TrackType};
use symphonia::core::io::{MediaSourceStream, ReadOnlySource};
use symphonia::core::meta::{MetadataOptions, MetadataRevision, StandardTag};

use super::source::Source;

/// Consecutive undecodable packets before the stream counts as broken.
const MAX_BAD_PACKETS: usize = 50;
const BITRATE_WINDOW_SECS: f64 = 2.0;

pub trait Sink {
    /// Receives interleaved stereo frames; false stops decoding.
    fn write(&mut self, frames: &[f32], rate: u32) -> bool;
}

pub enum Event {
    Format(String),
    Bitrate(u32),
    Title(String),
}

#[derive(Debug)]
pub enum DecodeError {
    Ended,
    Unsupported(String),
    Failed(String),
}

impl std::fmt::Display for DecodeError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            DecodeError::Ended => f.write_str("stream ended"),
            DecodeError::Unsupported(s) | DecodeError::Failed(s) => f.write_str(s),
        }
    }
}

impl From<SymError> for DecodeError {
    fn from(e: SymError) -> Self {
        match e {
            SymError::Unsupported(what) => DecodeError::Unsupported(what.to_string()),
            SymError::IoError(e) if e.kind() == std::io::ErrorKind::UnexpectedEof => DecodeError::Ended,
            e => DecodeError::Failed(e.to_string()),
        }
    }
}

/// Decodes until the stream ends, fails, `cancel` is set or the sink refuses
/// more. Returns Ok only when stopped by `cancel` or the sink.
pub fn run(
    source: Source,
    cancel: &AtomicBool,
    sink: &mut dyn Sink,
    on_event: &mut dyn FnMut(Event),
) -> Result<(), DecodeError> {
    let reader = SyncReader(std::sync::Mutex::new(source.reader));
    let mss = MediaSourceStream::new(Box::new(ReadOnlySource::new(reader)), Default::default());
    let mut hint = Hint::new();
    if let Some(mime) = &source.mime {
        hint.mime_type(mime);
    }
    let mut format = symphonia::default::get_probe().probe(
        &hint,
        mss,
        FormatOptions::default(),
        MetadataOptions::default(),
    )?;

    let (mut track_id, mut decoder) = open_decoder(format.as_ref())?;
    on_event(Event::Format(describe(decoder.as_ref())));
    if let Some(br) = source.bitrate {
        on_event(Event::Bitrate(br));
    }
    let mut last_title = String::new();
    let mut report_title = |rev: Option<&MetadataRevision>, on_event: &mut dyn FnMut(Event)| {
        if let Some(title) = rev.and_then(song_title) {
            if title != last_title {
                last_title = title.clone();
                on_event(Event::Title(title));
            }
        }
    };
    report_title(format.metadata().skip_to_latest(), on_event);

    let mut samples: Vec<f32> = Vec::new();
    let mut stereo: Vec<f32> = Vec::new();
    let mut bad_packets = 0;
    let (mut window_bytes, mut window_frames) = (0usize, 0usize);

    while !cancel.load(Ordering::Relaxed) {
        let packet = match format.next_packet() {
            Ok(Some(p)) => p,
            Ok(None) => return Err(DecodeError::Ended),
            Err(SymError::ResetRequired) => {
                (track_id, decoder) = open_decoder(format.as_ref())?;
                on_event(Event::Format(describe(decoder.as_ref())));
                continue;
            }
            Err(e) => return Err(e.into()),
        };
        if !format.metadata().is_latest() {
            report_title(format.metadata().skip_to_latest(), on_event);
        }
        if packet.track_id != track_id {
            continue;
        }

        let buf = match decoder.decode(&packet) {
            Ok(buf) => buf,
            Err(SymError::DecodeError(e)) => {
                bad_packets += 1;
                if bad_packets > MAX_BAD_PACKETS {
                    return Err(DecodeError::Failed(e.to_string()));
                }
                continue;
            }
            Err(e) => return Err(e.into()),
        };
        bad_packets = 0;
        if buf.frames() == 0 {
            continue;
        }

        let rate = buf.spec().rate();
        let channels = buf.spec().channels().count().max(1);
        buf.copy_to_vec_interleaved(&mut samples);
        to_stereo(&samples, channels, &mut stereo);

        if source.bitrate.is_none() {
            window_bytes += packet.data.len();
            window_frames += stereo.len() / 2;
            let secs = window_frames as f64 / rate as f64;
            if secs >= BITRATE_WINDOW_SECS {
                on_event(Event::Bitrate((window_bytes as f64 * 8.0 / secs / 1000.0).round() as u32));
                (window_bytes, window_frames) = (0, 0);
            }
        }

        if !sink.write(&stereo, rate) {
            return Ok(());
        }
    }
    Ok(())
}

/// Symphonia wants a Sync source; the Mutex provides it without locking, as
/// reads go through get_mut.
struct SyncReader(std::sync::Mutex<Box<dyn std::io::Read + Send>>);

impl std::io::Read for SyncReader {
    fn read(&mut self, buf: &mut [u8]) -> std::io::Result<usize> {
        self.0.get_mut().unwrap().read(buf)
    }
}

fn open_decoder(format: &dyn FormatReader) -> Result<(u32, Box<dyn AudioDecoder>), DecodeError> {
    let track = format
        .first_track_known_codec(TrackType::Audio)
        .ok_or_else(|| DecodeError::Unsupported("no supported audio track".into()))?;
    let params = track
        .codec_params
        .as_ref()
        .and_then(|p| p.audio())
        .ok_or_else(|| DecodeError::Unsupported("no audio parameters".into()))?;
    let decoder = symphonia::default::get_codecs()
        .make_audio_decoder(params, &AudioDecoderOptions::default())
        .map_err(|e| match e {
            SymError::Unsupported(what) => DecodeError::Unsupported(format!("unsupported codec ({what})")),
            e => e.into(),
        })?;
    Ok((track.id, decoder))
}

fn describe(decoder: &dyn AudioDecoder) -> String {
    let info = decoder.codec_info();
    let params = decoder.codec_params();
    let profile = params
        .profile
        .and_then(|p| info.profiles.iter().find(|i| i.profile == p))
        .map(|i| format!(" {}", i.short_name))
        .unwrap_or_default();
    match params.sample_rate {
        Some(rate) => format!("{}{} {} Hz", info.short_name, profile, rate),
        None => format!("{}{}", info.short_name, profile),
    }
}

/// Artist and title from Vorbis comments or ID3 tags; a lone title will do.
fn song_title(rev: &MetadataRevision) -> Option<String> {
    let tags = rev.media.tags.iter().chain(rev.per_track.iter().flat_map(|t| t.metadata.tags.iter()));
    let (mut artist, mut title) = (None, None);
    for tag in tags {
        match &tag.std {
            Some(StandardTag::Artist(a)) if !a.trim().is_empty() => artist = Some(a.trim().to_string()),
            Some(StandardTag::TrackTitle(t)) if !t.trim().is_empty() => title = Some(t.trim().to_string()),
            _ => {}
        }
    }
    match (artist, title) {
        (Some(a), Some(t)) => Some(format!("{a} - {t}")),
        (_, t) => t,
    }
}

/// Mono is duplicated; beyond two channels, front left and right are kept.
fn to_stereo(samples: &[f32], channels: usize, out: &mut Vec<f32>) {
    out.clear();
    match channels {
        1 => out.extend(samples.iter().flat_map(|&s| [s, s])),
        2 => out.extend_from_slice(samples),
        n => out.extend(samples.chunks_exact(n).flat_map(|f| [f[0], f[1]])),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn stereo_mapping() {
        let mut out = Vec::new();
        to_stereo(&[0.1, 0.2], 1, &mut out);
        assert_eq!(out, [0.1, 0.1, 0.2, 0.2]);
        to_stereo(&[1.0, 2.0, 3.0, 4.0, 5.0, 6.0], 3, &mut out);
        assert_eq!(out, [1.0, 2.0, 4.0, 5.0]);
    }
}
