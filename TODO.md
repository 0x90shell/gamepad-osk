# Post-Release TODO

- [ ] Accents: é is sent as Ctrl+Shift+U 00e9 Enter, and GNOME's terminal (ptyxis) printed junk instead of é. Survey which apps get it wrong before changing anything. If it's common, type the char directly via zwp_virtual_keyboard_v1 (Sway has it, KDE and GNOME don't) and keep Ctrl+Shift+U as fallback. Idea from Kam1ni/gamepad-osk-no-uinput.
- [ ] KDE Alt+Tab: quick second Alt+Tab sometimes waits for the 3 s Alt timeout before Tab is sent. Only seen in a Plasma VM during a scripted sequence -- not when run alone, not on X11/Sway/GNOME. Input reads were on time. Verify on real KDE hardware before touching code.
- [ ] Extra buttons: DualSense Edge paddles and Fn (BTN_TRIGGER_HAPPY1-4), maybe Elite and Deck back buttons too. Ignored right now, which is fine. Could add names like p1-p4 to the button table so people can map them. Check what xpad sends for Elite paddles first.
