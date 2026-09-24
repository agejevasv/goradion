# Theme sources

Every theme's colours are copied from its own published palette. Where a source
gives a colour with transparency, it is flattened onto the theme's background.
`TestThemeLegibility` in `internal/radio/themes_test.go` checks each theme
against the rules below.

## Rules

| Role    | Taken from | Must reach |
|---------|------------|------------|
| bg      | the editor background | |
| text    | the body text (the UI foreground for Material) | 4.5:1 on bg |
| dim     | the comment colour; if that is fainter than 2.3:1, the secondary UI text | 2.3:1 on bg |
| surface | the selection or selected-item background; if text on it drops under 4:1, the next UI background that keeps it readable | 1.1:1 on bg, text 4:1 on it |
| accent  | the theme's signature hue; if it is too faint, the readable palette colour nearest in hue | 4.5:1 on dark bg, 3:1 on light |
| bright  | a second hue; the purple when that one is too close to the accent | ΔE 15 from the accent |
| warn    | the yellow; the orange when the accent is itself yellow | ΔE 15 from the accent |
| danger  | the error colour, or the red | ΔE 15 from the accent |
| live    | the green, lighting the Now playing dot, also where the accent is green | ΔE 15 from dim |

ΔE is the CIE76 colour distance; under 15, two colours pass for one at a glance.

## Where a theme departs from its palette

| Theme | Change | Why |
|-------|--------|-----|
| material | accent `#80CBC4` (Links), not `#009688` | the official accent is 3.6:1 |
| palenight | accent `#C792EA` (Purple), not `#AB47BC` | 2.8:1 |
| material-lighter, -sky-blue, -sandy-beach | accent `#6182B8` (Blue), not the cyan accent | the cyan accents are 1.8–2.2:1 |
| material-lighter | dim `#94A7B0` (Text) | its comments are 1.8:1 |
| material-forest | dim `#49694D` (Text); warn `#F78C6C` (Orange) | its comments are 1.8:1; its accent `#FFCC80` is its yellow |
| material-deep-ocean | surface `#1F2233` (Highlight) | its Active is 1.1:1, invisible |
| material-volcano | surface `#550000` (Highlight); bright `#C792EA` | its Active is 1.06:1; its cyan is close to its accent |
| material-sky-blue | surface `#ADE2EB` (Selection) | its Active is 1.14:1 |
| material-sandy-beach | surface `#F6D7B0` (Buttons) | its Active matches the background; no visible surface keeps text at 4:1, this one is closest (3.9:1) |
| rose-pine-dawn | accent iris `#907AA9`, not rose | rose is 2.6:1; love is nearer in hue but is the danger colour |
| everforest-light | accent blue `#3A94C5`, not green | its green is 2.7:1, and blue is its only colour over 3:1 |
| ayu-light | accent `#A37ACC` (constant); dim `#828E9F` (UI foreground) | its accent is 2.2:1 and the constant is its only colour over 3:1; its comments are 2.1:1 |
| ayu-dark, ayu-mirage | warn is the keyword orange | the accent is ayu's yellow |
| nord | bright nord15 `#B48EAD` | nord7 is too close to nord8, the accent |
| kanagawa-dragon | bright dragonPink `#A292A3` | its special colour is too close to its accent |
| kanagawa, -dragon, -lotus | surface bg_p2, which kanagawa documents as the selected-item background | lotus's menu selection leaves text at 3.5:1 |
| tokyo-night-day | surface bg_highlight (CursorLine) `#C4C8DA` | its selection leaves text at 3.3:1; CursorLine is the most readable it has (3.5:1) |
| solarized-light | text stays base00 (4.1:1) | the Solarized spec sets body text to base00 |

## Sources

| Themes | Source |
|--------|--------|
| material, palenight, material-* | [Material Theme colour palette](https://material-theme.com/docs/reference/color-palette/) |
| catppuccin, -macchiato, -frappe, -latte | [catppuccin/palette](https://github.com/catppuccin/palette) `palette.json` |
| tokyo-night, -storm, -moon, -day | [folke/tokyonight.nvim](https://github.com/folke/tokyonight.nvim) `extras/` |
| rose-pine, -moon, -dawn | [rose-pine/neovim](https://github.com/rose-pine/neovim) `palette.lua` |
| gruvbox, gruvbox-light | [morhetz/gruvbox](https://github.com/morhetz/gruvbox) `colors/gruvbox.vim` |
| everforest, everforest-light | [sainnhe/everforest](https://github.com/sainnhe/everforest) (medium contrast) |
| kanagawa, -dragon, -lotus | [rebelot/kanagawa.nvim](https://github.com/rebelot/kanagawa.nvim) `colors.lua`, `themes.lua` |
| nightfox, carbonfox | [EdenEast/nightfox.nvim](https://github.com/EdenEast/nightfox.nvim) `palette/`, `extra/` |
| ayu-dark, -mirage, -light | [ayu](https://www.npmjs.com/package/ayu) npm package 9.0.0 |
| github-dark, github-light | [@primer/primitives](https://www.npmjs.com/package/@primer/primitives) functional tokens |
| monokai | [textmate/monokai.tmbundle](https://github.com/textmate/monokai.tmbundle) |
| dracula | [dracula/dracula-theme](https://github.com/dracula/dracula-theme) spec |
| nord | [nordtheme/nord](https://github.com/nordtheme/nord); dim is the brightened comment tone `#616E88` its ports use |
| one-dark | Atom's [one-dark-syntax](https://github.com/atom/atom/tree/master/packages/one-dark-syntax) `colors.less`, `syntax-variables.less` |
| night-owl | [sdras/night-owl-vscode-theme](https://github.com/sdras/night-owl-vscode-theme) |
| solarized-dark, -light | [altercation/solarized](https://github.com/altercation/solarized) |
| tomorrow-night (Rust version) | [chriskempson/tomorrow-theme](https://github.com/chriskempson/tomorrow-theme) |
