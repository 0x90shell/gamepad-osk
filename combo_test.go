package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

var updateCombos = flag.Bool("update-combos", false, "rewrite testdata/combos.golden from the current code")

// comboStates are the armed and held modifier states the matrix presses each key in.
var comboStates = []struct {
	name                               string
	shift, held, caps, ctrl, alt, meta bool
}{
	{name: "none"},
	{name: "shift", shift: true},
	{name: "shift-held", shift: true, held: true},
	{name: "caps", caps: true},
	{name: "caps+shift", caps: true, shift: true},
	{name: "ctrl", ctrl: true},
	{name: "alt", alt: true},
	{name: "super", meta: true},
	{name: "ctrl+alt", ctrl: true, alt: true},
	{name: "ctrl+shift", ctrl: true, shift: true},
	{name: "alt+shift", alt: true, shift: true},
	{name: "ctrl+alt+shift", ctrl: true, alt: true, shift: true},
	{name: "ctrl+shift-held", ctrl: true, shift: true, held: true},
	{name: "super+shift", meta: true, shift: true},
}

// comboKeys are the keys with their own branch in PressCurrent, plus one of each
// plain kind (letter, digit, symbol, Tab, Enter, Space, Backspace).
var comboKeys = []string{
	"q", "e", "1", "[", "Tab", "Enter", "Space", "Bksp",
	"Esc", "Del", "←", "→", "↑", "↓",
	"Paste", themeKeyLabel, "AltTab",
	"Undo", "Redo", "Cut", "SelAll", "Home", "End", "PgUp", "PgDn", "Play", "Mute",
}

func keyNames() map[uint16]string {
	names := map[uint16]string{
		KEY_LEFTSHIFT: "Shift", KEY_LEFTCTRL: "Ctrl", KEY_LEFTALT: "Alt", KEY_LEFTMETA: "Super",
		KEY_V: "v", KEY_C: "c", KEY_Z: "z", KEY_Y: "y", KEY_X: "x", KEY_A: "a", KEY_TAB: "Tab",
		KEY_U: "u",
	}
	for _, row := range LayoutQWERTY {
		for _, k := range row {
			if _, ok := names[uint16(k.Code)]; !ok && k.Code != 0 { //nolint:gosec // G115: key codes are small
				names[uint16(k.Code)] = k.Label //nolint:gosec // G115: key codes are small
			}
			if k.ShiftCode != 0 {
				names[uint16(k.ShiftCode)] = k.ShiftLabel //nolint:gosec // G115: key codes are small
			}
		}
	}
	return names
}

// comboRow presses one key in one state and describes what was sent and what
// stays armed: "+Ctrl +v -v -Ctrl | armed: ctrl".
func comboRow(t *testing.T, names map[uint16]string, st int, label string) string {
	t.Helper()
	s := comboStates[st]
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pr.Close() }()
	inj := &Injector{fd: pw}
	kb := NewKeyboardState(LayoutQWERTY)
	var calls []string
	kb.OnThemeCycle = func() { calls = append(calls, "theme+") }
	kb.OnThemeCycleReverse = func() { calls = append(calls, "theme-") }
	kb.OnSensitivityUp = func() { calls = append(calls, "speed+") }
	kb.OnSensitivityDown = func() { calls = append(calls, "speed-") }
	kb.ShiftActive, kb.ShiftHeld, kb.CapsActive = s.shift, s.held, s.caps
	kb.CtrlActive, kb.AltActive, kb.MetaActive = s.ctrl, s.alt, s.meta
	cursorOn(t, kb, label)
	kb.PressCurrent(inj)
	kb.ReleaseAltTab(inj) // close an Alt+Tab hold so its Alt release is in the row
	_ = pw.Close()
	data, err := io.ReadAll(pr)
	if err != nil {
		t.Fatal(err)
	}
	var sent []string
	for i := 0; i+24 <= len(data); i += 24 {
		if binary.LittleEndian.Uint16(data[i+16:]) != 1 { // EV_KEY only
			continue
		}
		code, val := binary.LittleEndian.Uint16(data[i+18:]), binary.LittleEndian.Uint32(data[i+20:])
		name, ok := names[code]
		if !ok {
			name = strconv.FormatUint(uint64(code), 10)
		}
		sent = append(sent, map[uint32]string{1: "+", 0: "-"}[val]+name)
	}
	sent = append(sent, calls...)
	var armed []string
	for _, a := range []struct {
		on   bool
		name string
	}{{kb.ShiftActive, "shift"}, {kb.CapsActive, "caps"}, {kb.CtrlActive, "ctrl"}, {kb.AltActive, "alt"}, {kb.MetaActive, "super"}} {
		if a.on {
			armed = append(armed, a.name)
		}
	}
	if len(sent) == 0 {
		sent = []string{"(nothing)"}
	}
	if len(armed) == 0 {
		armed = []string{"-"}
	}
	return fmt.Sprintf("%-15s %-6s %s | armed: %s", s.name, label, strings.Join(sent, " "), strings.Join(armed, ","))
}

// U42: every key with its own branch, pressed in every armed state, against a
// table in testdata/combos.golden. A change to what any combo sends, or to what
// stays armed after it, shows up as a diff of that file. Rewrite it on purpose
// with: go test -run TestComboMatrix -update-combos
func TestComboMatrix(t *testing.T) {
	old := keyHoldDuration
	keyHoldDuration = 0
	t.Cleanup(func() { keyHoldDuration = old })
	names := keyNames()
	var b strings.Builder
	for _, label := range comboKeys {
		for st := range comboStates {
			b.WriteString(comboRow(t, names, st, label) + "\n")
		}
	}
	got := b.String()
	const path = "testdata/combos.golden"
	if *updateCombos {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil { //nolint:gosec // G306: test data
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (create it with -update-combos)", err)
	}
	if got != string(want) {
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := range max(len(gl), len(wl)) {
			var g, w string
			if i < len(gl) {
				g = gl[i]
			}
			if i < len(wl) {
				w = wl[i]
			}
			if g != w {
				t.Errorf("row %d\n got: %s\nwant: %s", i+1, g, w)
			}
		}
	}
}
