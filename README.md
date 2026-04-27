# gamepad-osk

Gamepad-controlled on-screen keyboard for Linux. Designed for couch gaming, Sunshine/Moonlight streaming, and any setup where you need to type with a controller.

No Steam dependency. Works on X11 and Wayland (key injection via uinput).

![gamepad-osk](assets/matrix-hero.png)

## Table of Contents

- [Features](#features)
- [Controls](#controls)
- [Usage](#usage)
- [Installation](#installation)
  - [AUR (Arch Linux)](#aur-arch-linux)
  - [Pre-built binary (x86_64)](#pre-built-binary-x86_64)
  - [From source](#from-source)
  - [Bazzite / Immutable Fedora](#bazzite--immutable-fedora)
  - [Promptfont](#promptfont)
- [Permissions](#permissions)
- [Systemd User Service](#systemd-user-service)
- [Configuration](#configuration)
- [Themes](#themes)
- [Show & Hide](#show--hide)
- [X11](#x11)
- [Wayland](#wayland)
- [License](#license)

## Features

- Full QWERTY keyboard with shortcuts row (Undo, Redo, Cut, Select All, Alt+Tab, etc.)
- Native Wayland overlay via wlr-layer-shell (Sway, Hyprland, KDE, COSMIC - no compositor rules needed)
- Evdev gamepad input (works with any controller)
- Xbox pad auto-detection (swap_xy for xpad/xpadneo/xone drivers)
- 60 color themes (cycle live with Cfg key, or set via config/flag)
- Promptfont controller-agnostic button glyphs on mapped keys
- Mouse cursor via right stick + R3/RB click (hold to drag)
- Live mouse sensitivity adjustment (Shift + arrow keys, saved to config)
- Auto-reconnect on controller disconnect (timeout, power-off, unplug)
- Key repeat on hold (configurable delay and rate)
- Shift layer, Caps Lock, accent popup (Shift + hold on vowels)
- Paste/Copy key, media keys (Play/Pause, Mute)
- Always-on-top, no focus stealing (types into whatever app has focus)
- Multi-monitor aware (positions on primary monitor)
- Configurable: buttons, sticks, theme, scale, position, opacity, deadzone
- IPC toggle via Unix socket (`gamepad-osk --toggle`) for evsieve/hotkey integration
- Built-in configurable toggle combo (e.g. `guide+a`, `l3+r3`) for zero-dependency show/hide
- Daemon mode for systemd user service (close button hides instead of exiting)
- Single static binary, ~5MB

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
| Shift (LT) + up/down arrow | Adjust mouse sensitivity (saved to config) |
| Cfg key | Cycle themes (Shift+Cfg = reverse) |
| Toggle combo (configurable) | Show/hide keyboard |

## Usage

```
gamepad-osk                          # start (auto-detect gamepad)
gamepad-osk --device /dev/input/X    # use specific device
gamepad-osk --theme synthwave        # start with theme
gamepad-osk --config /path/to/config  # use specific config file
gamepad-osk --toggle                 # toggle running instance
gamepad-osk --daemon                 # start hidden, B hides instead of exiting
gamepad-osk --help                   # show all options
```

## Installation

**Runtime dependencies:** SDL3, SDL3_ttf, wayland, libX11, [promptfont](https://codeberg.org/shinmera/promptfont)

**Build dependencies:** Go, SDL3 (dev), SDL3_ttf (dev), libX11 (dev), wayland (dev), wayland-protocols (dev)

Package names vary by distro. Find yours below:

| Distro | Base | Instructions |
|--------|------|--------------|
| Manjaro, EndeavourOS | Arch | [AUR / Arch](#aur-arch-linux) |
| CachyOS, Garuda, ChimeraOS | Arch | [AUR / Arch](#aur-arch-linux) |
| Nobara | Fedora | [Fedora](#from-source) |
| Bazzite, Bluefin | Fedora Atomic | [Immutable Fedora](#bazzite--immutable-fedora) |
| Pop!_OS, Linux Mint | Ubuntu | [Debian / Ubuntu](#from-source) |

### AUR (Arch Linux)

The fastest way to install on Arch:

```bash
yay -S gamepad-osk-bin   # pre-built binary from GitHub release
yay -S gamepad-osk-git   # build from latest source
```

AUR packages install all dependencies, the systemd service, udev rules, and promptfont automatically.

To auto-update `-git` packages when upstream changes, enable devel checking:

```bash
yay --devel --save
```

### Pre-built binary (x86_64)

Download the binary and support files from the [releases page](https://github.com/0x90shell/gamepad-osk/releases). This is the fastest option for non-Arch distros.

Install runtime dependencies for your distro first (see [Promptfont](#promptfont) and the dep commands in [From source](#from-source)), then:

```bash
chmod +x gamepad-osk
sudo install -Dm755 gamepad-osk /usr/bin/gamepad-osk
sudo install -Dm644 gamepad-osk.service /usr/lib/systemd/user/gamepad-osk.service
sudo install -Dm644 gamepad-osk.udev /usr/lib/udev/rules.d/80-gamepad-osk.rules
sudo udevadm control --reload-rules
```

A default config is auto-copied to `~/.config/gamepad-osk/` on first run.

### From source

Clone and build:

```bash
git clone https://github.com/0x90shell/gamepad-osk.git
cd gamepad-osk
go build -o gamepad-osk .
sudo install -Dm755 gamepad-osk /usr/bin/gamepad-osk
sudo install -Dm644 config.example /usr/share/gamepad-osk/config
sudo install -Dm644 gamepad-osk.service /usr/lib/systemd/user/gamepad-osk.service
sudo install -Dm644 gamepad-osk.udev /usr/lib/udev/rules.d/80-gamepad-osk.rules
sudo udevadm control --reload-rules
```

Install build dependencies for your distro before building:

**Arch:**

```bash
sudo pacman -S go sdl3 sdl3_ttf libx11 wayland wlr-protocols
yay -S ttf-promptfont
```

**Fedora / Nobara:**

```bash
sudo dnf install golang SDL3-devel SDL3_ttf-devel libX11-devel wayland-devel wayland-protocols-devel
```

**Debian 13+ / Ubuntu 25.04+:**

```bash
sudo apt install golang-go libsdl3-dev libsdl3-ttf-dev libx11-dev libwayland-dev wayland-protocols
```

SDL3 is not available on Ubuntu 24.04 or Debian 12. Use a newer release or build SDL3 from source.

### Bazzite / Immutable Fedora

gamepad-osk needs raw access to `/dev/input` and `/dev/uinput` for gamepad reading and key injection. This rules out Flatpak. On immutable Fedora-based systems (Bazzite, Bluefin, Fedora Atomic), layer the runtime dependencies and reboot:

```bash
rpm-ostree install SDL3 SDL3_ttf
systemctl reboot
```

The udev rule goes in `/etc` which is writable:

```bash
sudo cp gamepad-osk.udev /etc/udev/rules.d/80-gamepad-osk.rules
sudo udevadm control --reload-rules
```

**Option 1: Pre-built binary (recommended).** Download from the [releases page](https://github.com/0x90shell/gamepad-osk/releases) and copy to `~/.local/bin/`:

```bash
chmod +x gamepad-osk
mkdir -p ~/.local/bin
cp gamepad-osk ~/.local/bin/
```

**Option 2: Build in a distrobox.**

```bash
distrobox create --name build --image fedora:41
distrobox enter build
sudo dnf install golang SDL3-devel SDL3_ttf-devel libX11-devel wayland-devel wayland-protocols-devel
git clone https://github.com/0x90shell/gamepad-osk.git
cd gamepad-osk
go build -o gamepad-osk .
cp gamepad-osk ~/.local/bin/
exit
```

Make sure `~/.local/bin` is in your `$PATH`.

### Promptfont

[Promptfont](https://codeberg.org/shinmera/promptfont) displays controller button glyphs on mapped keys. Without it, those keys show text labels instead.

**Arch:**

```bash
yay -S ttf-promptfont
```

**All other distros:**

```bash
wget https://codeberg.org/shinmera/promptfont/releases/download/v1.14/promptfont.zip
unzip promptfont.zip promptfont.ttf
mkdir -p ~/.local/share/fonts
mv promptfont.ttf ~/.local/share/fonts/
fc-cache
rm promptfont.zip
```

## Permissions

gamepad-osk reads directly from `/dev/input` for gamepad input and `/dev/uinput` for key injection. Your user must be in the `input` group:

```bash
sudo usermod -aG input $USER
```

Log out and back in for the group change to take effect. Verify with `groups`.

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

## Show & Hide

### Toggle Combo

Set `toggle_combo` in config to show/hide the keyboard with a button combo, no external tools needed:

```ini
[gamepad]
toggle_combo = guide+a       # or l3+r3, select+start, etc.
combo_period_ms = 200        # timing window (ms)
```

Available buttons: `a`, `b`, `x`, `y`, `lb`, `rb`, `lt`, `rt`, `l3`, `r3`, `start`, `select`, `guide`, `dpad_up`, `dpad_down`, `dpad_left`, `dpad_right`. Requires 2-4 buttons.

Works in both normal and daemon mode. Pair with `--daemon` to keep the OSK running as a service (B hides instead of exiting).

`--toggle` / evsieve works independently of this setting. Leave `toggle_combo` empty to rely on those methods alone (default).

### Evsieve Integration

If you already use [evsieve](https://github.com/KarsMulder/evsieve) for gamepad hotkeys, add a single hook to toggle the OSK alongside your existing combos:

```bash
evsieve \
  --input /dev/input/gamepad0 \
  --hook btn:select btn:start period=200 exec-shell="gamepad-osk --toggle"
```

Run `gamepad-osk --daemon` in the background so `--toggle` has something to signal. When the OSK is hidden, both evsieve and the daemon read the device nonexclusively — games still see input. When shown, gamepad-osk takes an exclusive grab so typing doesn't leak into the game; press **B** to hide and release the grab.

For zero-dependency show/hide without evsieve, use the built-in `toggle_combo` instead.

### Device Grab

When `grab = true` (default), the gamepad is exclusively grabbed while the keyboard is visible. This prevents controller input from bleeding into the game while you're typing. The grab is released when the keyboard hides.

## X11

Works on any EWMH-compliant window manager. The keyboard renders always-on-top without stealing focus from the active window.

**Fullscreen games (Bottles/Wine):** Toggling the OSK will not cause fullscreen games to minimize. On X11, the keyboard detects fullscreen windows and positions at the screen edge (ignoring panel/taskbar offsets). On hide, the pointer is returned to the game window center to restore input focus. On Wayland, set `panel_avoid = false` in config to position at the screen edge regardless of panels.

**Toggle combo tip:** Avoid using joystick clicks (L3/R3) in your toggle combo if they are mapped to mouse click or other actions. A combo like `guide+a` or `select+start` avoids conflicts. If the combo includes a button with an action, the first button pressed fires its action before the combo is detected.

If the OSK appears behind the game window, run the game in windowed or borderless mode.

## Wayland

Uses wlr-layer-shell for native overlay support - no compositor rules needed. The keyboard renders as a non-focusable overlay that stays above all windows. Position toggle (Start) works natively.

**Supported compositors:**
- **wlroots-based** (Sway, wayfire, river, labwc, dwl) - native
- **Hyprland** - native
- **KDE Plasma 6** - native (via Layer Shell Qt)
- **COSMIC (Pop!_OS)** - native (via Smithay)
- **GNOME (Mutter)** - no layer-shell support; falls back to standard window (same as X11 without hints)

**Wayland limitations:**
- Pointer warp on hide is not available (Wayland does not allow clients to move the cursor). If you move the mouse to another monitor with the right stick, you need to move it back before hiding.
- Window opacity (`opacity` config) is not supported. SDL_SetWindowOpacity is a no-op on Wayland. See TODO.md for planned client-side fallback.

## License

MIT
