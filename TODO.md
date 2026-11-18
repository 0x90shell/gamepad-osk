# TODO

- Accents: é goes out as Ctrl+Shift+U 00e9 Enter. GNOME's terminal (ptyxis) took that as plain keys and printed junk instead of é. Check which apps get it wrong on each test desktop before changing anything. If it's common, type the character directly with zwp_virtual_keyboard_v1 where the compositor has it (Sway does, KDE and GNOME don't) and keep Ctrl+Shift+U as the fallback. Idea from Kam1ni/gamepad-osk-no-uinput.
