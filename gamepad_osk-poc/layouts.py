"""Keyboard layout definitions using evdev keycodes."""

from dataclasses import dataclass, field
from evdev import ecodes


@dataclass
class KeyDef:
    label: str
    code: int
    width: float = 1.0
    shift_label: str = ""
    is_modifier: bool = False
    modifier_type: str = ""  # "shift", "caps", "ctrl", "alt", "meta"
    accents: list = field(default_factory=list)


def _k(label, code, width=1.0, shift_label="", **kwargs):
    """Shorthand key constructor."""
    return KeyDef(label=label, code=code, width=width,
                  shift_label=shift_label, **kwargs)


def _mod(label, code, width=1.0, modifier_type=""):
    """Shorthand modifier constructor."""
    return KeyDef(label=label, code=code, width=width,
                  is_modifier=True, modifier_type=modifier_type)


# Accent maps for long-press (label, unicode codepoint)
# We'll inject these via Ctrl+Shift+U on GTK/Qt
ACCENT_A = [("\u00e0", 0xe0), ("\u00e1", 0xe1), ("\u00e2", 0xe2),
            ("\u00e4", 0xe4), ("\u00e5", 0xe5), ("\u00e3", 0xe3)]
ACCENT_E = [("\u00e8", 0xe8), ("\u00e9", 0xe9), ("\u00ea", 0xea),
            ("\u00eb", 0xeb)]
ACCENT_I = [("\u00ec", 0xec), ("\u00ed", 0xed), ("\u00ee", 0xee),
            ("\u00ef", 0xef)]
ACCENT_O = [("\u00f2", 0xf2), ("\u00f3", 0xf3), ("\u00f4", 0xf4),
            ("\u00f6", 0xf6), ("\u00f5", 0xf5)]
ACCENT_U = [("\u00f9", 0xf9), ("\u00fa", 0xfa), ("\u00fb", 0xfb),
            ("\u00fc", 0xfc)]
ACCENT_N = [("\u00f1", 0xf1)]
ACCENT_C = [("\u00e7", 0xe7)]
ACCENT_S = [("\u00df", 0xdf)]


LAYOUT_QWERTY = [
    # Row 0: Escape + Function keys
    [
        _k("Esc", ecodes.KEY_ESC, 1.5),
        _k("F1", ecodes.KEY_F1),
        _k("F2", ecodes.KEY_F2),
        _k("F3", ecodes.KEY_F3),
        _k("F4", ecodes.KEY_F4),
        _k("F5", ecodes.KEY_F5),
        _k("F6", ecodes.KEY_F6),
        _k("F7", ecodes.KEY_F7),
        _k("F8", ecodes.KEY_F8),
        _k("F9", ecodes.KEY_F9),
        _k("F10", ecodes.KEY_F10),
        _k("F11", ecodes.KEY_F11),
        _k("F12", ecodes.KEY_F12),
        _k("Del", ecodes.KEY_DELETE, 1.5),
    ],
    # Row 1: Number row
    [
        _k("`", ecodes.KEY_GRAVE, shift_label="~"),
        _k("1", ecodes.KEY_1, shift_label="!"),
        _k("2", ecodes.KEY_2, shift_label="@"),
        _k("3", ecodes.KEY_3, shift_label="#"),
        _k("4", ecodes.KEY_4, shift_label="$"),
        _k("5", ecodes.KEY_5, shift_label="%"),
        _k("6", ecodes.KEY_6, shift_label="^"),
        _k("7", ecodes.KEY_7, shift_label="&"),
        _k("8", ecodes.KEY_8, shift_label="*"),
        _k("9", ecodes.KEY_9, shift_label="("),
        _k("0", ecodes.KEY_0, shift_label=")"),
        _k("-", ecodes.KEY_MINUS, shift_label="_"),
        _k("=", ecodes.KEY_EQUAL, shift_label="+"),
        _k("Bksp", ecodes.KEY_BACKSPACE, 2.0),
    ],
    # Row 2: QWERTY
    [
        _k("Tab", ecodes.KEY_TAB, 1.5),
        _k("q", ecodes.KEY_Q, shift_label="Q"),
        _k("w", ecodes.KEY_W, shift_label="W"),
        _k("e", ecodes.KEY_E, shift_label="E", accents=ACCENT_E),
        _k("r", ecodes.KEY_R, shift_label="R"),
        _k("t", ecodes.KEY_T, shift_label="T"),
        _k("y", ecodes.KEY_Y, shift_label="Y"),
        _k("u", ecodes.KEY_U, shift_label="U", accents=ACCENT_U),
        _k("i", ecodes.KEY_I, shift_label="I", accents=ACCENT_I),
        _k("o", ecodes.KEY_O, shift_label="O", accents=ACCENT_O),
        _k("p", ecodes.KEY_P, shift_label="P"),
        _k("[", ecodes.KEY_LEFTBRACE, shift_label="{"),
        _k("]", ecodes.KEY_RIGHTBRACE, shift_label="}"),
        _k("\\", ecodes.KEY_BACKSLASH, 1.5, shift_label="|"),
    ],
    # Row 3: Home row
    [
        _mod("Caps", ecodes.KEY_CAPSLOCK, 1.75, modifier_type="caps"),
        _k("a", ecodes.KEY_A, shift_label="A", accents=ACCENT_A),
        _k("s", ecodes.KEY_S, shift_label="S", accents=ACCENT_S),
        _k("d", ecodes.KEY_D, shift_label="D"),
        _k("f", ecodes.KEY_F, shift_label="F"),
        _k("g", ecodes.KEY_G, shift_label="G"),
        _k("h", ecodes.KEY_H, shift_label="H"),
        _k("j", ecodes.KEY_J, shift_label="J"),
        _k("k", ecodes.KEY_K, shift_label="K"),
        _k("l", ecodes.KEY_L, shift_label="L"),
        _k(";", ecodes.KEY_SEMICOLON, shift_label=":"),
        _k("'", ecodes.KEY_APOSTROPHE, shift_label='"'),
        _k("Enter", ecodes.KEY_ENTER, 2.25),
    ],
    # Row 4: Bottom row
    [
        _mod("Shift", ecodes.KEY_LEFTSHIFT, 2.25, modifier_type="shift"),
        _k("z", ecodes.KEY_Z, shift_label="Z"),
        _k("x", ecodes.KEY_X, shift_label="X"),
        _k("c", ecodes.KEY_C, shift_label="C", accents=ACCENT_C),
        _k("v", ecodes.KEY_V, shift_label="V"),
        _k("b", ecodes.KEY_B, shift_label="B"),
        _k("n", ecodes.KEY_N, shift_label="N", accents=ACCENT_N),
        _k("m", ecodes.KEY_M, shift_label="M"),
        _k(",", ecodes.KEY_COMMA, shift_label="<"),
        _k(".", ecodes.KEY_DOT, shift_label=">"),
        _k("/", ecodes.KEY_SLASH, shift_label="?"),
        _mod("Shift", ecodes.KEY_RIGHTSHIFT, 2.75, modifier_type="shift"),
    ],
    # Row 5: Space bar row
    [
        _mod("Ctrl", ecodes.KEY_LEFTCTRL, 1.5, modifier_type="ctrl"),
        _mod("Super", ecodes.KEY_LEFTMETA, 1.25, modifier_type="meta"),
        _mod("Alt", ecodes.KEY_LEFTALT, 1.25, modifier_type="alt"),
        _k("Space", ecodes.KEY_SPACE, 6.0),
        _mod("Alt", ecodes.KEY_RIGHTALT, 1.25, modifier_type="alt"),
        _mod("Super", ecodes.KEY_RIGHTMETA, 1.25, modifier_type="meta"),
        _k("\u2190", ecodes.KEY_LEFT),
        _k("\u2191", ecodes.KEY_UP),
        _k("\u2193", ecodes.KEY_DOWN),
        _k("\u2192", ecodes.KEY_RIGHT),
    ],
]


def get_layout():
    return LAYOUT_QWERTY
