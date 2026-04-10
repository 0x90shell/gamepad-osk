# gamepad-osk

Gamepad-controlled on-screen keyboard for Linux. Designed for couch gaming, Sunshine/Moonlight streaming, and any setup where you need to type with a controller.

No Steam dependency. Works on X11 and Wayland (key injection via uinput).

![gamepad-osk](assets/matrix-hero.png)

## Features

- Full QWERTY keyboard with shortcuts row (Undo, Redo, Cut, Select All, Alt+Tab, etc.)
- Native Wayland overlay via wlr-layer-shell (Sway, Hyprland, KDE, COSMIC - no compositor rules needed)
- Evdev gamepad input (works with any controller)
- Xbox 360 pad auto-detection (swap_xy quirk handled automatically)
- 60 color themes (cycle live with Cfg key, or set via config/flag)
- Promptfont controller-agnostic button glyphs on mapped keys
- Mouse cursor via right stick + R3/RB click (hold to drag)
- Key repeat on hold (configurable delay and rate)
- Shift layer, Caps Lock, accent popup (Shift + hold on vowels)
- Paste/Copy key, media keys (Play/Pause, Mute)
- Always-on-top, no focus stealing (types into whatever app has focus)
- Multi-monitor aware (positions on primary monitor)
- Configurable: buttons, sticks, theme, scale, position, opacity, deadzone
- IPC toggle via Unix socket (`gamepad-osk --toggle`) for evsieve/hotkey integration
- Built-in configurable toggle combo (e.g. `guide+a`, `l3+r3`) for zero-dependency show/hide
- Daemon mode for systemd user service
- Single static binary, ~5MB

## Dependencies

**Runtime:** `sdl3` `sdl3_ttf` `ttf-promptfont` (AUR)

**Build:** `go` `sdl3` `sdl3_ttf` `libx11` `wayland` `wlr-protocols`

## Installation

### AUR (Arch Linux)

```bash
yay -S gamepad-osk-bin   # pre-built binary from GitHub release
yay -S gamepad-osk-git   # build from latest source
```

To auto-update `-git` packages when upstream changes, enable devel checking:

```bash
yay --devel --save
```

### From source

Install build dependencies (Arch):

```bash
sudo pacman -S go sdl3 sdl3_ttf libx11 wayland wlr-protocols
yay -S ttf-promptfont
```

```bash
git clone https://github.com/0x90shell/gamepad-osk.git
cd gamepad-osk
go build -o gamepad-osk .
sudo install -Dm755 gamepad-osk /usr/bin/gamepad-osk
sudo install -Dm644 config.example /usr/share/gamepad-osk/config
sudo install -Dm644 gamepad-osk.service /usr/lib/systemd/user/gamepad-osk.service
```

## Permissions

gamepad-osk reads directly from `/dev/input` for gamepad input and `/dev/uinput` for key injection. Your user must be in the `input` group:

```bash
sudo usermod -aG input $USER
```

Log out and back in for the group change to take effect. Verify with `groups`.

AUR packages install a udev rule that sets the correct permissions on input devices. If installing from source, copy the rule manually:

```bash
sudo install -Dm644 gamepad-osk.udev /usr/lib/udev/rules.d/80-gamepad-osk.rules
sudo udevadm control --reload-rules
```

If you must use sudo, pass your config explicitly to avoid loading root's config:

```bash
sudo gamepad-osk --config ~/.config/gamepad-osk/config
```

## Systemd User Service

AUR packages install the service file automatically. To enable:

```bash
systemctl --user enable --now gamepad-osk
```

Toggle visibility (bind to evsieve or hotkey):

```bash
gamepad-osk --toggle
```

## Usage

```
gamepad-osk                          # start (auto-detect gamepad)
gamepad-osk --device /dev/input/X    # use specific device
gamepad-osk --theme synthwave        # start with theme
gamepad-osk --config /path/to/config  # use specific config file
gamepad-osk --toggle                 # toggle running instance
gamepad-osk --daemon                 # start hidden, wait for toggle
gamepad-osk --help                   # show all options
```

## Controls

| Input | Action |
|-------|--------|
| Left stick / D-pad | Navigate keyboard |
| Right stick | Move mouse cursor |
| A | Press highlighted key (hold to repeat) |
| B | Close keyboard |
| X | Backspace (hold to repeat) |
| Y | Space (hold to repeat) |
| LT (hold) | Shift |
| RT | Enter (hold to repeat) |
| RB | Left mouse click (hold to drag) |
| LB | Right mouse click |
| Mouse stick click (R3) | Left mouse click |
| Nav stick click (L3) | Caps Lock |
| Start | Toggle keyboard top/bottom |
| Shift (LT) + hold A (on vowel) | Accent popup (é, ñ, ü, etc.) |
| Cfg key | Cycle themes (Shift+Cfg = reverse) |
| Toggle combo (configurable) | Show/hide keyboard (daemon mode) |

## Configuration

Config is loaded from (first found):
1. `--config` flag
2. `~/.config/gamepad-osk/config`
3. `/etc/gamepad-osk/config`
4. `config` next to binary
5. `config` in working directory

A default config is auto-copied to `~/.config/gamepad-osk/` on first run.

See `config.example` for all options including button remapping, toggle combo, mouse stick, theme, scale, opacity, and deadzone.

## Themes

60 built-in themes. Cycle live with the Cfg key on the keyboard.

`ayu_dark` `candy` `catppuccin` `catppuccin_frappe` `cga` `chalk` `cobalt` `copper` `coral` `cyberpunk` `dark` `dracula` `ember` `everforest` `fjord` `forest` `gameboy` `gold` `gotham` `gruvbox` `high_contrast` `horizon` `ice` `kanagawa` `lavender` `material` `matrix` `mellow` `midnight` `monokai` `moss` `navy` `neon` `nightfox` `nord` `ocean` `olive` `onedark` `oxocarbon` `palenight` `paper` `plum` `retro` `rose_pine` `sakura` `sand` `slate` `solarized` `solarized_light` `steam_green` `sunset` `synthwave` `teal` `terminal` `tokyo_night` `tokyo_storm` `vapor` `virtualboy` `wine` `zx_spectrum`

| | | |
|:---:|:---:|:---:|
| ![ayu_dark](assets/ayu_dark.png)<br><sub>ayu_dark</sub> | ![candy](assets/candy.png)<br><sub>candy</sub> | ![catppuccin](assets/catppuccin.png)<br><sub>catppuccin</sub> |
| ![catppuccin_frappe](assets/catppuccin_frappe.png)<br><sub>catppuccin_frappe</sub> | ![cga](assets/cga.png)<br><sub>cga</sub> | ![chalk](assets/chalk.png)<br><sub>chalk</sub> |
| ![cobalt](assets/cobalt.png)<br><sub>cobalt</sub> | ![copper](assets/copper.png)<br><sub>copper</sub> | ![coral](assets/coral.png)<br><sub>coral</sub> |
| ![cyberpunk](assets/cyberpunk.png)<br><sub>cyberpunk</sub> | ![dark](assets/dark.png)<br><sub>dark</sub> | ![dracula](assets/dracula.png)<br><sub>dracula</sub> |
| ![ember](assets/ember.png)<br><sub>ember</sub> | ![everforest](assets/everforest.png)<br><sub>everforest</sub> | ![fjord](assets/fjord.png)<br><sub>fjord</sub> |
| ![forest](assets/forest.png)<br><sub>forest</sub> | ![gameboy](assets/gameboy.png)<br><sub>gameboy</sub> | ![gold](assets/gold.png)<br><sub>gold</sub> |
| ![gotham](assets/gotham.png)<br><sub>gotham</sub> | ![gruvbox](assets/gruvbox.png)<br><sub>gruvbox</sub> | ![high_contrast](assets/high_contrast.png)<br><sub>high_contrast</sub> |
| ![horizon](assets/horizon.png)<br><sub>horizon</sub> | ![ice](assets/ice.png)<br><sub>ice</sub> | ![kanagawa](assets/kanagawa.png)<br><sub>kanagawa</sub> |
| ![lavender](assets/lavender.png)<br><sub>lavender</sub> | ![material](assets/material.png)<br><sub>material</sub> | ![matrix](assets/matrix.png)<br><sub>matrix</sub> |
| ![mellow](assets/mellow.png)<br><sub>mellow</sub> | ![midnight](assets/midnight.png)<br><sub>midnight</sub> | ![monokai](assets/monokai.png)<br><sub>monokai</sub> |
| ![moss](assets/moss.png)<br><sub>moss</sub> | ![navy](assets/navy.png)<br><sub>navy</sub> | ![neon](assets/neon.png)<br><sub>neon</sub> |
| ![nightfox](assets/nightfox.png)<br><sub>nightfox</sub> | ![nord](assets/nord.png)<br><sub>nord</sub> | ![ocean](assets/ocean.png)<br><sub>ocean</sub> |
| ![olive](assets/olive.png)<br><sub>olive</sub> | ![onedark](assets/onedark.png)<br><sub>onedark</sub> | ![oxocarbon](assets/oxocarbon.png)<br><sub>oxocarbon</sub> |
| ![palenight](assets/palenight.png)<br><sub>palenight</sub> | ![paper](assets/paper.png)<br><sub>paper</sub> | ![plum](assets/plum.png)<br><sub>plum</sub> |
| ![retro](assets/retro.png)<br><sub>retro</sub> | ![rose_pine](assets/rose_pine.png)<br><sub>rose_pine</sub> | ![sakura](assets/sakura.png)<br><sub>sakura</sub> |
| ![sand](assets/sand.png)<br><sub>sand</sub> | ![slate](assets/slate.png)<br><sub>slate</sub> | ![solarized](assets/solarized.png)<br><sub>solarized</sub> |
| ![solarized_light](assets/solarized_light.png)<br><sub>solarized_light</sub> | ![steam_green](assets/steam_green.png)<br><sub>steam_green</sub> | ![sunset](assets/sunset.png)<br><sub>sunset</sub> |
| ![synthwave](assets/synthwave.png)<br><sub>synthwave</sub> | ![teal](assets/teal.png)<br><sub>teal</sub> | ![terminal](assets/terminal.png)<br><sub>terminal</sub> |
| ![tokyo_night](assets/tokyo_night.png)<br><sub>tokyo_night</sub> | ![tokyo_storm](assets/tokyo_storm.png)<br><sub>tokyo_storm</sub> | ![vapor](assets/vapor.png)<br><sub>vapor</sub> |
| ![virtualboy](assets/virtualboy.png)<br><sub>virtualboy</sub> | ![wine](assets/wine.png)<br><sub>wine</sub> | ![zx_spectrum](assets/zx_spectrum.png)<br><sub>zx_spectrum</sub> |

## Toggle Combo

Set `toggle_combo` in config to show/hide the keyboard with a button combo, no external tools needed:

```ini
[gamepad]
toggle_combo = guide+a       # or l3+r3, select+start, etc.
combo_period_ms = 200        # timing window (ms)
```

Available buttons: `a`, `b`, `x`, `y`, `lb`, `rb`, `lt`, `rt`, `l3`, `r3`, `start`, `select`, `guide`, `dpad_up`, `dpad_down`, `dpad_left`, `dpad_right`. Requires 2-4 buttons.

Leave `toggle_combo` empty to use `--toggle` / evsieve instead (default).

## Evsieve Integration

For advanced input routing, use evsieve. Example to toggle the keyboard with Guide+Start:

```bash
evsieve \
  --input /dev/input/gamepad0 grab \
  --hook key:btn_mode key:btn_start exec-shell="gamepad-osk --toggle" \
  --output
```

## Device Grab

When `grab = true` (default), the gamepad is exclusively grabbed while the keyboard is visible. This prevents controller input from bleeding into the game while you're typing. The grab is released when the keyboard hides.

If using evsieve, set `grab = false` in config and let evsieve handle routing.

## Wayland

Uses wlr-layer-shell for native overlay support - no compositor rules needed. The keyboard renders as a non-focusable overlay that stays above all windows. Position toggle (Start) works natively.

**Supported compositors:**
- **wlroots-based** (Sway, wayfire, river, labwc, dwl) - native
- **Hyprland** - native
- **KDE Plasma 6** - native (via Layer Shell Qt)
- **COSMIC (Pop!_OS)** - native (via Smithay)
- **GNOME (Mutter)** - no layer-shell support; falls back to standard window (same as X11 without hints)

## License

MIT
