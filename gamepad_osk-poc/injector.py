"""Key injection via evdev UInput — works on X11 and Wayland."""

import time
from evdev import UInput, ecodes


class KeyInjector:
    def __init__(self):
        self.ui = UInput(
            {
                ecodes.EV_KEY: list(range(1, 249)),
                ecodes.EV_REL: [ecodes.REL_X, ecodes.REL_Y],
            },
            name="gamepad-osk",
            vendor=0x1234,
            product=0x5678,
        )

    def press_key(self, code, modifiers=None):
        """Press and release a key with optional modifiers."""
        mods = modifiers or set()
        for m in mods:
            self.ui.write(ecodes.EV_KEY, m, 1)
        self.ui.syn()
        self.ui.write(ecodes.EV_KEY, code, 1)
        self.ui.syn()
        self.ui.write(ecodes.EV_KEY, code, 0)
        self.ui.syn()
        for m in mods:
            self.ui.write(ecodes.EV_KEY, m, 0)
        self.ui.syn()

    def type_unicode(self, codepoint):
        """Inject Unicode char via Ctrl+Shift+U + hex + Enter (GTK/Qt method)."""
        # Press Ctrl+Shift+U
        self.ui.write(ecodes.EV_KEY, ecodes.KEY_LEFTCTRL, 1)
        self.ui.write(ecodes.EV_KEY, ecodes.KEY_LEFTSHIFT, 1)
        self.ui.syn()
        self.ui.write(ecodes.EV_KEY, ecodes.KEY_U, 1)
        self.ui.syn()
        self.ui.write(ecodes.EV_KEY, ecodes.KEY_U, 0)
        self.ui.syn()
        self.ui.write(ecodes.EV_KEY, ecodes.KEY_LEFTSHIFT, 0)
        self.ui.write(ecodes.EV_KEY, ecodes.KEY_LEFTCTRL, 0)
        self.ui.syn()

        # Small delay for input method to activate
        time.sleep(0.02)

        # Type hex digits
        hex_str = f"{codepoint:04x}"
        hex_key_map = {
            "0": ecodes.KEY_0, "1": ecodes.KEY_1, "2": ecodes.KEY_2,
            "3": ecodes.KEY_3, "4": ecodes.KEY_4, "5": ecodes.KEY_5,
            "6": ecodes.KEY_6, "7": ecodes.KEY_7, "8": ecodes.KEY_8,
            "9": ecodes.KEY_9, "a": ecodes.KEY_A, "b": ecodes.KEY_B,
            "c": ecodes.KEY_C, "d": ecodes.KEY_D, "e": ecodes.KEY_E,
            "f": ecodes.KEY_F,
        }
        for ch in hex_str:
            code = hex_key_map[ch]
            self.ui.write(ecodes.EV_KEY, code, 1)
            self.ui.syn()
            self.ui.write(ecodes.EV_KEY, code, 0)
            self.ui.syn()

        # Confirm with Enter
        self.ui.write(ecodes.EV_KEY, ecodes.KEY_ENTER, 1)
        self.ui.syn()
        self.ui.write(ecodes.EV_KEY, ecodes.KEY_ENTER, 0)
        self.ui.syn()

    def move_mouse(self, dx, dy):
        """Move mouse cursor by relative amount."""
        if dx != 0:
            self.ui.write(ecodes.EV_REL, ecodes.REL_X, int(dx))
        if dy != 0:
            self.ui.write(ecodes.EV_REL, ecodes.REL_Y, int(dy))
        if dx != 0 or dy != 0:
            self.ui.syn()

    def close(self):
        try:
            self.ui.close()
        except Exception:
            pass
