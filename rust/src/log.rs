//! The debug log, off unless enabled with -d.

use std::fs::{File, OpenOptions};
use std::io::Write;
use std::sync::{Mutex, OnceLock};
use std::time::{SystemTime, UNIX_EPOCH};

const FILE_NAME: &str = "goradion.log";

static FILE: OnceLock<Mutex<File>> = OnceLock::new();

/// Enables the log in goradion.log in the current directory.
pub fn init() -> std::io::Result<()> {
    let f = OpenOptions::new().append(true).create(true).open(FILE_NAME)?;
    let _ = FILE.set(Mutex::new(f));
    Ok(())
}

pub fn write(module: &str, message: std::fmt::Arguments<'_>) {
    if let Some(f) = FILE.get() {
        let line = format!("{} {} {module}: {message}\n", utc_now(), std::process::id());
        let _ = f.lock().unwrap().write_all(line.as_bytes());
    }
}

#[macro_export]
macro_rules! log {
    ($($arg:tt)*) => {
        $crate::log::write(module_path!(), format_args!($($arg)*))
    };
}

fn utc_now() -> String {
    let secs = SystemTime::now().duration_since(UNIX_EPOCH).map_or(0, |d| d.as_secs()) as i64;
    let (days, rem) = (secs.div_euclid(86400), secs.rem_euclid(86400));
    // Howard Hinnant's civil_from_days.
    let z = days + 719468;
    let era = z.div_euclid(146097);
    let doe = z - era * 146097;
    let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let day = doy - (153 * mp + 2) / 5 + 1;
    let month = if mp < 10 { mp + 3 } else { mp - 9 };
    let year = yoe + era * 400 + i64::from(month <= 2);
    format!("{year:04}-{month:02}-{day:02} {:02}:{:02}:{:02}Z", rem / 3600, rem / 60 % 60, rem % 60)
}
