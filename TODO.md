# Post-Release TODO

- [x] Opacity on Wayland layer-shell: compositor alpha modifier (`wp_alpha_modifier_v1`), or an off-screen layer drawn with alpha where the compositor lacks it. X11 keeps window opacity.
- [ ] GNOME: fall back to a normal xdg_toplevel window when the compositor has no wlr-layer-shell. Today the surface gets no role and the keyboard never appears.
