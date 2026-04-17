# Post-Release TODO

- [ ] Client-side opacity for Wayland (SDL_SetWindowOpacity is a no-op on Wayland). Multiply alpha on all renderer colors (background, keys, text, pills, flash, accent popup). Affects both X11 and Wayland if done unconditionally -- needs X11 regression testing.
