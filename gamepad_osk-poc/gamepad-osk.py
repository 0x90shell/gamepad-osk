#!/usr/bin/env python3
"""gamepad-osk: Python proof-of-concept (DEPRECATED — see Go implementation in repo root)

This was the initial prototype used to iterate on the design. Ported to Go for:
- Single static binary (no python-pygame/python-evdev runtime deps)
- SDL2 GameController API for normalized input (fixes Xbox 360 button mapping)
- CGo uinput for reliable mouse REL event registration
- ~50ms startup vs ~500ms Python
- Simpler AUR packaging

Kept as reference. Use the Go version.
"""

import ctypes
import ctypes.util
import fcntl
import os
import select
import socket
import sys
import threading
import time
from dataclasses import dataclass, field

import pygame
from evdev import InputDevice, UInput, ecodes, list_devices

__version__ = "0.1.0"

# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------

if sys.version_info >= (3, 11):
    import tomllib
else:
    try:
        import tomli as tomllib
    except ImportError:
        tomllib = None

DEFAULTS = {
    "theme": {"name": "dark"},
    "window": {"bottom_margin": 20, "opacity": 0.95},
    "keys": {"unit_size": 0, "padding": 4, "font_size": 0},
    "gamepad": {"device": "", "grab": True, "deadzone": 0.25, "long_press_ms": 500},
    "mouse": {"enabled": True, "sensitivity": 8},
}

CONFIG_PATHS = [
    os.path.expanduser("~/.config/gamepad-osk/config.toml"),
    os.path.join(os.path.dirname(os.path.abspath(__file__)), "config.toml"),
]


def _deep_merge(base, override):
    result = dict(base)
    for k, v in override.items():
        if k in result and isinstance(result[k], dict) and isinstance(v, dict):
            result[k] = _deep_merge(result[k], v)
        else:
            result[k] = v
    return result


class Config:
    def __init__(self, data):
        self._d = data

    theme_name = property(lambda s: s._d["theme"]["name"])
    bottom_margin = property(lambda s: s._d["window"]["bottom_margin"])
    opacity = property(lambda s: s._d["window"]["opacity"])
    unit_size = property(lambda s: s._d["keys"]["unit_size"])
    padding = property(lambda s: s._d["keys"]["padding"])
    font_size = property(lambda s: s._d["keys"]["font_size"])
    device = property(lambda s: s._d["gamepad"]["device"])
    grab = property(lambda s: s._d["gamepad"]["grab"])
    deadzone = property(lambda s: s._d["gamepad"]["deadzone"])
    long_press_ms = property(lambda s: s._d["gamepad"]["long_press_ms"])
    mouse_enabled = property(lambda s: s._d["mouse"]["enabled"])
    mouse_sensitivity = property(lambda s: s._d["mouse"]["sensitivity"])


def load_config():
    data = dict(DEFAULTS)
    for path in CONFIG_PATHS:
        if os.path.isfile(path):
            if tomllib is None:
                print(f"Warning: cannot parse {path} (no tomllib)", file=sys.stderr)
                break
            with open(path, "rb") as f:
                user = tomllib.load(f)
            data = _deep_merge(data, user)
            break
    return Config(data)


# ---------------------------------------------------------------------------
# Themes
# ---------------------------------------------------------------------------

@dataclass(frozen=True)
class Theme:
    name: str
    bg: tuple
    key_bg: tuple
    key_bg_pressed: tuple
    key_border: tuple
    key_text: tuple
    highlight_bg: tuple
    highlight_border: tuple
    modifier_bg: tuple
    modifier_active_bg: tuple
    modifier_text: tuple
    fn_key_bg: tuple
    accent_popup_bg: tuple
    accent_popup_text: tuple
    accent_highlight_bg: tuple
    glyph_color: tuple = (120, 120, 140)  # controller button glyph text


THEMES = {
    "dark": Theme("dark", (30, 30, 35), (55, 55, 65), (80, 80, 95), (70, 70, 80),
                  (220, 220, 230), (60, 110, 180), (80, 140, 220), (50, 50, 60),
                  (90, 70, 130), (180, 180, 200), (45, 45, 55), (50, 50, 60),
                  (220, 220, 230), (60, 110, 180)),
    "steam_green": Theme("steam_green", (22, 32, 22), (35, 55, 35), (50, 80, 50),
                         (45, 70, 45), (200, 220, 200), (56, 150, 60), (76, 185, 80),
                         (30, 48, 30), (40, 120, 45), (170, 200, 170), (28, 44, 28),
                         (32, 50, 32), (200, 220, 200), (56, 150, 60)),
    "candy": Theme("candy", (40, 18, 55), (70, 35, 90), (100, 55, 125), (85, 45, 110),
                   (240, 210, 250), (210, 80, 140), (240, 110, 170), (60, 28, 78),
                   (180, 60, 120), (220, 190, 230), (55, 25, 72), (65, 30, 85),
                   (240, 210, 250), (210, 80, 140)),
    "ocean": Theme("ocean", (15, 25, 45), (25, 45, 75), (35, 65, 105), (35, 55, 90),
                   (190, 215, 240), (30, 120, 190), (50, 150, 220), (20, 38, 65),
                   (25, 100, 170), (170, 200, 230), (20, 35, 60), (22, 40, 70),
                   (190, 215, 240), (30, 120, 190)),
    "solarized": Theme("solarized", (0, 43, 54), (7, 54, 66), (88, 110, 117),
                       (0, 54, 66), (147, 161, 161), (38, 139, 210), (42, 161, 152),
                       (0, 43, 54), (133, 153, 0), (131, 148, 150), (0, 38, 48),
                       (7, 54, 66), (147, 161, 161), (38, 139, 210)),
    "high_contrast": Theme("high_contrast", (0, 0, 0), (20, 20, 20), (60, 60, 60),
                           (200, 200, 200), (255, 255, 255), (255, 255, 0),
                           (255, 255, 255), (10, 10, 10), (200, 0, 0),
                           (255, 255, 255), (15, 15, 15), (20, 20, 20),
                           (255, 255, 255), (255, 255, 0),
                           glyph_color=(180, 180, 0)),
    "terminal": Theme("terminal", (0, 0, 0), (10, 18, 10), (20, 40, 20),
                      (0, 50, 0), (0, 204, 0), (0, 80, 0), (0, 120, 0),
                      (8, 14, 8), (0, 160, 0), (0, 140, 0), (6, 12, 6),
                      (10, 18, 10), (0, 204, 0), (0, 80, 0),
                      glyph_color=(0, 100, 0)),
}


# ---------------------------------------------------------------------------
# Keyboard Layout
# ---------------------------------------------------------------------------

@dataclass
class KeyDef:
    label: str
    code: int
    width: float = 1.0
    shift_label: str = ""
    is_modifier: bool = False
    modifier_type: str = ""
    accents: list = field(default_factory=list)


def _k(label, code, width=1.0, shift_label="", **kw):
    return KeyDef(label, code, width, shift_label, **kw)


def _mod(label, code, width=1.0, modifier_type=""):
    return KeyDef(label, code, width, is_modifier=True, modifier_type=modifier_type)


# Accents: (display_char, unicode_codepoint)
_AC_A = [("\u00e0", 0xe0), ("\u00e1", 0xe1), ("\u00e2", 0xe2),
         ("\u00e4", 0xe4), ("\u00e5", 0xe5), ("\u00e3", 0xe3)]
_AC_E = [("\u00e8", 0xe8), ("\u00e9", 0xe9), ("\u00ea", 0xea), ("\u00eb", 0xeb)]
_AC_I = [("\u00ec", 0xec), ("\u00ed", 0xed), ("\u00ee", 0xee), ("\u00ef", 0xef)]
_AC_O = [("\u00f2", 0xf2), ("\u00f3", 0xf3), ("\u00f4", 0xf4),
         ("\u00f6", 0xf6), ("\u00f5", 0xf5)]
_AC_U = [("\u00f9", 0xf9), ("\u00fa", 0xfa), ("\u00fb", 0xfb), ("\u00fc", 0xfc)]
_AC_N = [("\u00f1", 0xf1)]
_AC_C = [("\u00e7", 0xe7)]
_AC_S = [("\u00df", 0xdf)]

LAYOUT_QWERTY = [
    [_k("Esc", ecodes.KEY_ESC, 1.5),
     _k("F1", ecodes.KEY_F1), _k("F2", ecodes.KEY_F2), _k("F3", ecodes.KEY_F3),
     _k("F4", ecodes.KEY_F4), _k("F5", ecodes.KEY_F5), _k("F6", ecodes.KEY_F6),
     _k("F7", ecodes.KEY_F7), _k("F8", ecodes.KEY_F8), _k("F9", ecodes.KEY_F9),
     _k("F10", ecodes.KEY_F10), _k("F11", ecodes.KEY_F11), _k("F12", ecodes.KEY_F12),
     _k("Del", ecodes.KEY_DELETE, 1.5)],
    [_k("`", ecodes.KEY_GRAVE, shift_label="~"),
     _k("1", ecodes.KEY_1, shift_label="!"), _k("2", ecodes.KEY_2, shift_label="@"),
     _k("3", ecodes.KEY_3, shift_label="#"), _k("4", ecodes.KEY_4, shift_label="$"),
     _k("5", ecodes.KEY_5, shift_label="%"), _k("6", ecodes.KEY_6, shift_label="^"),
     _k("7", ecodes.KEY_7, shift_label="&"), _k("8", ecodes.KEY_8, shift_label="*"),
     _k("9", ecodes.KEY_9, shift_label="("), _k("0", ecodes.KEY_0, shift_label=")"),
     _k("-", ecodes.KEY_MINUS, shift_label="_"), _k("=", ecodes.KEY_EQUAL, shift_label="+"),
     _k("Bksp", ecodes.KEY_BACKSPACE, 2.0)],
    [_k("Tab", ecodes.KEY_TAB, 1.5),
     _k("q", ecodes.KEY_Q, shift_label="Q"),
     _k("w", ecodes.KEY_W, shift_label="W"),
     _k("e", ecodes.KEY_E, shift_label="E", accents=_AC_E),
     _k("r", ecodes.KEY_R, shift_label="R"), _k("t", ecodes.KEY_T, shift_label="T"),
     _k("y", ecodes.KEY_Y, shift_label="Y"),
     _k("u", ecodes.KEY_U, shift_label="U", accents=_AC_U),
     _k("i", ecodes.KEY_I, shift_label="I", accents=_AC_I),
     _k("o", ecodes.KEY_O, shift_label="O", accents=_AC_O),
     _k("p", ecodes.KEY_P, shift_label="P"),
     _k("[", ecodes.KEY_LEFTBRACE, shift_label="{"),
     _k("]", ecodes.KEY_RIGHTBRACE, shift_label="}"),
     _k("\\", ecodes.KEY_BACKSLASH, 1.5, shift_label="|")],
    [_mod("Caps", ecodes.KEY_CAPSLOCK, 1.75, "caps"),
     _k("a", ecodes.KEY_A, shift_label="A", accents=_AC_A),
     _k("s", ecodes.KEY_S, shift_label="S", accents=_AC_S),
     _k("d", ecodes.KEY_D, shift_label="D"), _k("f", ecodes.KEY_F, shift_label="F"),
     _k("g", ecodes.KEY_G, shift_label="G"), _k("h", ecodes.KEY_H, shift_label="H"),
     _k("j", ecodes.KEY_J, shift_label="J"), _k("k", ecodes.KEY_K, shift_label="K"),
     _k("l", ecodes.KEY_L, shift_label="L"),
     _k(";", ecodes.KEY_SEMICOLON, shift_label=":"),
     _k("'", ecodes.KEY_APOSTROPHE, shift_label='"'),
     _k("Enter", ecodes.KEY_ENTER, 2.25)],
    [_mod("Shift", ecodes.KEY_LEFTSHIFT, 2.25, "shift"),
     _k("z", ecodes.KEY_Z, shift_label="Z"), _k("x", ecodes.KEY_X, shift_label="X"),
     _k("c", ecodes.KEY_C, shift_label="C", accents=_AC_C),
     _k("v", ecodes.KEY_V, shift_label="V"), _k("b", ecodes.KEY_B, shift_label="B"),
     _k("n", ecodes.KEY_N, shift_label="N", accents=_AC_N),
     _k("m", ecodes.KEY_M, shift_label="M"),
     _k(",", ecodes.KEY_COMMA, shift_label="<"), _k(".", ecodes.KEY_DOT, shift_label=">"),
     _k("/", ecodes.KEY_SLASH, shift_label="?"),
     _mod("Shift", ecodes.KEY_RIGHTSHIFT, 2.75, "shift")],
    [_mod("Ctrl", ecodes.KEY_LEFTCTRL, 1.5, "ctrl"),
     _mod("Super", ecodes.KEY_LEFTMETA, 1.25, "meta"),
     _mod("Alt", ecodes.KEY_LEFTALT, 1.25, "alt"),
     _k("Space", ecodes.KEY_SPACE, 6.0),
     _mod("Alt", ecodes.KEY_RIGHTALT, 1.25, "alt"),
     _k("\u2190", ecodes.KEY_LEFT, shift_label="\u2191"),
     _k("\u2192", ecodes.KEY_RIGHT, shift_label="\u2193"),
     _k("Paste", ecodes.KEY_V, 1.5)],  # Paste = Ctrl+V, handled specially
]


# ---------------------------------------------------------------------------
# Keyboard State
# ---------------------------------------------------------------------------

class KeyboardState:
    def __init__(self, layout):
        self.layout = layout
        self.cursor_row = 2
        self.cursor_col = 1  # land on 'q'
        self.shift_active = False
        self.caps_active = False
        self.ctrl_active = False
        self.alt_active = False
        self.meta_active = False
        self.long_press_start = 0.0
        self.long_press_active = False
        self.accent_popup = None  # (accents_list, selected_index)
        self._target_x = None
        # Visual flash: evdev keycode that was just triggered by a shortcut button
        self.flash_code = 0
        self.flash_until = 0.0

    def navigate(self, dx, dy):
        if self.accent_popup is not None:
            accents, idx = self.accent_popup
            new_idx = idx + dx
            if 0 <= new_idx < len(accents):
                self.accent_popup = (accents, new_idx)
            return
        if dy != 0:
            if self._target_x is None:
                self._target_x = self._key_center_x(self.cursor_row, self.cursor_col)
            self.cursor_row = (self.cursor_row + dy) % len(self.layout)
            self.cursor_col = self._find_closest_col(self.cursor_row, self._target_x)
        elif dx != 0:
            row = self.layout[self.cursor_row]
            c = self.cursor_col + dx
            self.cursor_col = c % len(row)
            self._target_x = None

    def _key_center_x(self, row_idx, col_idx):
        x = 0.0
        for i, key in enumerate(self.layout[row_idx]):
            if i == col_idx:
                return x + key.width / 2.0
            x += key.width
        return x

    def _find_closest_col(self, row_idx, target_x):
        best_col, best_dist, x = 0, float("inf"), 0.0
        for i, key in enumerate(self.layout[row_idx]):
            center = x + key.width / 2.0
            dist = abs(center - target_x)
            if dist < best_dist:
                best_dist = dist
                best_col = i
            x += key.width
        return best_col

    def get_current_key(self):
        row = self.layout[self.cursor_row]
        return row[min(self.cursor_col, len(row) - 1)]

    def get_display_label(self, key):
        if (self.shift_active ^ self.caps_active) and key.shift_label:
            return key.shift_label
        return key.label

    def press_current(self, injector):
        if self.accent_popup is not None:
            _, idx = self.accent_popup
            label, codepoint = self.accent_popup[0][idx]
            injector.type_unicode(codepoint)
            self.close_accent_popup()
            return
        key = self.get_current_key()
        if key.is_modifier:
            self._toggle_modifier(key)
            return

        # Paste button: always sends Ctrl+V
        if key.label == "Paste":
            injector.press_key(ecodes.KEY_V, {ecodes.KEY_LEFTCTRL})
            return

        # Arrow keys with shift: Left→Up, Right→Down
        shift_on = self.shift_active ^ self.caps_active
        code = key.code
        if shift_on and key.code == ecodes.KEY_LEFT:
            code = ecodes.KEY_UP
        elif shift_on and key.code == ecodes.KEY_RIGHT:
            code = ecodes.KEY_DOWN

        mods = set()
        # XOR: shift when either shift or caps is active, but not both
        # Don't send shift for arrow keys (they switch direction instead)
        if shift_on and code not in (ecodes.KEY_UP, ecodes.KEY_DOWN,
                                     ecodes.KEY_LEFT, ecodes.KEY_RIGHT):
            mods.add(ecodes.KEY_LEFTSHIFT)
        if self.ctrl_active:
            mods.add(ecodes.KEY_LEFTCTRL)
        if self.alt_active:
            mods.add(ecodes.KEY_LEFTALT)
        if self.meta_active:
            mods.add(ecodes.KEY_LEFTMETA)
        injector.press_key(code, mods)
        if self.shift_active and not self.caps_active:
            self.shift_active = False
        self.ctrl_active = False
        self.alt_active = False
        self.meta_active = False

    def start_long_press(self):
        key = self.get_current_key()
        if key.accents:
            self.long_press_start = time.monotonic()
            self.long_press_active = True

    def cancel_long_press(self):
        self.long_press_active = False
        self.long_press_start = 0.0

    def check_long_press(self, long_press_ms):
        if not self.long_press_active:
            return False
        if (time.monotonic() - self.long_press_start) * 1000 >= long_press_ms:
            key = self.get_current_key()
            if key.accents:
                self.accent_popup = (key.accents, 0)
                self.long_press_active = False
                return True
        return False

    def close_accent_popup(self):
        self.accent_popup = None
        self.long_press_active = False

    def flash_key(self, keycode, duration=0.15):
        """Briefly highlight a key by its evdev code (for shortcut buttons)."""
        self.flash_code = keycode
        self.flash_until = time.monotonic() + duration

    def is_flashed(self, key):
        if self.flash_code and key.code == self.flash_code:
            if time.monotonic() < self.flash_until:
                return True
            self.flash_code = 0
        return False

    def _toggle_modifier(self, key):
        attr = {"shift": "shift_active", "caps": "caps_active", "ctrl": "ctrl_active",
                "alt": "alt_active", "meta": "meta_active"}.get(key.modifier_type)
        if attr:
            setattr(self, attr, not getattr(self, attr))


# ---------------------------------------------------------------------------
# Key Injector (uinput)
# ---------------------------------------------------------------------------

class KeyInjector:
    def __init__(self):
        self.ui = UInput(
            {ecodes.EV_KEY: list(range(1, 249)),
             ecodes.EV_REL: [ecodes.REL_X, ecodes.REL_Y]},
            name="gamepad-osk", vendor=0x1234, product=0x5678,
        )

    def press_key(self, code, modifiers=None):
        for m in (modifiers or set()):
            self.ui.write(ecodes.EV_KEY, m, 1)
        self.ui.syn()
        self.ui.write(ecodes.EV_KEY, code, 1)
        self.ui.syn()
        self.ui.write(ecodes.EV_KEY, code, 0)
        self.ui.syn()
        for m in (modifiers or set()):
            self.ui.write(ecodes.EV_KEY, m, 0)
        self.ui.syn()

    def type_unicode(self, codepoint):
        """Ctrl+Shift+U + hex + Enter (GTK/Qt Unicode input)."""
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
        time.sleep(0.02)
        hex_map = {c: getattr(ecodes, f"KEY_{c.upper()}") for c in "0123456789abcdef"}
        for ch in f"{codepoint:04x}":
            k = hex_map[ch]
            self.ui.write(ecodes.EV_KEY, k, 1)
            self.ui.syn()
            self.ui.write(ecodes.EV_KEY, k, 0)
            self.ui.syn()
        self.ui.write(ecodes.EV_KEY, ecodes.KEY_ENTER, 1)
        self.ui.syn()
        self.ui.write(ecodes.EV_KEY, ecodes.KEY_ENTER, 0)
        self.ui.syn()

    def move_mouse(self, dx, dy):
        if dx:
            self.ui.write(ecodes.EV_REL, ecodes.REL_X, int(dx))
        if dy:
            self.ui.write(ecodes.EV_REL, ecodes.REL_Y, int(dy))
        if dx or dy:
            self.ui.syn()

    def close(self):
        try:
            self.ui.close()
        except Exception:
            pass


# ---------------------------------------------------------------------------
# Gamepad Reader (evdev)
# ---------------------------------------------------------------------------

@dataclass
class NavAxis:
    direction: int = 0
    held_since: float = 0.0
    last_move: float = 0.0

    def repeat_interval(self):
        if self.held_since == 0:
            return 0.3
        t = min((time.monotonic() - self.held_since) / 1.0, 1.0)
        return 0.3 + (0.08 - 0.3) * t


@dataclass
class Action:
    type: str
    dx: int = 0
    dy: int = 0


class GamepadReader:
    def __init__(self, config):
        self.config = config
        self.device = None
        self.grabbed = False
        self.nav_x = NavAxis()
        self.nav_y = NavAxis()
        self.deadzone = config.deadzone
        self.axis_max = 32767
        self.trigger_max = 1023
        self.mouse_x = 0.0
        self.mouse_y = 0.0
        self.lt_active = False
        self.rt_active = False

    def open_device(self, device_path=None):
        path = device_path or self.config.device
        if path:
            self.device = InputDevice(path)
        else:
            self.device = self._auto_detect()
        if self.device:
            caps = self.device.capabilities(absinfo=True)
            for code, absinfo in caps.get(ecodes.EV_ABS, []):
                if code == ecodes.ABS_X:
                    self.axis_max = absinfo.max or 32767
                elif code == ecodes.ABS_Z:
                    self.trigger_max = absinfo.max or 1023
        return self.device is not None

    def _auto_detect(self):
        for path in list_devices():
            try:
                dev = InputDevice(path)
                caps = dev.capabilities()
                if ecodes.EV_ABS in caps and ecodes.EV_KEY in caps:
                    abs_codes = [c if isinstance(c, int) else c[0]
                                 for c in caps[ecodes.EV_ABS]]
                    if ecodes.ABS_X in abs_codes and ecodes.BTN_SOUTH in caps[ecodes.EV_KEY]:
                        print(f"Auto-detected gamepad: {dev.name} ({dev.path})")
                        return dev
                dev.close()
            except (PermissionError, OSError):
                continue
        return None

    def grab(self):
        if self.device and not self.grabbed:
            try:
                self.device.grab()
                self.grabbed = True
            except OSError as e:
                print(f"Warning: could not grab device: {e}")

    def ungrab(self):
        if self.device and self.grabbed:
            try:
                self.device.ungrab()
                self.grabbed = False
            except OSError:
                pass

    def fileno(self):
        return self.device.fd if self.device else -1

    def process_events(self):
        if not self.device:
            return []
        actions = []
        try:
            for event in self.device.read():
                a = self._handle_event(event)
                if a:
                    actions.append(a)
        except BlockingIOError:
            pass
        now = time.monotonic()
        for nav, is_x in [(self.nav_x, True), (self.nav_y, False)]:
            if nav.direction != 0 and now - nav.last_move >= nav.repeat_interval():
                nav.last_move = now
                actions.append(Action("navigate",
                                      dx=nav.direction if is_x else 0,
                                      dy=0 if is_x else nav.direction))
        if self.config.mouse_enabled and (abs(self.mouse_x) > 0.01 or abs(self.mouse_y) > 0.01):
            s = self.config.mouse_sensitivity
            actions.append(Action("mouse_move", dx=int(self.mouse_x * s),
                                  dy=int(self.mouse_y * s)))
        return actions

    def _handle_event(self, event):
        if event.type == ecodes.EV_KEY:
            return self._handle_button(event.code, event.value)
        elif event.type == ecodes.EV_ABS:
            return self._handle_axis(event.code, event.value)
        return None

    def _handle_button(self, code, value):
        # Steam OSK mapping (Xbox 360 layout)
        if code == ecodes.BTN_SOUTH:  # A — press highlighted key
            return Action("press_start") if value == 1 else Action("press")
        elif code == ecodes.BTN_EAST and value == 1:  # B — close
            return Action("close")
        elif code == ecodes.BTN_NORTH and value == 1:  # X — backspace
            return Action("backspace")
        elif code == ecodes.BTN_WEST and value == 1:  # Y — space
            return Action("space")
        elif code == ecodes.BTN_TL and value == 1:  # LB — backspace (Steam legacy)
            return Action("backspace")
        elif code == ecodes.BTN_TR and value == 1:  # RB — space (Steam legacy)
            return Action("space")
        elif code == ecodes.BTN_THUMBL and value == 1:  # L3 — caps lock
            return Action("caps_toggle")
        return None

    def _handle_axis(self, code, value):
        now = time.monotonic()
        dz = self.deadzone
        if code == ecodes.ABS_RX:
            return self._update_nav(self.nav_x, self._dz(value / self.axis_max, dz), now, True)
        elif code == ecodes.ABS_RY:
            return self._update_nav(self.nav_y, self._dz(value / self.axis_max, dz), now, False)
        elif code == ecodes.ABS_HAT0X:
            return self._update_nav(self.nav_x, value, now, True)
        elif code == ecodes.ABS_HAT0Y:
            return self._update_nav(self.nav_y, value, now, False)
        elif code == ecodes.ABS_X:
            n = value / self.axis_max
            self.mouse_x = 0.0 if abs(n) < dz else n
        elif code == ecodes.ABS_Y:
            n = value / self.axis_max
            self.mouse_y = 0.0 if abs(n) < dz else n
        elif code == ecodes.ABS_Z:
            active = value > self.trigger_max * 0.3
            if active != self.lt_active:
                self.lt_active = active
                return Action("shift_on" if active else "shift_off")
        elif code == ecodes.ABS_RZ:
            active = value > self.trigger_max * 0.3
            if active and not self.rt_active:
                self.rt_active = True
                return Action("enter")
            elif not active:
                self.rt_active = False
        return None

    @staticmethod
    def _dz(normalized, deadzone):
        if normalized > deadzone:
            return 1
        elif normalized < -deadzone:
            return -1
        return 0

    def _update_nav(self, nav, direction, now, is_x):
        if direction != nav.direction:
            nav.direction = direction
            if direction != 0:
                nav.held_since = now
                nav.last_move = now
                return Action("navigate",
                              dx=direction if is_x else 0,
                              dy=0 if is_x else direction)
            else:
                nav.held_since = 0.0
        return None

    def close(self):
        self.ungrab()
        if self.device:
            try:
                self.device.close()
            except Exception:
                pass


# ---------------------------------------------------------------------------
# IPC (Unix socket)
# ---------------------------------------------------------------------------

SOCK_PATH = os.path.expanduser("~/.cache/gamepad-osk.sock")


class IPCServer:
    def __init__(self, on_command):
        self.on_command = on_command
        self.sock = None
        self.running = False

    def start(self):
        if os.path.exists(SOCK_PATH):
            try:
                os.unlink(SOCK_PATH)
            except OSError:
                pass
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.bind(SOCK_PATH)
        self.sock.listen(1)
        self.sock.settimeout(1.0)
        self.running = True
        threading.Thread(target=self._listen, daemon=True).start()

    def _listen(self):
        while self.running:
            try:
                conn, _ = self.sock.accept()
                data = conn.recv(256).decode("utf-8").strip()
                if data:
                    self.on_command(data)
                conn.close()
            except socket.timeout:
                continue
            except OSError:
                break

    def stop(self):
        self.running = False
        if self.sock:
            try:
                self.sock.close()
            except Exception:
                pass
        try:
            os.unlink(SOCK_PATH)
        except OSError:
            pass


def ipc_send(cmd):
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    try:
        sock.connect(SOCK_PATH)
        sock.sendall(cmd.encode("utf-8"))
        return True
    except (ConnectionRefusedError, FileNotFoundError):
        return False
    finally:
        sock.close()


# ---------------------------------------------------------------------------
# Window Setup
# ---------------------------------------------------------------------------


def _sdl_set_window_pos(x, y):
    """Force window position via SDL2 C API (env var is unreliable)."""
    sdl2_name = ctypes.util.find_library("SDL2-2.0") or "libSDL2-2.0.so.0"
    try:
        sdl2 = ctypes.CDLL(sdl2_name)
        # SDL_GL_GetCurrentWindow or get from pygame internals
        sdl2.SDL_GetKeyboardFocus.restype = ctypes.c_void_p
        win = sdl2.SDL_GetKeyboardFocus()
        if not win:
            # Fallback: iterate windows (SDL2 doesn't have a simple getter)
            # Try the focused window approach
            return
        sdl2.SDL_SetWindowPosition.argtypes = [ctypes.c_void_p, ctypes.c_int, ctypes.c_int]
        sdl2.SDL_SetWindowPosition(win, x, y)
    except Exception:
        pass


def _apply_x11_hints():
    """Best-effort X11 window properties for overlay behavior."""
    try:
        wm_info = pygame.display.get_wm_info()
        window_id = wm_info.get("window")
    except Exception:
        return
    if not window_id:
        return

    x11_name = ctypes.util.find_library("X11")
    if not x11_name:
        return
    try:
        x11 = ctypes.CDLL(x11_name)
        # Set proper 64-bit return types for X11 functions
        x11.XOpenDisplay.restype = ctypes.c_void_p
        x11.XOpenDisplay.argtypes = [ctypes.c_char_p]
        x11.XInternAtom.restype = ctypes.c_ulong
        x11.XInternAtom.argtypes = [ctypes.c_void_p, ctypes.c_char_p, ctypes.c_int]
        x11.XChangeProperty.argtypes = [
            ctypes.c_void_p, ctypes.c_ulong, ctypes.c_ulong, ctypes.c_ulong,
            ctypes.c_int, ctypes.c_int, ctypes.c_char_p, ctypes.c_int]
        x11.XFlush.argtypes = [ctypes.c_void_p]
        x11.XCloseDisplay.argtypes = [ctypes.c_void_p]

        display_name = os.environ.get("DISPLAY")
        if not display_name:
            return
        display = x11.XOpenDisplay(display_name.encode())
        if not display:
            return

        def intern(name):
            return x11.XInternAtom(display, name.encode(), False)

        wm_state = intern("_NET_WM_STATE")
        above = intern("_NET_WM_STATE_ABOVE")
        skip_tb = intern("_NET_WM_STATE_SKIP_TASKBAR")
        skip_pg = intern("_NET_WM_STATE_SKIP_PAGER")
        atoms = (ctypes.c_long * 3)(above, skip_tb, skip_pg)
        x11.XChangeProperty(display, window_id, wm_state, intern("ATOM"),
                            32, 0, ctypes.cast(atoms, ctypes.c_char_p), 3)
        # WM_HINTS: don't take input focus
        wm_hints = intern("WM_HINTS")
        hints = (ctypes.c_long * 9)()
        hints[0] = 1  # InputHint
        hints[1] = 0  # input = False
        x11.XChangeProperty(display, window_id, wm_hints, wm_hints,
                            32, 0, ctypes.cast(hints, ctypes.c_char_p), 9)
        x11.XFlush(display)
        x11.XCloseDisplay(display)
    except Exception:
        pass


# ---------------------------------------------------------------------------
# Renderer
# ---------------------------------------------------------------------------

# Controller glyphs via Promptfont (ttf-promptfont AUR package)
# Generic face buttons by position + Xbox-style shoulder/trigger icons
PROMPTFONT_PATH = "/usr/share/fonts/TTF/promptfont.ttf"
KEY_GLYPHS = {
    ecodes.KEY_BACKSPACE: "\u21a4",   # face left (Xbox X, PS Square)
    ecodes.KEY_ENTER: "\u2197",       # right trigger (RT/R2)
    ecodes.KEY_LEFTSHIFT: "\u2196",   # left trigger (LT/L2)
    ecodes.KEY_RIGHTSHIFT: "\u2196",  # left trigger
    ecodes.KEY_CAPSLOCK: "\u21ba",    # left stick click (L3)
    ecodes.KEY_SPACE: "\u21a5",       # face top (Xbox Y, PS Triangle)
}


class Renderer:
    def __init__(self, surface, theme, unit_size, padding):
        self.surface = surface
        self.theme = theme
        self.unit = unit_size
        self.pad = padding
        font_size = max(12, int(unit_size * 0.32))
        font_small_size = max(10, font_size - 4)
        glyph_size = max(12, int(unit_size * 0.28))
        pygame.font.init()
        self.font = self.font_small = None
        for name in ("DejaVu Sans", "Liberation Sans", "FreeSans"):
            path = pygame.font.match_font(name)
            if path:
                self.font = pygame.font.Font(path, font_size)
                self.font_small = pygame.font.Font(path, font_small_size)
                break
        if not self.font:
            self.font = pygame.font.SysFont(None, font_size + 4)
            self.font_small = pygame.font.SysFont(None, font_size)
        # Promptfont for controller glyphs
        if os.path.isfile(PROMPTFONT_PATH):
            self.font_glyph = pygame.font.Font(PROMPTFONT_PATH, glyph_size)
        else:
            self.font_glyph = self.font_small  # fallback to text

    def draw(self, kb):
        self.surface.fill(self.theme.bg)
        y = self.pad
        for ri, row in enumerate(kb.layout):
            x = self.pad
            for ci, key in enumerate(row):
                self._draw_key(key, x, y, ri == kb.cursor_row and ci == kb.cursor_col, kb)
                x += int(key.width * self.unit) + self.pad
            y += self.unit + self.pad
        if kb.accent_popup is not None:
            self._draw_accent_popup(kb)
        self._draw_modifier_pills(kb)

    def _draw_key(self, key, x, y, is_cur, kb):
        w, h = int(key.width * self.unit), self.unit
        rect = pygame.Rect(x, y, w, h)
        t = self.theme
        flashed = kb.is_flashed(key)
        if is_cur or flashed:
            bg, border = t.highlight_bg, t.highlight_border
        elif key.is_modifier and self._mod_active(key, kb):
            bg, border = t.modifier_active_bg, t.highlight_border
        elif key.is_modifier:
            bg, border = t.modifier_bg, t.key_border
        elif key.label.startswith("F") and key.label[1:].isdigit():
            bg, border = t.fn_key_bg, t.key_border
        else:
            bg, border = t.key_bg, t.key_border
        pygame.draw.rect(self.surface, bg, rect, border_radius=6)
        pygame.draw.rect(self.surface, border, rect, width=1, border_radius=6)
        label = kb.get_display_label(key)
        tc = (255, 255, 255) if (is_cur or flashed) else t.key_text
        ts = self.font.render(label, True, tc)
        self.surface.blit(ts, ts.get_rect(center=rect.center))
        # Shift hint (top-right)
        if key.shift_label and not key.is_modifier and not is_cur:
            if not (kb.shift_active ^ kb.caps_active):
                hs = self.font_small.render(key.shift_label, True, t.modifier_text)
                self.surface.blit(hs, hs.get_rect(topright=(rect.right - 4, rect.top + 2)))
        # Controller glyph (bottom-right corner, small)
        glyph = KEY_GLYPHS.get(key.code)
        if glyph:
            gs = self.font_glyph.render(glyph, True, t.glyph_color)
            self.surface.blit(gs, gs.get_rect(bottomright=(rect.right - 3, rect.bottom - 2)))

    @staticmethod
    def _mod_active(key, kb):
        return {"shift": kb.shift_active, "caps": kb.caps_active, "ctrl": kb.ctrl_active,
                "alt": kb.alt_active, "meta": kb.meta_active}.get(key.modifier_type, False)

    def _draw_accent_popup(self, kb):
        accents, sel = kb.accent_popup
        row = kb.layout[kb.cursor_row]
        xo = self.pad
        for i in range(kb.cursor_col):
            xo += int(row[i].width * self.unit) + self.pad
        ky = self.pad + kb.cursor_row * (self.unit + self.pad)
        py = ky - self.unit - self.pad
        tw = len(accents) * (self.unit + self.pad) + self.pad
        pr = pygame.Rect(xo, py, tw, self.unit + self.pad * 2)
        pygame.draw.rect(self.surface, self.theme.accent_popup_bg, pr, border_radius=8)
        pygame.draw.rect(self.surface, self.theme.highlight_border, pr, width=2, border_radius=8)
        ax = xo + self.pad
        for i, (label, _cp) in enumerate(accents):
            ar = pygame.Rect(ax, py + self.pad, self.unit, self.unit)
            if i == sel:
                pygame.draw.rect(self.surface, self.theme.accent_highlight_bg, ar, border_radius=6)
            ts = self.font.render(label, True, self.theme.accent_popup_text)
            self.surface.blit(ts, ts.get_rect(center=ar.center))
            ax += self.unit + self.pad

    def _draw_modifier_pills(self, kb):
        labels = []
        if kb.shift_active:
            labels.append("SHIFT")
        if kb.caps_active:
            labels.append("CAPS")
        if kb.ctrl_active:
            labels.append("CTRL")
        if kb.alt_active:
            labels.append("ALT")
        if kb.meta_active:
            labels.append("SUPER")
        if not labels:
            return
        # Stack vertically in top-right corner
        x = self.surface.get_width() - self.pad
        y = self.pad
        for label in labels:
            ts = self.font_small.render(label, True, (255, 255, 255))
            pill = ts.get_rect(topright=(x, y)).inflate(10, 4)
            pygame.draw.rect(self.surface, self.theme.modifier_active_bg, pill, border_radius=4)
            ts2 = self.font_small.render(label, True, (255, 255, 255))
            self.surface.blit(ts2, ts2.get_rect(center=pill.center))
            y = pill.bottom + 4


# ---------------------------------------------------------------------------
# Sizing
# ---------------------------------------------------------------------------

def get_primary_monitor():
    """Get primary monitor geometry (x, y, w, h) via xrandr. Falls back to full screen."""
    import subprocess
    try:
        out = subprocess.check_output(
            ["xrandr", "--query"], stderr=subprocess.DEVNULL, timeout=2
        ).decode()
        for line in out.splitlines():
            if "primary" in line and " connected " in line:
                # Parse: "HDMI-A-0 connected primary 2560x1440+1920+0 ..."
                for part in line.split():
                    if "+" in part and "x" in part.split("+")[0]:
                        geo = part  # e.g. "2560x1440+1920+0"
                        res, ox, oy = geo.split("+")[0], int(geo.split("+")[1]), int(geo.split("+")[2])
                        w, h = int(res.split("x")[0]), int(res.split("x")[1])
                        return ox, oy, w, h
    except Exception:
        pass
    return None


def calc_unit_size(layout, screen_width, config):
    if config.unit_size > 0:
        return config.unit_size
    max_units, max_keys = 0, 0
    for row in layout:
        ru = sum(k.width for k in row)
        if ru > max_units:
            max_units = ru
            max_keys = len(row)
    target = int(screen_width * 0.70)
    return int(max(30, (target - (max_keys + 1) * config.padding) / max_units))


def calc_window_size(layout, unit, pad):
    mw = 0
    for row in layout:
        rw = pad
        for key in row:
            rw += int(key.width * unit) + pad
        mw = max(mw, rw)
    return mw, pad + len(layout) * (unit + pad)


# ---------------------------------------------------------------------------
# Main Application
# ---------------------------------------------------------------------------

class Application:
    def __init__(self, config, device_path=None):
        self.config = config
        self.device_path = device_path
        self.running = False
        self.visible = True
        self._toggle_pending = False
        self._lock = threading.Lock()

    def run(self):
        layout = LAYOUT_QWERTY
        theme = THEMES.get(self.config.theme_name, THEMES["dark"])

        # Init display, compute sizes, create properly sized window
        os.environ.setdefault("SDL_VIDEO_ALLOW_SCREENSAVER", "1")
        os.environ["SDL_AUDIODRIVER"] = "dummy"
        pygame.display.init()
        pygame.font.init()

        # Get primary monitor geometry (multi-monitor aware)
        monitor = get_primary_monitor()
        if monitor:
            mon_x, mon_y, mon_w, mon_h = monitor
        else:
            info = pygame.display.Info()
            mon_x, mon_y, mon_w, mon_h = 0, 0, info.current_w, info.current_h

        unit = calc_unit_size(layout, mon_w, self.config)
        pad = self.config.padding
        width, height = calc_window_size(layout, unit, pad)

        # Position: bottom center of primary monitor
        x = mon_x + (mon_w - width) // 2
        y = mon_y + mon_h - height - self.config.bottom_margin
        os.environ["SDL_VIDEO_WINDOW_POS"] = f"{x},{y}"
        surface = pygame.display.set_mode((width, height), pygame.NOFRAME)
        pygame.display.set_caption("gamepad-osk")
        _sdl_set_window_pos(x, y)
        _apply_x11_hints()

        clock = pygame.time.Clock()
        kb = KeyboardState(layout)
        renderer = Renderer(surface, theme, unit, pad)

        try:
            injector = KeyInjector()
        except Exception as e:
            print(f"Error: cannot create UInput device: {e}", file=sys.stderr)
            print("Ensure you're in the 'input' group: sudo usermod -aG input $USER",
                  file=sys.stderr)
            pygame.quit()
            return

        gamepad = GamepadReader(self.config)
        if not gamepad.open_device(self.device_path):
            print("Error: no gamepad found", file=sys.stderr)
            print("Usage: gamepad-osk /dev/input/gamepad0", file=sys.stderr)
            injector.close()
            pygame.quit()
            return

        # Non-blocking evdev reads
        flags = fcntl.fcntl(gamepad.fileno(), fcntl.F_GETFL)
        fcntl.fcntl(gamepad.fileno(), fcntl.F_SETFL, flags | os.O_NONBLOCK)

        if self.config.grab:
            gamepad.grab()

        ipc = IPCServer(self._on_ipc)
        ipc.start()

        self.running = True
        try:
            while self.running:
                # IPC toggle
                with self._lock:
                    if self._toggle_pending:
                        self._toggle_pending = False
                        self.visible = not self.visible
                        if self.visible:
                            if self.config.grab:
                                gamepad.grab()
                        else:
                            if self.config.grab:
                                gamepad.ungrab()
                            pygame.display.iconify()

                for event in pygame.event.get():
                    if event.type == pygame.QUIT:
                        self.running = False

                # Gamepad input
                gp_fd = gamepad.fileno()
                if gp_fd >= 0:
                    select.select([gp_fd], [], [], 0)
                    for action in gamepad.process_events():
                        self._handle(action, kb, injector, gamepad)

                kb.check_long_press(self.config.long_press_ms)

                if self.visible:
                    renderer.draw(kb)
                    pygame.display.flip()

                clock.tick(60)
        finally:
            ipc.stop()
            gamepad.close()
            injector.close()
            pygame.quit()

    def _handle(self, a, kb, inj, gp):
        if a.type == "navigate":
            kb.navigate(a.dx, a.dy)
        elif a.type == "press":
            # A released: if accent popup is open, select from it.
            # If long-press was active but popup never opened, it was a quick tap — type normally.
            if kb.accent_popup is not None:
                kb.press_current(inj)
            else:
                kb.press_current(inj)
            kb.cancel_long_press()
        elif a.type == "press_start":
            kb.start_long_press()
        elif a.type == "backspace":
            inj.press_key(ecodes.KEY_BACKSPACE)
            kb.flash_key(ecodes.KEY_BACKSPACE)
        elif a.type == "space":
            inj.press_key(ecodes.KEY_SPACE)
            kb.flash_key(ecodes.KEY_SPACE)
        elif a.type == "enter":
            inj.press_key(ecodes.KEY_ENTER)
            kb.flash_key(ecodes.KEY_ENTER)
        elif a.type == "shift_on":
            kb.shift_active = True
        elif a.type == "shift_off":
            kb.shift_active = False
        elif a.type == "caps_toggle":
            kb.caps_active = not kb.caps_active
        elif a.type == "close":
            self.running = False
        elif a.type == "mouse_move":
            inj.move_mouse(a.dx, a.dy)

    def _on_ipc(self, cmd):
        if cmd == "toggle":
            with self._lock:
                self._toggle_pending = True


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

def main():
    args = sys.argv[1:]

    if "--toggle" in args:
        if ipc_send("toggle"):
            sys.exit(0)
        print("No running instance, starting new...")
        args.remove("--toggle")

    if "--help" in args or "-h" in args:
        print(__doc__.strip())
        sys.exit(0)

    device_path = None
    for arg in args:
        if not arg.startswith("-"):
            device_path = arg
            break

    config = load_config()
    app = Application(config, device_path)
    try:
        app.run()
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
