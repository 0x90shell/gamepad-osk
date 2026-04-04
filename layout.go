package main

// Linux evdev keycodes (from linux/input-event-codes.h)
const (
	KEY_ESC       = 1
	KEY_1         = 2
	KEY_2         = 3
	KEY_3         = 4
	KEY_4         = 5
	KEY_5         = 6
	KEY_6         = 7
	KEY_7         = 8
	KEY_8         = 9
	KEY_9         = 10
	KEY_0         = 11
	KEY_MINUS     = 12
	KEY_EQUAL     = 13
	KEY_BACKSPACE = 14
	KEY_TAB       = 15
	KEY_Q         = 16
	KEY_W         = 17
	KEY_E         = 18
	KEY_R         = 19
	KEY_T         = 20
	KEY_U         = 22
	KEY_I         = 23
	KEY_O         = 24
	KEY_P         = 25
	KEY_LEFTBRACE = 26
	KEY_RIGHTBRACE = 27
	KEY_ENTER     = 28
	KEY_LEFTCTRL  = 29
	KEY_A         = 30
	KEY_S         = 31
	KEY_D         = 32
	KEY_F         = 33
	KEY_G         = 34
	KEY_H         = 35
	KEY_J         = 36
	KEY_K         = 37
	KEY_L         = 38
	KEY_SEMICOLON = 39
	KEY_APOSTROPHE = 40
	KEY_GRAVE     = 41
	KEY_LEFTSHIFT = 42
	KEY_BACKSLASH = 43
	KEY_Z         = 44
	KEY_X         = 45
	KEY_C         = 46
	KEY_V         = 47
	KEY_B         = 48
	KEY_N         = 49
	KEY_M         = 50
	KEY_COMMA     = 51
	KEY_DOT       = 52
	KEY_SLASH     = 53
	KEY_RIGHTSHIFT = 54
	KEY_LEFTALT   = 56
	KEY_SPACE     = 57
	KEY_CAPSLOCK  = 58
	KEY_F1        = 59
	KEY_F2        = 60
	KEY_F3        = 61
	KEY_F4        = 62
	KEY_F5        = 63
	KEY_F6        = 64
	KEY_F7        = 65
	KEY_F8        = 66
	KEY_F9        = 67
	KEY_F10       = 68
	KEY_F11       = 87
	KEY_F12       = 88
	KEY_RIGHTCTRL = 97
	KEY_RIGHTALT  = 100
	KEY_HOME      = 102
	KEY_UP        = 103
	KEY_LEFT      = 105
	KEY_RIGHT     = 106
	KEY_END       = 107
	KEY_DOWN      = 108
	KEY_DELETE    = 111
	KEY_LEFTMETA  = 125
	KEY_RIGHTMETA = 126
	KEY_Y         = 21
	KEY_MUTE      = 113
	KEY_PAGEUP    = 104
	KEY_PAGEDOWN  = 109
	KEY_PLAYPAUSE = 164
)

type AccentDef struct {
	Label     string
	Codepoint int
}

type KeyDef struct {
	Label        string
	Code         int
	Width        float64
	ShiftLabel   string
	IsModifier   bool
	ModifierType string // "shift", "caps", "ctrl", "alt", "meta"
	Accents      []AccentDef
	// Combo: if set, pressing this key sends these codes as modifiers + Code
	// e.g., Combo={KEY_LEFTCTRL} + Code=KEY_Z = Ctrl+Z
	Combo []int
	// ShiftCode: if set and shift active, send this code instead of Code
	ShiftCode int
}

func k(label string, code int, opts ...interface{}) KeyDef {
	kd := KeyDef{Label: label, Code: code, Width: 1.0}
	for i := 0; i < len(opts); i++ {
		switch v := opts[i].(type) {
		case float64:
			kd.Width = v
		case string:
			if kd.ShiftLabel == "" {
				kd.ShiftLabel = v
			}
		case []AccentDef:
			kd.Accents = v
		}
	}
	return kd
}

// combo creates a key that sends modifier+code as default, with shift sending shiftCode
func combo(label string, code int, mods []int, shiftLabel string, shiftCode int, width float64) KeyDef {
	return KeyDef{Label: label, Code: code, Width: width, ShiftLabel: shiftLabel,
		Combo: mods, ShiftCode: shiftCode}
}

func mod(label string, code int, width float64, modType string) KeyDef {
	return KeyDef{Label: label, Code: code, Width: width, IsModifier: true, ModifierType: modType}
}

var (
	accentA = []AccentDef{{"à", 0xe0}, {"á", 0xe1}, {"â", 0xe2}, {"ä", 0xe4}, {"å", 0xe5}, {"ã", 0xe3}}
	accentE = []AccentDef{{"è", 0xe8}, {"é", 0xe9}, {"ê", 0xea}, {"ë", 0xeb}}
	accentI = []AccentDef{{"ì", 0xec}, {"í", 0xed}, {"î", 0xee}, {"ï", 0xef}}
	accentO = []AccentDef{{"ò", 0xf2}, {"ó", 0xf3}, {"ô", 0xf4}, {"ö", 0xf6}, {"õ", 0xf5}}
	accentU = []AccentDef{{"ù", 0xf9}, {"ú", 0xfa}, {"û", 0xfb}, {"ü", 0xfc}}
	accentN = []AccentDef{{"ñ", 0xf1}}
	accentC = []AccentDef{{"ç", 0xe7}}
	accentS = []AccentDef{{"ß", 0xdf}}
)

var LayoutQWERTY = [][]KeyDef{
	// Row 0: Shortcuts (shift shows F-keys)
	{
		k("Esc", KEY_ESC, 1.5),
		k("Paste", KEY_V, 1.0, "Copy"),
		combo("Undo", KEY_Z, []int{KEY_LEFTCTRL}, "F1", KEY_F1, 1.0),
		combo("Redo", KEY_Y, []int{KEY_LEFTCTRL}, "F2", KEY_F2, 1.0),
		combo("Cut", KEY_X, []int{KEY_LEFTCTRL}, "F3", KEY_F3, 1.0),
		combo("SelAll", KEY_A, []int{KEY_LEFTCTRL}, "F4", KEY_F4, 1.0),
		combo("AltTab", KEY_TAB, []int{KEY_LEFTALT}, "F5", KEY_F5, 1.0),
		combo("Home", KEY_HOME, nil, "F6", KEY_F6, 1.0),
		combo("End", KEY_END, nil, "F7", KEY_F7, 1.0),
		combo("PgUp", KEY_PAGEUP, nil, "F8", KEY_F8, 1.0),
		combo("PgDn", KEY_PAGEDOWN, nil, "F9", KEY_F9, 1.0),
		combo("Play", KEY_PLAYPAUSE, nil, "F10", KEY_F10, 1.0),
		combo("Mute", KEY_MUTE, nil, "F11", KEY_F11, 1.0),
		k("Del", KEY_DELETE, 1.5),
	},
	// Row 1: Numbers
	{
		k("`", KEY_GRAVE, "~"), k("1", KEY_1, "!"), k("2", KEY_2, "@"),
		k("3", KEY_3, "#"), k("4", KEY_4, "$"), k("5", KEY_5, "%"),
		k("6", KEY_6, "^"), k("7", KEY_7, "&"), k("8", KEY_8, "*"),
		k("9", KEY_9, "("), k("0", KEY_0, ")"), k("-", KEY_MINUS, "_"),
		k("=", KEY_EQUAL, "+"), k("Bksp", KEY_BACKSPACE, 2.0),
	},
	// Row 2: QWERTY
	{
		k("Tab", KEY_TAB, 1.5),
		k("q", KEY_Q, "Q"), k("w", KEY_W, "W"),
		k("e", KEY_E, "E", accentE), k("r", KEY_R, "R"), k("t", KEY_T, "T"),
		k("y", KEY_Y, "Y"), k("u", KEY_U, "U", accentU),
		k("i", KEY_I, "I", accentI), k("o", KEY_O, "O", accentO),
		k("p", KEY_P, "P"), k("[", KEY_LEFTBRACE, "{"),
		k("]", KEY_RIGHTBRACE, "}"), k("\\", KEY_BACKSLASH, 1.5, "|"),
	},
	// Row 3: Home row
	{
		mod("Caps", KEY_CAPSLOCK, 1.75, "caps"),
		k("a", KEY_A, "A", accentA), k("s", KEY_S, "S", accentS),
		k("d", KEY_D, "D"), k("f", KEY_F, "F"), k("g", KEY_G, "G"),
		k("h", KEY_H, "H"), k("j", KEY_J, "J"), k("k", KEY_K, "K"),
		k("l", KEY_L, "L"), k(";", KEY_SEMICOLON, ":"),
		k("'", KEY_APOSTROPHE, "\""), k("Enter", KEY_ENTER, 2.25),
	},
	// Row 4: Bottom alpha
	{
		mod("Shift", KEY_LEFTSHIFT, 2.25, "shift"),
		k("z", KEY_Z, "Z"), k("x", KEY_X, "X"),
		k("c", KEY_C, "C", accentC), k("v", KEY_V, "V"), k("b", KEY_B, "B"),
		k("n", KEY_N, "N", accentN), k("m", KEY_M, "M"),
		k(",", KEY_COMMA, "<"), k(".", KEY_DOT, ">"), k("/", KEY_SLASH, "?"),
		mod("Shift", KEY_RIGHTSHIFT, 2.75, "shift"),
	},
	// Row 5: Space bar
	{
		mod("Ctrl", KEY_LEFTCTRL, 1.5, "ctrl"),
		mod("Super", KEY_LEFTMETA, 1.25, "meta"),
		mod("Alt", KEY_LEFTALT, 1.25, "alt"),
		k("Space", KEY_SPACE, 6.0),
		mod("Alt", KEY_RIGHTALT, 1.25, "alt"),
		k("←", KEY_LEFT, "↑"), k("→", KEY_RIGHT, "↓"),
		k("Cfg", 0, 1.75),
	},
}

// KeyGlyphs is built at runtime from config via BuildKeyGlyphs()
var KeyGlyphs map[int]string
