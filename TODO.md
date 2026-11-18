# Post-Release TODO

- [ ] Accents: é is sent as Ctrl+Shift+U 00e9 Enter, and GNOME's terminal (ptyxis) printed junk instead of é. Survey which apps get it wrong before changing anything. If it's common, type the char directly via zwp_virtual_keyboard_v1 (Sway has it, KDE and GNOME don't) and keep Ctrl+Shift+U as fallback. Idea from Kam1ni/gamepad-osk-no-uinput.
- [ ] KDE Alt+Tab: quick second Alt+Tab sometimes waits for the 3 s Alt timeout before Tab is sent. Only seen in a Plasma VM during a scripted sequence -- not when run alone, not on X11/Sway/GNOME. Input reads were on time. Verify on real KDE hardware before touching code.
