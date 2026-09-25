use ratatui::buffer::Buffer;
use ratatui::layout::Rect;
use ratatui::style::Modifier;

use super::Look;
use super::text::{Seg, draw_segs, seg, width};

fn lines(look: &Look) -> Vec<Vec<Seg>> {
    let (t, g) = (&look.t, look.g);
    let arrows = format!("{} {} - +", g.left, g.right);
    let moves = format!("{} {} PgUp PgDn", g.up, g.down);
    let sections: [(&str, Vec<(&str, &str)>); 3] = [
        (
            "Playing",
            vec![
                ("Enter Space", "play or stop the station under the cursor"),
                ("*", "play a random station from the list"),
                ("1 to 9", "play one of the first nine bookmarks"),
                (arrows.as_str(), "volume down or up"),
                ("Ctrl+B", "bookmark the station under the cursor, or the one playing; again to remove"),
                ("Ctrl+R", "shuffle: a random station every few minutes"),
                ("Ctrl+Z", "sleep timer: 15, 30, 45, 60, 90 minutes, off; fades out in the last minute"),
                ("Alt+1 to 9", "shuffle interval in minutes"),
            ],
        ),
        (
            "Finding stations",
            vec![
                ("a-z", "filter the list; while filtering every key types"),
                (":", "search your stations"),
                ("Ctrl+F", "search your stations; press again to search online"),
                ("Ctrl+S", "search online; press again to search your stations"),
                ("Backspace", "edit the filter"),
                ("Esc", "clear the filter, go back, or quit from the tags list"),
                ("/ #", "tags"),
                (moves.as_str(), "move through the list"),
                ("Tab", "switch between tags and stations (wide terminals)"),
            ],
        ),
        (
            "More",
            vec![
                ("?", "this help"),
                ("Ctrl+P", "control goradion from your phone"),
                ("Ctrl+T", "colour theme"),
                ("Mouse", "click a station to play it, scroll lists; scroll or click the volume gauge"),
                ("", "click the sleep countdown to turn the timer off"),
                ("Ctrl+C", "quit"),
            ],
        ),
    ];
    let mut out = vec![vec![seg(crate::version_string(), t.accent().add_modifier(Modifier::BOLD))]];
    for (title, keys) in sections {
        out.push(Vec::new());
        out.push(vec![seg(title, t.text().add_modifier(Modifier::BOLD))]);
        for (key, what) in keys {
            let pad = 16usize.saturating_sub(width(key));
            out.push(vec![seg(format!("  {key}{}", " ".repeat(pad)), t.accent()), seg(what, t.text())]);
        }
    }
    out.push(Vec::new());
    out.push(vec![seg("Settings", t.text().add_modifier(Modifier::BOLD))]);
    out.push(vec![seg(format!("  {}", crate::files::config_dir().display()), t.text())]);
    out
}

pub fn draw(look: &Look, buf: &mut Buffer, area: Rect, scroll: &mut usize) {
    super::draw_box(look, buf, area, look.t.dim(), &[seg(" Help ", look.t.text())]);
    if area.height < 3 || area.width < 5 {
        return;
    }
    let lines = lines(look);
    let h = area.height as usize - 2;
    *scroll = (*scroll).min(lines.len().saturating_sub(h));
    for (i, line) in lines.iter().skip(*scroll).take(h).enumerate() {
        draw_segs(buf, area.x + 2, area.y + 1 + i as u16, area.width as usize - 4, line);
    }
}
