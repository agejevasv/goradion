//! Closing the console window on Windows. Windows ends the process as soon as
//! the handler returns, so it waits until the session is saved.

use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Condvar, Mutex, OnceLock, PoisonError};

use windows_sys::Win32::Foundation::{FALSE, TRUE};
use windows_sys::Win32::System::Console::{
    CTRL_CLOSE_EVENT, CTRL_LOGOFF_EVENT, CTRL_SHUTDOWN_EVENT, SetConsoleCtrlHandler,
};
use windows_sys::core::BOOL;

use crate::log;

struct Exit {
    quit: Arc<AtomicBool>,
    saved: Mutex<bool>,
    saved_changed: Condvar,
}

static EXIT: OnceLock<Exit> = OnceLock::new();

/// Sets quit when the window is closed, and on Ctrl+Break; a close then
/// waits for `saved`.
#[allow(unsafe_code)]
pub fn on_close(quit: Arc<AtomicBool>) {
    if EXIT.set(Exit { quit, saved: Mutex::new(false), saved_changed: Condvar::new() }).is_err() {
        return;
    }
    // SAFETY: handler is a plain function, valid for the life of the process.
    if unsafe { SetConsoleCtrlHandler(Some(handler), TRUE) } == FALSE {
        log!("console: {}", std::io::Error::last_os_error());
    }
}

/// Lets a close waiting in the handler go on, which ends the process.
pub fn saved() {
    if let Some(exit) = EXIT.get() {
        *exit.saved.lock().unwrap_or_else(PoisonError::into_inner) = true;
        exit.saved_changed.notify_all();
    }
}

/// Runs on a thread Windows starts for the event. In raw mode Ctrl+C is a key,
/// not an event.
extern "system" fn handler(event: u32) -> BOOL {
    let Some(exit) = EXIT.get() else { return FALSE };
    exit.quit.store(true, Ordering::Relaxed);
    if matches!(event, CTRL_CLOSE_EVENT | CTRL_LOGOFF_EVENT | CTRL_SHUTDOWN_EVENT) {
        // Windows stops waiting after 5 seconds anyway.
        let saved = exit.saved.lock().unwrap_or_else(PoisonError::into_inner);
        drop(exit.saved_changed.wait_while(saved, |saved| !*saved));
    }
    TRUE
}
