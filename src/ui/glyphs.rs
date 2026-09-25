use std::time::SystemTime;

pub struct Glyphs {
    pub ascii: bool,
    pub play: &'static str,
    pub stop: &'static str,
    pub fail: &'static str,
    pub song: &'static str,
    pub notes: &'static str,
    pub back: &'static str,
    pub dot: &'static str,
    pub shuffle: &'static str,
    pub star: &'static str,
    pub check: &'static str,
    pub swatch: &'static str,
    pub sleep: &'static str,
    /// The cursor bar's left edge.
    pub edge: &'static str,
    /// The card's status light.
    pub live: &'static str,
    pub left: &'static str,
    pub right: &'static str,
    pub up: &'static str,
    pub down: &'static str,
    pub ellipsis: &'static str,
    pub spinner: &'static [&'static str],
    pub gauge_full: &'static str,
    pub gauge_half: &'static str,
    pub gauge_empty: &'static str,
    /// Spectrum bars, from quiet to full.
    pub bands: &'static [&'static str],
    /// Box corners and lines: top-left, top-right, bottom-left, bottom-right,
    /// horizontal, vertical.
    pub border: [&'static str; 6],
}

pub const UNICODE: Glyphs = Glyphs {
    ascii: false,
    play: "▶",
    stop: "■",
    fail: "!",
    song: "♪",
    notes: "♫",
    back: "‹",
    dot: "·",
    shuffle: "🔀",
    star: "★",
    check: "✓",
    swatch: "██",
    sleep: "☾",
    edge: "▎",
    live: "•",
    left: "←",
    right: "→",
    up: "↑",
    down: "↓",
    ellipsis: "…",
    spinner: &["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"],
    gauge_full: "━",
    gauge_half: "╸",
    gauge_empty: "─",
    bands: &["▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"],
    border: ["╭", "╮", "╰", "╯", "─", "│"],
};

pub const ASCII: Glyphs = Glyphs {
    ascii: true,
    play: ">",
    stop: "#",
    fail: "!",
    song: "~",
    notes: "*",
    back: "<",
    dot: "-",
    shuffle: "",
    star: "*",
    check: "*",
    swatch: "##",
    sleep: "z",
    edge: "",
    live: "o",
    left: "<-",
    right: "->",
    up: "^",
    down: "v",
    ellipsis: "~",
    spinner: &["|", "/", "-", "\\"],
    gauge_full: "=",
    gauge_half: "",
    gauge_empty: "-",
    bands: &[".", ":", "|", "#"],
    border: ["+", "+", "+", "+", "-", "|"],
};

impl Glyphs {
    pub fn spinner_frame(&self, t: SystemTime) -> &'static str {
        let ms = t.duration_since(SystemTime::UNIX_EPOCH).map_or(0, |d| d.as_millis());
        self.spinner[(ms / 100) as usize % self.spinner.len()]
    }
}

/// Follows tcell's charset rules, which the Go version used: where it fell
/// back to ASCII, Unicode symbols would print as question marks.
pub fn detect_ascii() -> bool {
    if cfg!(windows) {
        return false;
    }
    if std::env::var("TERM").is_ok_and(|t| t == "linux") {
        return true;
    }
    let locale = ["LC_ALL", "LC_CTYPE", "LANG"]
        .iter()
        .filter_map(|k| std::env::var(k).ok())
        .find(|v| !v.is_empty())
        .unwrap_or_default();
    !locale_is_utf8(&locale)
}

fn locale_is_utf8(locale: &str) -> bool {
    if locale == "C" || locale == "POSIX" {
        return false;
    }
    let locale = locale.split('@').next().unwrap_or("");
    match locale.split_once('.') {
        // No charset given, e.g. "en_US" or empty: UTF-8 is assumed.
        None => true,
        Some((_, charset)) => matches!(charset.to_ascii_lowercase().as_str(), "utf-8" | "utf8"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn locales() {
        assert!(locale_is_utf8("en_US.UTF-8"));
        assert!(locale_is_utf8("de_DE.utf8@euro"));
        assert!(locale_is_utf8(""));
        assert!(!locale_is_utf8("C"));
        assert!(!locale_is_utf8("en_US.ISO-8859-1"));
    }
}
