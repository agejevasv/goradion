mod audio;
mod bookmarks;
mod check;
mod config;
mod files;
mod log;
mod radiobrowser;
mod remote;
mod stations;
mod ui;

use std::io::Write as _;
use std::process::ExitCode;

const USAGE: &str = "usage: goradion [-s stations.csv|URL] [-r [key]] [-p port] [-c] [-d] [-v] [--ascii] [--no-vu]

  -s    a link or a path to a stations.csv file
  -r    start the phone remote at launch, with an optional access key (\"\" for none)
  -p    preferred port for the phone remote (Ctrl+P)
  -c    check all streams
  -d    debug log (goradion.log in the current directory)
  -v    show the version and quit";

fn usage() -> ExitCode {
    eprintln!("{USAGE}");
    ExitCode::FAILURE
}

pub fn version_string() -> String {
    format!("goradion v{} ({}/{})", env!("CARGO_PKG_VERSION"), std::env::consts::ARCH, std::env::consts::OS)
}

fn main() -> ExitCode {
    let mut args = std::env::args().skip(1).peekable();
    let (mut remote, mut remote_port): (Option<Option<String>>, Option<u16>) = (None, None);
    let (mut source, mut check, mut play) = (String::new(), false, None);
    let (mut ascii, mut vu) = (ui::glyphs::detect_ascii(), true);
    while let Some(arg) = args.next() {
        // As Go's flag package reads them: -name or --name, with the value
        // next or joined by "=". A key for -r that starts with "-" must be
        // joined.
        let flag = arg.strip_prefix("--").or_else(|| arg.strip_prefix('-')).unwrap_or_default();
        let (name, joined) = flag.split_once('=').map_or((flag, None), |(n, v)| (n, Some(v.to_string())));
        match (name, joined) {
            ("v", None) => {
                println!("{}", version_string());
                return ExitCode::SUCCESS;
            }
            ("d", None) => {
                if let Err(e) = log::init() {
                    eprintln!("{e}");
                    return ExitCode::FAILURE;
                }
            }
            ("c", None) => check = true,
            ("ascii", None) => ascii = true,
            ("no-vu", None) => vu = false,
            ("r", key) => remote = Some(key.or_else(|| args.next_if(|a| !a.starts_with('-')))),
            ("s", value) => match value.or_else(|| args.next()) {
                Some(s) => source = s,
                None => return usage(),
            },
            ("p", value) => match value.or_else(|| args.next()).and_then(|p| p.parse().ok()) {
                Some(p) => remote_port = Some(p),
                None => return usage(),
            },
            ("play", value) => play = value.or_else(|| args.next()),
            _ => return usage(),
        }
    }

    if let Some(url) = play {
        return play_url(&url);
    }

    let stations = match stations::load(&source) {
        Ok(s) => s,
        Err(e) => {
            eprintln!("{e}");
            return ExitCode::FAILURE;
        }
    };
    if check {
        check::run(&stations);
        return ExitCode::SUCCESS;
    }
    if stations.is_empty() {
        println!("Stations list is empty, exiting.");
        return ExitCode::SUCCESS;
    }

    let player = match audio::Player::new() {
        Ok(p) => p,
        Err(e) => {
            eprintln!("{e}");
            return ExitCode::FAILURE;
        }
    };
    let bookmarks = bookmarks::Bookmarks::load(&stations);
    let config = config::Config::load();
    let opts = ui::Options { ascii, vu, remote, remote_port };
    let app = ui::App::new(player.clone(), stations, bookmarks, config, &opts);
    let result = app.run();
    player.stop();
    if let Err(e) = result {
        // Not eprintln, which panics when the terminal is gone.
        let _ = writeln!(std::io::stderr(), "{e}");
        return ExitCode::FAILURE;
    }
    ExitCode::SUCCESS
}

/// Plays one URL and prints what the player reports, until Ctrl+C. A stand-in
/// until the TUI is ported.
fn play_url(url: &str) -> ExitCode {
    let player = match audio::Player::new() {
        Ok(p) => p,
        Err(e) => {
            eprintln!("{e}");
            return ExitCode::FAILURE;
        }
    };
    let changes = player.subscribe();
    player.play(url, url);
    loop {
        let _ = changes.recv_timeout(std::time::Duration::from_millis(500));
        let inf = player.snapshot();
        let level = player.level().map_or(0.0, |(l, _)| l);
        let bars: String = player
            .spectrum()
            .map(|(b, _)| {
                b.iter().map(|&v| [' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'][(v * 8.0).round() as usize]).collect()
            })
            .unwrap_or_default();
        eprint!(
            "\r\x1b[K{} | {} | {} kbps | {} | level {:.2} {bars}",
            inf.status, inf.format, inf.bitrate, inf.song, level
        );
    }
}
