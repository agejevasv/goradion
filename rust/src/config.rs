//! config.yaml. Saving rewrites only the named values in the file as it is
//! now, which the user may have edited meanwhile, keeping comments and layout.
//!
//! The parser takes the YAML the file needs: `key: value` lines, one level of
//! nested mappings, comments and quoted strings. Anything else under a key
//! goradion doesn't read is skipped; flow collections must close on their line.

use std::path::PathBuf;

use crate::audio::player::DEFAULT_VOLUME;
use crate::files;
use crate::log;
use crate::ui::theme;

pub const DEFAULT_REMOTE_PORT: u16 = 7373;

#[derive(Clone, Debug, PartialEq)]
pub struct Config {
    pub theme: String,
    pub volume: i32,
    pub tag: String,
    /// URL.
    pub station: String,
    pub autoplay: bool,
    pub remote: Remote,
    path: PathBuf,
}

#[derive(Clone, Debug, PartialEq)]
pub struct Remote {
    pub autostart: bool,
    pub port: u16,
    /// None: a random code per run.
    pub key: Option<String>,
}

pub fn file() -> PathBuf {
    files::config_dir().join("config.yaml")
}

fn default_text() -> String {
    let names: Vec<&str> = theme::names().collect();
    format!(
        r#"# goradion settings. goradion updates this file itself, e.g. when you pick a
# theme with Ctrl+T, and keeps your comments.

# Colour theme, one of:
{}# "{def}" uses the terminal's own colours and background.
theme: {def}

# Play the last station at launch.
autoplay: true

# Remembered from the last run.
volume: {DEFAULT_VOLUME}
tag: ""
station: ""

# Phone remote, Ctrl+P. The key is a fixed access code instead of a random one
# per run; "" for none.
# remote:
#   autostart: true
#   port: {DEFAULT_REMOTE_PORT}
#   key: abc123
"#,
        comment_lines(&names.join(", "), 76),
        def = theme::DEFAULT_THEME,
    )
}

/// Wraps text at spaces into indented YAML comment lines.
fn comment_lines(text: &str, width: usize) -> String {
    let mut out = String::new();
    let mut line = String::new();
    for word in text.split_whitespace() {
        if !line.is_empty() && line.len() + 1 + word.len() > width {
            out.push_str(&format!("#   {line}\n"));
            line.clear();
        }
        if !line.is_empty() {
            line.push(' ');
        }
        line.push_str(word);
    }
    out.push_str(&format!("#   {line}\n"));
    out
}

impl Config {
    fn defaults(path: PathBuf) -> Self {
        Config {
            theme: theme::DEFAULT_THEME.to_string(),
            volume: DEFAULT_VOLUME,
            tag: String::new(),
            station: String::new(),
            autoplay: true,
            remote: Remote { autostart: false, port: DEFAULT_REMOTE_PORT, key: None },
            path,
        }
    }

    pub fn load() -> Self {
        Self::load_from(file())
    }

    /// Creates the file when it is missing. A file that can't be read or
    /// parsed is logged and the defaults are used.
    pub fn load_from(path: PathBuf) -> Self {
        let text = match std::fs::read_to_string(&path) {
            Ok(text) => Ok(text),
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
                let text = default_text();
                files::write_atomic(&path, text.as_bytes()).map(|_| text)
            }
            Err(e) => Err(e),
        };
        let mut c = Config::defaults(path);
        let result = text.map_err(|e| e.to_string()).and_then(|text| c.parse(&text));
        if let Err(e) = result {
            log!("config: {e}");
            c = Config::defaults(c.path);
        }
        c
    }

    fn parse(&mut self, text: &str) -> Result<(), String> {
        for entry in parse_yaml(text)? {
            let value = entry.value.as_deref();
            match (entry.parent.as_deref(), entry.key.as_str()) {
                (None, "theme") => self.theme = value.unwrap_or_default().to_string(),
                (None, "volume") => self.volume = number(&entry)?,
                (None, "tag") => self.tag = value.unwrap_or_default().to_string(),
                (None, "station") => self.station = value.unwrap_or_default().to_string(),
                (None, "autoplay") => self.autoplay = boolean(&entry)?,
                (Some("remote"), "autostart") => self.remote.autostart = boolean(&entry)?,
                (Some("remote"), "port") => self.remote.port = number(&entry)?,
                (Some("remote"), "key") => self.remote.key = value.map(String::from),
                _ => {}
            }
        }
        Ok(())
    }

    /// Rewrites the named top-level keys (theme, volume, tag, station) in the
    /// file as it is now; missing keys are appended.
    pub fn save(&self, names: &[&str]) -> Result<(), String> {
        let path = self.path.display();
        let text = match std::fs::read_to_string(&self.path) {
            Ok(text) => text,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => default_text(),
            Err(e) => return Err(e.to_string()),
        };
        let entries = parse_yaml(&text).map_err(|e| format!("{path}: {e}, not overwriting it"))?;
        let mut lines: Vec<String> = text.split_inclusive('\n').map(String::from).collect();
        let mut appended = String::new();
        for &name in names {
            let value = match name {
                "theme" => quote(&self.theme),
                "volume" => self.volume.to_string(),
                "tag" => quote(&self.tag),
                "station" => quote(&self.station),
                _ => continue,
            };
            match entries.iter().find(|e| e.parent.is_none() && e.key == name) {
                None => appended.push_str(&format!("{name}: {value}\n")),
                Some(e) if e.multiline => return Err(format!("{path}: {name} is not a one-line value")),
                Some(e) => lines[e.line] = replace_value(&lines[e.line], e.colon, &value),
            }
        }
        let mut out = lines.concat();
        if !appended.is_empty() {
            if !out.is_empty() && !out.ends_with('\n') {
                out.push('\n');
            }
            out.push_str(&appended);
        }
        files::write_atomic(&self.path, out.as_bytes()).map_err(|e| e.to_string())
    }
}

fn number<T: std::str::FromStr>(e: &Entry) -> Result<T, String> {
    e.value.as_deref().unwrap_or("").parse().map_err(|_| format!("line {}: {} is not a number", e.line + 1, e.key))
}

fn boolean(e: &Entry) -> Result<bool, String> {
    match e.value.as_deref() {
        Some("true" | "True" | "TRUE") => Ok(true),
        Some("false" | "False" | "FALSE") => Ok(false),
        _ => Err(format!("line {}: {} is not true or false", e.line + 1, e.key)),
    }
}

#[derive(Debug)]
struct Entry {
    parent: Option<String>,
    key: String,
    /// None for an empty or null value.
    value: Option<String>,
    line: usize,
    /// Byte offset of the colon after the key.
    colon: usize,
    /// The value continues on the following lines.
    multiline: bool,
}

fn parse_yaml(text: &str) -> Result<Vec<Entry>, String> {
    let mut entries = Vec::new();
    // The top-level entry whose value is the block being read, and the
    // block's indent.
    let mut block: Option<(usize, Option<usize>)> = None;
    for (n, raw) in text.lines().enumerate() {
        let err = |what: &str| format!("line {}: {what}", n + 1);
        let content = strip_comment(raw);
        if content.trim().is_empty() || raw.trim_start().starts_with("---") {
            continue;
        }
        if content.starts_with('\t') {
            return Err(err("tab indentation"));
        }
        let indent = content.len() - content.trim_start().len();
        let line = content.trim();

        let parent = if indent == 0 {
            block = None;
            None
        } else {
            let Some((parent, child_indent)) = &mut block else {
                return Err(err("unexpected indentation"));
            };
            let parent_entry: &mut Entry = &mut entries[*parent];
            parent_entry.multiline = true;
            let child_indent = *child_indent.get_or_insert(indent);
            if indent != child_indent || line.starts_with("- ") || line == "-" {
                // Deeper structure under a key goradion doesn't read.
                continue;
            }
            Some(parent_entry.key.clone())
        };

        let colon = find_colon(line).ok_or_else(|| err("expected key: value"))?;
        let key = unquote(line[..colon].trim()).map_err(|e| err(&e))?;
        let rest = line[colon + 1..].trim();
        let value = if rest.is_empty() || rest == "~" || rest == "null" {
            None
        } else if matches!(rest.chars().next(), Some('|' | '>')) {
            None
        } else if rest.starts_with('[') || rest.starts_with('{') {
            if !balanced(rest) {
                return Err(err("unclosed flow collection"));
            }
            Some(rest.to_string())
        } else {
            Some(unquote(rest).map_err(|e| err(&e))?)
        };
        let multiline = matches!(rest.chars().next(), Some('|' | '>'));
        if indent == 0 && (rest.is_empty() || multiline) {
            block = Some((entries.len(), None));
        }
        entries.push(Entry { parent, key, value, line: n, colon: indent + colon, multiline });
    }
    Ok(entries)
}

/// The line without a comment: `#` at the start or after whitespace, outside
/// quotes.
fn strip_comment(line: &str) -> &str {
    let (mut single, mut double, mut prev_space) = (false, false, true);
    for (i, c) in line.char_indices() {
        match c {
            '\'' if !double => single = !single,
            '"' if !single => double = !double,
            '#' if !single && !double && prev_space => return &line[..i],
            _ => {}
        }
        prev_space = c.is_whitespace();
    }
    line.trim_end_matches(['\r', '\n'])
}

/// The colon that ends a key: followed by a space or the line end, outside
/// quotes.
fn find_colon(line: &str) -> Option<usize> {
    let (mut single, mut double) = (false, false);
    let bytes = line.as_bytes();
    for (i, c) in line.char_indices() {
        match c {
            '\'' if !double => single = !single,
            '"' if !single => double = !double,
            ':' if !single && !double && (i + 1 == bytes.len() || bytes[i + 1] == b' ') => return Some(i),
            _ => {}
        }
    }
    None
}

fn balanced(s: &str) -> bool {
    let mut depth = 0i32;
    for c in s.chars() {
        match c {
            '[' | '{' => depth += 1,
            ']' | '}' => depth -= 1,
            _ => {}
        }
    }
    depth == 0
}

fn unquote(s: &str) -> Result<String, String> {
    if let Some(inner) = s.strip_prefix('\'') {
        let inner = inner.strip_suffix('\'').ok_or("unclosed quote")?;
        return Ok(inner.replace("''", "'"));
    }
    if let Some(inner) = s.strip_prefix('"') {
        let inner = inner.strip_suffix('"').ok_or("unclosed quote")?;
        let mut out = String::new();
        let mut chars = inner.chars();
        while let Some(c) = chars.next() {
            if c != '\\' {
                out.push(c);
                continue;
            }
            match chars.next() {
                Some('n') => out.push('\n'),
                Some('t') => out.push('\t'),
                Some('"') => out.push('"'),
                Some('\\') => out.push('\\'),
                Some('/') => out.push('/'),
                Some(other) => return Err(format!("unknown escape \\{other}")),
                None => return Err("unclosed quote".into()),
            }
        }
        return Ok(out);
    }
    Ok(s.to_string())
}

/// A plain scalar when YAML reads it back as the same string, else double
/// quoted.
fn quote(s: &str) -> String {
    let special_start = s.starts_with(|c: char| "-?:,[]{}#&*!|>'\"%@`".contains(c) || c.is_whitespace());
    let ambiguous = matches!(
        s.to_ascii_lowercase().as_str(),
        "" | "~" | "null" | "true" | "false" | "yes" | "no" | "on" | "off"
    ) || s.parse::<f64>().is_ok();
    let plain = !special_start
        && !ambiguous
        && !s.ends_with(char::is_whitespace)
        && !s.contains(": ")
        && !s.ends_with(':')
        && !s.contains(" #")
        && !s.chars().any(char::is_control);
    if plain {
        return s.to_string();
    }
    let escaped: String = s
        .chars()
        .flat_map(|c| match c {
            '"' => vec!['\\', '"'],
            '\\' => vec!['\\', '\\'],
            '\n' => vec!['\\', 'n'],
            '\t' => vec!['\\', 't'],
            c => vec![c],
        })
        .collect();
    format!("\"{escaped}\"")
}

/// Puts text after the colon at `colon`, keeping a trailing comment and the
/// line ending.
fn replace_value(line: &str, colon: usize, text: &str) -> String {
    let body = line.trim_end_matches(['\r', '\n']);
    let ending = &line[body.len()..];
    let stripped = strip_comment(body);
    let tail = if stripped.len() < body.len() {
        // The spaces before the comment belong to it.
        let value_end = stripped.trim_end().len().max(colon + 1);
        &body[value_end..]
    } else {
        ""
    };
    format!("{} {text}{tail}{ending}", &body[..=colon])
}

#[cfg(test)]
mod tests {
    use super::*;

    fn temp(name: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!("goradion-cfg-{}-{name}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        dir.join("config.yaml")
    }

    fn write(path: &PathBuf, text: &str) {
        std::fs::create_dir_all(path.parent().unwrap()).unwrap();
        std::fs::write(path, text).unwrap();
    }

    fn read(path: &PathBuf) -> String {
        std::fs::read_to_string(path).unwrap()
    }

    #[test]
    fn defaults_and_save() {
        let path = temp("defaults");
        let c = Config::load_from(path.clone());
        assert_eq!(c.theme, theme::DEFAULT_THEME);
        assert!(read(&path).contains(&format!("theme: {}", theme::DEFAULT_THEME)));

        // Saving keeps comments and keys goradion does not know.
        write(&path, "# mine\ntheme: nord # dark\nfuture: 1\n");
        let mut c = Config::load_from(path.clone());
        assert_eq!(c.theme, "nord");
        c.theme = "dracula".into();
        c.volume = 35;
        c.save(&["theme"]).unwrap();
        assert_eq!(read(&path), "# mine\ntheme: dracula # dark\nfuture: 1\n");

        // A broken file is left alone.
        write(&path, "theme: [\n");
        let c = Config::load_from(path.clone());
        assert_eq!(c.theme, theme::DEFAULT_THEME);
        assert!(c.save(&["theme"]).is_err());
    }

    #[test]
    fn remote() {
        let path = temp("remote");
        let c = Config::load_from(path.clone());
        assert!(c.autoplay && !c.remote.autostart && c.remote.port == DEFAULT_REMOTE_PORT && c.remote.key.is_none());

        for (text, want) in [("remote:\n  key: 123456\n", "123456"), ("remote:\n  key: \"\"\n", "")] {
            write(&path, text);
            assert_eq!(Config::load_from(path.clone()).remote.key.as_deref(), Some(want), "{text:?}");
        }

        // Saving leaves the hand-edited keys alone.
        let text = "autoplay: false # quiet\nremote:\n  # mine\n  autostart: true\n  port: 8123\n";
        write(&path, text);
        let mut c = Config::load_from(path.clone());
        assert!(!c.autoplay && c.remote.autostart && c.remote.port == 8123, "{c:?}");
        c.theme = "nord".into();
        c.save(&["theme"]).unwrap();
        assert!(read(&path).starts_with(text), "{}", read(&path));
    }

    #[test]
    fn save_keeps_layout() {
        let path = temp("layout");
        write(&path, "# top\n\ntheme: nord   # dark\n\nvolume: 10\r\ntag:\nstation: 'x' # s\n\nremote:\n  port: 1\n");
        let mut c = Config::load_from(path.clone());
        c.theme = "dracula".into();
        c.volume = 35;
        c.tag = "Jazz".into();
        c.station = "https://a/b?c=d#e".into();
        c.save(&["theme", "volume", "tag", "station"]).unwrap();
        assert_eq!(
            read(&path),
            "# top\n\ntheme: dracula   # dark\n\nvolume: 35\r\ntag: Jazz\nstation: https://a/b?c=d#e # s\n\nremote:\n  port: 1\n"
        );
        assert_eq!(Config::load_from(path.clone()), c);
    }

    #[test]
    fn save_keeps_edits_made_meanwhile() {
        let path = temp("meanwhile");
        let mut c = Config::load_from(path.clone());
        let edited = "theme: nord\nremote:\n  autostart: true\n  key: mine\n";
        write(&path, edited);
        c.volume = 35;
        c.save(&["volume"]).unwrap();
        assert_eq!(read(&path), format!("{edited}volume: 35\n"));

        // Broken mid-edit: left alone. Fixed later: saved.
        write(&path, "theme: [\n");
        assert!(c.save(&["volume"]).is_err());
        write(&path, "theme: nord\n");
        c.save(&["volume"]).unwrap();
    }

    #[test]
    fn quoting_round_trips() {
        for s in ["", "true", "123", "a: b", " x", "#tag", "it's \"q\"", "Jazz", "https://a/b?c=d#e"] {
            let text = format!("tag: {}\n", quote(s));
            let entries = parse_yaml(&text).unwrap();
            assert_eq!(entries[0].value.as_deref().unwrap_or(""), s, "{text:?}");
        }
    }

    #[test]
    fn default_text_parses() {
        let mut c = Config::defaults(PathBuf::new());
        c.parse(&default_text()).unwrap();
        assert_eq!(c, Config::defaults(PathBuf::new()));
    }
}
