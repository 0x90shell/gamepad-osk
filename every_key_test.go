package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// usKeys is every printable key of the layout with its Linux key code
// (input-event-codes.h) and its US shifted character. It is written out here,
// not taken from layout.go, so a wrong code or label there shows up.
var usKeys = map[string]struct {
	code    uint16
	shifted string
}{
	"`": {41, "~"}, "1": {2, "!"}, "2": {3, "@"}, "3": {4, "#"}, "4": {5, "$"}, "5": {6, "%"},
	"6": {7, "^"}, "7": {8, "&"}, "8": {9, "*"}, "9": {10, "("}, "0": {11, ")"}, "-": {12, "_"},
	"=": {13, "+"},
	"q": {16, "Q"}, "w": {17, "W"}, "e": {18, "E"}, "r": {19, "R"}, "t": {20, "T"}, "y": {21, "Y"},
	"u": {22, "U"}, "i": {23, "I"}, "o": {24, "O"}, "p": {25, "P"}, "[": {26, "{"}, "]": {27, "}"},
	"\\": {43, "|"},

	"a": {30, "A"}, "s": {31, "S"}, "d": {32, "D"}, "f": {33, "F"}, "g": {34, "G"}, "h": {35, "H"},
	"j": {36, "J"}, "k": {37, "K"}, "l": {38, "L"}, ";": {39, ":"}, "'": {40, "\""},
	"z": {44, "Z"}, "x": {45, "X"}, "c": {46, "C"}, "v": {47, "V"}, "b": {48, "B"}, "n": {49, "N"},
	"m": {50, "M"}, ",": {51, "<"}, ".": {52, ">"}, "/": {53, "?"},
	"Space": {57, ""},
}

// usAccents is every accent popup, in popup order (codepoints).
var usAccents = map[string][]rune{
	"e": {'è', 'é', 'ê', 'ë'}, "u": {'ù', 'ú', 'û', 'ü'}, "i": {'ì', 'í', 'î', 'ï'},
	"o": {'ò', 'ó', 'ô', 'ö', 'õ'}, "a": {'à', 'á', 'â', 'ä', 'å', 'ã'}, "s": {'ß'}, "c": {'ç'}, "n": {'ñ'},
}

const (
	codeShift = 42 // KEY_LEFTSHIFT
	codeCtrl  = 29 // KEY_LEFTCTRL
	codeU     = 22
	codeEnter = 28
)

// chords turns key events into "Shift+16" style key-downs, with the
// modifiers held at that moment.
func chords(evs []keyEvent) []string {
	held := map[uint16]bool{}
	var out []string
	for _, e := range evs {
		switch e.code {
		case codeShift, codeCtrl:
			held[e.code] = e.value != 0
			continue
		}
		if e.value != 1 {
			continue
		}
		var mods []string
		if held[codeCtrl] {
			mods = append(mods, "Ctrl")
		}
		if held[codeShift] {
			mods = append(mods, "Shift")
		}
		out = append(out, strings.Join(append(mods, num(e.code)), "+"))
	}
	return out
}

func num(code uint16) string { return strconv.FormatUint(uint64(code), 10) }

func isLetterLabel(l string) bool { return len(l) == 1 && l[0] >= 'a' && l[0] <= 'z' }

// U70: every printable key sends its own code, with Shift exactly when it
// should: plain, Shift (LT held), Caps (letters only) and Caps + Shift.
func TestEveryKeyTypes(t *testing.T) {
	old := keyHoldDuration
	keyHoldDuration = 0
	t.Cleanup(func() { keyHoldDuration = old })
	l := newTestLoop(t)
	kb := l.app.kb
	seen := map[string]bool{}
	for r, row := range kb.Layout {
		for c, key := range row {
			want, ok := usKeys[key.Label]
			if !ok {
				continue
			}
			seen[key.Label] = true
			if uint16(key.Code) != want.code { //nolint:gosec // G115: key codes are small
				t.Errorf("%q: code %d, want %d", key.Label, key.Code, want.code)
				continue
			}
			if want.shifted != "" && key.ShiftLabel != want.shifted {
				t.Errorf("%q: shift label %q, want %q", key.Label, key.ShiftLabel, want.shifted)
			}
			for _, st := range []struct {
				name        string
				shift, caps bool
			}{{"plain", false, false}, {"Shift", true, false}, {"Caps", false, true}, {"Caps+Shift", true, true}} {
				kb.CursorRow, kb.CursorCol = r, c
				kb.ShiftActive, kb.ShiftHeld, kb.CapsActive = st.shift, st.shift, st.caps
				kb.PressCurrent(l.app.inj)
				shifted := st.shift
				if st.caps && isLetterLabel(key.Label) {
					shifted = !shifted // Caps changes letters only
				}
				exp := num(want.code)
				if shifted {
					exp = "Shift+" + exp
				}
				if got := chords(l.keys()); !slices.Equal(got, []string{exp}) {
					t.Errorf("%q %s: sent %v, want [%s]", key.Label, st.name, got, exp)
				}
			}
		}
	}
	kb.ShiftActive, kb.ShiftHeld, kb.CapsActive = false, false, false
	for label := range usKeys {
		if !seen[label] {
			t.Errorf("no key %q in the layout", label)
		}
	}
}

// U71: every accent in every popup is sent as Ctrl+Shift+U, its four hex
// digits and Enter, with nothing held over the digits.
func TestEveryAccentTypes(t *testing.T) {
	old := keyHoldDuration
	keyHoldDuration = 0
	t.Cleanup(func() { keyHoldDuration = old })
	hex := map[rune]uint16{'0': 11, '1': 2, '2': 3, '3': 4, '4': 5, '5': 6, '6': 7, '7': 8, '8': 9, '9': 10,
		'a': 30, 'b': 48, 'c': 46, 'd': 32, 'e': 18, 'f': 33}
	l := newTestLoop(t)
	kb := l.app.kb
	seen := map[string]bool{}
	for r, row := range kb.Layout {
		for c, key := range row {
			want, ok := usAccents[key.Label]
			if !ok {
				if len(key.Accents) > 0 {
					t.Errorf("%q has an accent popup this test does not know", key.Label)
				}
				continue
			}
			seen[key.Label] = true
			if len(key.Accents) != len(want) {
				t.Errorf("%q: %d accents, want %d", key.Label, len(key.Accents), len(want))
				continue
			}
			for j, cp := range want {
				kb.CursorRow, kb.CursorCol = r, c
				kb.AccentPopup = &AccentPopupState{Accents: key.Accents, Selected: j}
				kb.PressCurrent(l.app.inj)
				exp := []string{fmt.Sprintf("Ctrl+Shift+%d", codeU)}
				for _, h := range fmt.Sprintf("%04x", cp) {
					exp = append(exp, num(hex[h]))
				}
				exp = append(exp, num(codeEnter))
				if got := chords(l.keys()); !slices.Equal(got, exp) {
					t.Errorf("%q accent %d (%c): sent %v, want %v", key.Label, j, cp, got, exp)
				}
				if kb.AccentPopup != nil {
					t.Fatalf("%q: the popup stayed open after the accent", key.Label)
				}
			}
		}
	}
	for label := range usAccents {
		if !seen[label] {
			t.Errorf("no accent popup on %q", label)
		}
	}
}
