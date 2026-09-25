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
            let opus = Opus::new(channels, track.delay.unwrap_or(0) as usize)
                .map_err(|e| DecodeError::Unsupported(format!("Opus: {e}")))?;
            return Ok(Decoder::Opus(Box::new(opus)));
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
                out.clear();
                o.decode(&packet.data, out).map_err(|e| Failure::Packet(format!("Opus: {e}")))?;
                Ok((OPUS_RATE, o.channels))
            }
        }
    }
}

impl Opus {
    fn new(channels: usize, skip: usize) -> Result<Opus, &'static str> {
        let decoder = OpusDecoder::new(OPUS_RATE as i32, channels)?;
        Ok(Opus { decoder, channels, skip, buf: vec![0.0; MAX_OPUS_FRAME * channels] })
    }

    /// Appends the packet's audio to out.
    fn decode(&mut self, packet: &[u8], out: &mut Vec<f32>) -> Result<(), &'static str> {
        // opus-rs 0.1.34 reads the frame lengths of a code 3 packet as if each
        // came right before its frame, which turns a packet of three or more
        // frames into noise. Its frames go through one by one instead.
        match packet.first() {
            Some(&toc) if toc & 3 == 3 => {
                for frame in code3_frames(packet).ok_or("malformed packet")? {
                    self.decode_frames(&[&[toc & !3], frame].concat(), out)?;
                }
                Ok(())
            }
            _ => self.decode_frames(packet, out),
        }
    }

    fn decode_frames(&mut self, packet: &[u8], out: &mut Vec<f32>) -> Result<(), &'static str> {
        let frames = self.decoder.decode(packet, MAX_OPUS_FRAME, &mut self.buf)?;
        let skipped = self.skip.min(frames);
        self.skip -= skipped;
        out.extend_from_slice(&self.buf[skipped * self.channels..frames * self.channels]);
        Ok(())
    }
}

/// The frames of a code 3 packet (RFC 6716 3.2.5): after the TOC and count
/// bytes come the padding length, then with VBR the lengths of all frames but
/// the last, then the frames, then the padding.
fn code3_frames(packet: &[u8]) -> Option<Vec<&[u8]>> {
    let (&count, mut rest) = packet.get(1..)?.split_first()?;
    let n = usize::from(count & 0x3f);
    if n == 0 {
        return None;
    }
    if count & 0x40 != 0 {
        let mut padding = 0;
        loop {
            let (&b, tail) = rest.split_first()?;
            rest = tail;
            padding += usize::from(b.min(254));
            if b < 255 {
                break;
            }
        }
        rest = rest.get(..rest.len().checked_sub(padding)?)?;
    }
    if count & 0x80 == 0 {
        if !rest.len().is_multiple_of(n) {
            return None;
        }
        let len = rest.len() / n;
        return Some((0..n).map(|i| &rest[i * len..(i + 1) * len]).collect());
    }
    let mut lens = Vec::with_capacity(n - 1);
    for _ in 1..n {
        let (&b0, tail) = rest.split_first()?;
        rest = tail;
        let mut len = usize::from(b0);
        if b0 >= 252 {
            let (&b1, tail) = rest.split_first()?;
            rest = tail;
            len += 4 * usize::from(b1);
        }
        lens.push(len);
    }
    let mut frames = Vec::with_capacity(n);
    for len in lens {
        let (frame, tail) = rest.split_at_checked(len)?;
        frames.push(frame);
        rest = tail;
    }
    frames.push(rest);
    Some(frames)
}

#[cfg(test)]
mod tests {
    use super::*;
    use opus_rs::{Application, OpusEncoder};

    /// Three 20 ms frames of a stereo tone, each a code 0 packet.
    fn encoded_frames() -> Vec<Vec<u8>> {
        let mut enc = OpusEncoder::new(OPUS_RATE as i32, 2, Application::RestrictedLowDelay).unwrap();
        enc.bitrate_bps = 64_000;
        (0..3)
            .map(|n| {
                let pcm: Vec<f32> = (0..960 * 2)
                    .map(|i| ((n * 960 + i / 2) as f32 * 440.0 * std::f32::consts::TAU / 48_000.0).sin() * 0.3)
                    .collect();
                let mut out = vec![0; 1275];
                let len = enc.encode(&pcm, 960, &mut out).unwrap();
                out.truncate(len);
                out
            })
            .collect()
    }

    #[test]
    fn code3_vbr_packet_decodes_like_its_frames() {
        let packets = encoded_frames();
        let toc = packets[0][0];
        assert!(packets.iter().all(|p| p[0] == toc), "one mode");
        let mut code3 = vec![toc | 3, 0x80 | 3];
        for p in &packets[..2] {
            // Lengths from 252 take two bytes: first + 4 * second.
            let len = p.len() - 1;
            if len < 252 {
                code3.push(len as u8);
            } else {
                let first = 252 + (len - 252) % 4;
                code3.extend([first as u8, ((len - first) / 4) as u8]);
            }
        }
        for p in &packets {
            code3.extend_from_slice(&p[1..]);
        }

        let mut want = Vec::new();
        let mut separate = Opus::new(2, 0).unwrap();
        for p in &packets {
            separate.decode(p, &mut want).unwrap();
        }
        let mut got = Vec::new();
        Opus::new(2, 0).unwrap().decode(&code3, &mut got).unwrap();
        assert_eq!(got.len(), 3 * 960 * 2);
        assert!(got == want, "a code 3 packet must decode as its frames do");
    }

    #[test]
    fn code3_layouts() {
        // CBR: two frames of two bytes; padding of 1 + 254 + 2 bytes.
        let mut padded = vec![0x03, 0x40 | 2, 255, 3, 1, 2, 3, 4];
        padded.resize(padded.len() + 257, 0);
        assert_eq!(code3_frames(&padded), Some(vec![&[1u8, 2][..], &[3, 4]]));
        // VBR: a two-byte length (252 + 4 * 1 = 256), a one-byte one, and the rest.
        let mut vbr = vec![0x03, 0x80 | 3, 252, 1, 2];
        vbr.extend(std::iter::repeat_n(7, 256));
        vbr.extend([8, 8, 9]);
        let frames = code3_frames(&vbr).unwrap();
        assert_eq!(frames.iter().map(|f| f.len()).collect::<Vec<_>>(), [256, 2, 1]);
        assert_eq!((frames[1], frames[2]), (&[8u8, 8][..], &[9u8][..]));
        assert_eq!(code3_frames(&[0x03, 0x80 | 3, 200, 1]), None, "lengths beyond the packet");
        assert_eq!(code3_frames(&[0x03, 3, 1, 2]), None, "CBR bytes not a multiple of the frames");
        assert_eq!(code3_frames(&[0x03]), None);
    }
}
