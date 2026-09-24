mod audio;
mod bookmarks;
mod check;
mod config;
mod files;
mod radiobrowser;
mod log;
mod stations;
mod ui;

use std::process::ExitCode;

const USAGE: &str = "usage: goradion [-s stations.csv|URL] [-c] [-d] [-v] [--ascii] [--no-vu] [--play URL]";

fn main() -> ExitCode {
    let mut args = std::env::args().skip(1);
    let (mut source, mut check, mut play) = (String::new(), false, None);
    let (mut ascii, mut vu) = (ui::glyphs::detect_ascii(), true);
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "-v" => {
                println!("goradion {}", env!("CARGO_PKG_VERSION"));
                return ExitCode::SUCCESS;
            }
            "-d" => {
                if let Err(e) = log::init() {
                    eprintln!("{e}");
                    return ExitCode::FAILURE;
                }
            }
            "-c" => check = true,
            "-s" => source = args.next().unwrap_or_default(),
            "--play" => play = args.next(),
            "-ascii" | "--ascii" => ascii = true,
            "-no-vu" | "--no-vu" => vu = false,
            _ => {
                eprintln!("{USAGE}");
                return ExitCode::FAILURE;
            }
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
    let opts = ui::Options { ascii, vu };
    let app = ui::App::new(player.clone(), stations, bookmarks, config, &opts);
    let result = app.run();
    player.stop();
    if let Err(e) = result {
        eprintln!("{e}");
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
            .map(|(b, _)| b.iter().map(|&v| [' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'][(v * 8.0).round() as usize]).collect())
            .unwrap_or_default();
        eprint!(
            "\r\x1b[K{} | {} | {} kbps | {} | level {:.2} {bars}",
            inf.status, inf.format, inf.bitrate, inf.song, level
        );
    }
}
