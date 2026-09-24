//! `-c`: plays a few seconds of every station through the decoder, without
//! a sound card, and reports what can't be played.

use std::collections::BTreeMap;
use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
use std::sync::Mutex;
use std::thread;

use crate::audio::decode::{self, DecodeError, Event, Sink};
use crate::audio::source::{self, OpenError};
use crate::log;
use crate::stations::Station;

const WORKERS: usize = 16;
const SECONDS: f64 = 3.0;

struct Counter {
    frames: usize,
}

impl Sink for Counter {
    fn write(&mut self, frames: &[f32], rate: u32) -> bool {
        self.frames += frames.len() / 2;
        (self.frames as f64) < SECONDS * rate as f64
    }
}

enum Outcome {
    Played(String),
    Unsupported(String),
    Dead(String),
}

fn check(url: &str) -> Outcome {
    let owned = url.to_string();
    let source = match source::open(url, Box::new(move |t| log!("{owned}: icy title {t:?}"))) {
        Ok(s) => s,
        Err(OpenError::Unsupported(what)) => return Outcome::Unsupported(what.to_string()),
        Err(e) => return Outcome::Dead(e.to_string()),
    };
    let mut format = String::new();
    let result = decode::run(source, &AtomicBool::new(false), &mut Counter { frames: 0 }, &mut |e| {
        match e {
            Event::Format(f) => format = f,
            Event::Title(t) => log!("{url}: title {t:?}"),
            Event::Bitrate(b) => log!("{url}: {b} kbps"),
        }
    });
    match result {
        Ok(()) => {
            log!("{url}: {format}");
            Outcome::Played(format)
        }
        Err(DecodeError::Unsupported(what)) => Outcome::Unsupported(what),
        Err(e) => Outcome::Dead(e.to_string()),
    }
}

pub fn run(stations: &[Station]) {
    let next = AtomicUsize::new(0);
    let outcomes: Mutex<Vec<Option<Outcome>>> = Mutex::new((0..stations.len()).map(|_| None).collect());
    thread::scope(|s| {
        for _ in 0..WORKERS {
            s.spawn(|| {
                loop {
                    let i = next.fetch_add(1, Ordering::Relaxed);
                    let Some(station) = stations.get(i) else { break };
                    let outcome = check(&station.url);
                    outcomes.lock().unwrap()[i] = Some(outcome);
                }
            });
        }
    });

    let mut formats: BTreeMap<String, usize> = BTreeMap::new();
    let (mut unsupported, mut dead) = (0, 0);
    for (station, outcome) in stations.iter().zip(outcomes.into_inner().unwrap()) {
        match outcome.expect("every station was checked") {
            Outcome::Played(format) => {
                let codec = format.split_whitespace().next().unwrap_or("?").to_string();
                *formats.entry(codec).or_default() += 1;
            }
            Outcome::Unsupported(why) => {
                println!("  UNSUPPORTED  {}  ({})  {why}", station.title, station.url);
                unsupported += 1;
            }
            Outcome::Dead(why) => {
                println!("  DEAD  {}  ({})  {why}", station.title, station.url);
                dead += 1;
            }
        }
    }
    let played = stations.len() - unsupported - dead;
    let by_codec: Vec<String> = formats.iter().map(|(c, n)| format!("{c} {n}")).collect();
    println!(
        "\n{played}/{} stations play ({}), {unsupported} unsupported, {dead} dead",
        stations.len(),
        by_codec.join(", ")
    );
}
