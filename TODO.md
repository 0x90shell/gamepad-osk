# TODO

## Wayland: wlr-layer-shell (v1.2.0)
Native overlay support for Sway/Hyprland/KDE — replaces compositor rules for positioning, always-on-top, and no-focus. Currently researching approach:
- **SDL3 cgo**: Call SDL3's `CreateWindowWithProperties` with roleless surface flag, attach layer-shell via cgo. sdl2-compat already wraps SDL3. ~400-600 lines, medium effort.
- **purego-sdl3**: Replace go-sdl2 with SDL3 Go binding. Full SDL3 access but large refactor touching every file.
- **Raw Wayland + EGL**: Bypass SDL2 for window management on Wayland entirely. Most control but biggest rewrite.

SDL2 cannot do layer-shell (xdg-toplevel role assigned at window creation, Wayland forbids role changes). SDL3 added roleless surface support in Jan 2024.

## Bugs
- Dual instance conflict: running two instances grabs the controller and neither responds. Need IPC detection to prevent or warn.

## Minor
- Rename `bottom_margin` to `margin` or split into `top_margin`/`bottom_margin` — currently the same value is used for both positions but the name implies bottom-only.
