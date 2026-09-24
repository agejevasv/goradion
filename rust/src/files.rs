use std::io::Write;
use std::path::{Path, PathBuf};

pub fn config_dir() -> PathBuf {
    let home = std::env::var_os(if cfg!(windows) { "USERPROFILE" } else { "HOME" })
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from("."));
    if cfg!(windows) {
        home.join("AppData").join("Roaming").join("goradion")
    } else if cfg!(target_os = "macos") {
        home.join("Library").join("Application Support").join("goradion")
    } else {
        home.join(".config").join("goradion")
    }
}

/// Replaces the file through a rename, so that a crash never leaves it half
/// written.
pub fn write_atomic(path: &Path, data: &[u8]) -> std::io::Result<()> {
    let dir = path.parent().unwrap_or(Path::new("."));
    std::fs::create_dir_all(dir)?;
    let name = path.file_name().and_then(|n| n.to_str()).unwrap_or("file");
    let tmp = dir.join(format!(".{name}.{}", std::process::id()));
    let result = (|| {
        let mut f = std::fs::File::create(&tmp)?;
        f.write_all(data)?;
        f.sync_all()?;
        std::fs::rename(&tmp, path)
    })();
    if result.is_err() {
        let _ = std::fs::remove_file(&tmp);
    }
    result
}
