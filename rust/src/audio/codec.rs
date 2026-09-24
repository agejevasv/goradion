//! Decoders: symphonia's, and a pure-Rust Opus decoder for the Opus tracks
//! that symphonia demuxes but can't decode.

use opus_rs::OpusDecoder;
use symphonia::core::audio::Channels;
use symphonia::core::codecs::audio::well_known::CODEC_ID_OPUS;
use symphonia::core::codecs::audio::{AudioDecoder, AudioDecoderOptions};
use symphonia::core::errors::Error as SymError;
use symphonia::core::formats::Track;
use symphonia::core::packet::Packet;

use super::decode::DecodeError;

const OPUS_RATE: u32 = 48_000;
/// 120 ms at 48 kHz, the longest Opus packet.
const MAX_OPUS_FRAME: usize = 5760;

pub enum Decoder {
    Symphonia(Box<dyn AudioDecoder>),
    Opus(Box<Opus>),
}

pub struct Opus {
    decoder: OpusDecoder,
    channels: usize,
    /// Frames still to drop at the start: the encoder's pre-skip.
    skip: usize,
    buf: Vec<f32>,
}

pub enum Failure {
    /// The packet is lost; decoding goes on.
    Packet(String),
    Stream(DecodeError),
}

impl Decoder {
    pub fn open(track: &Track) -> Result<Decoder, DecodeError> {
        let params = track
            .codec_params
            .as_ref()
            .and_then(|p| p.audio())
            .ok_or_else(|| DecodeError::Unsupported("no audio parameters".into()))?;
        if params.codec == CODEC_ID_OPUS {
            let channels = params.channels.as_ref().map_or(2, Channels::count);
            if !(1..=2).contains(&channels) {
                return Err(DecodeError::Unsupported(format!("Opus with {channels} channels")));
            }
            let decoder = OpusDecoder::new(OPUS_RATE as i32, channels)
                .map_err(|e| DecodeError::Unsupported(format!("Opus: {e}")))?;
            let buf = vec![0.0; MAX_OPUS_FRAME * channels];
            let skip = track.delay.unwrap_or(0) as usize;
            return Ok(Decoder::Opus(Box::new(Opus { decoder, channels, skip, buf })));
        }
        symphonia::default::get_codecs()
            .make_audio_decoder(params, &AudioDecoderOptions::default())
            .map(Decoder::Symphonia)
            .map_err(|e| match e {
                SymError::Unsupported(what) => DecodeError::Unsupported(format!("unsupported codec ({what})")),
                e => e.into(),
            })
    }

    /// Codec and sample rate, such as "mp3 44100 Hz".
    pub fn describe(&self) -> String {
        match self {
            Decoder::Opus(_) => format!("opus {OPUS_RATE} Hz"),
            Decoder::Symphonia(d) => {
                let info = d.codec_info();
                let params = d.codec_params();
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
        }
    }

    /// Decodes a packet into out, interleaved; returns the sample rate and the
    /// channel count.
    pub fn decode(&mut self, packet: &Packet, out: &mut Vec<f32>) -> Result<(u32, usize), Failure> {
        match self {
            Decoder::Symphonia(d) => match d.decode(packet) {
                Ok(buf) => {
                    buf.copy_to_vec_interleaved(out);
                    Ok((buf.spec().rate(), buf.spec().channels().count().max(1)))
                }
                Err(SymError::DecodeError(e)) => Err(Failure::Packet(e.to_string())),
                Err(e) => Err(Failure::Stream(e.into())),
            },
            Decoder::Opus(o) => {
                let frames = o
                    .decoder
                    .decode(&packet.data, MAX_OPUS_FRAME, &mut o.buf)
                    .map_err(|e| Failure::Packet(format!("Opus: {e}")))?;
                let skipped = o.skip.min(frames);
                o.skip -= skipped;
                out.clear();
                out.extend_from_slice(&o.buf[skipped * o.channels..frames * o.channels]);
                Ok((OPUS_RATE, o.channels))
            }
        }
    }
}
