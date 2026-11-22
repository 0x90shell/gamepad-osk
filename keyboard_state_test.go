package main

import (
	"strings"
	"testing"
	"time"
)

// cursorOn puts the cursor on the first key with this label.
func cursorOn(t *testing.T, kb *KeyboardState, label string) {
	t.Helper()
	for r, row := range kb.Layout {
		for c, key := range row {
			if key.Label == label {
				kb.CursorRow, kb.CursorCol = r, c
				return
			}
		}
	}
	t.Fatalf("no key %q in the layout", label)
}

// U35: the Alt+Tab auto release happens at 3 s of idle, not before. The
// field is public, so the test moves the last Tab back in time.
func TestCheckAltTabTimeoutEdges(t *testing.T) {
	cases := []struct {
		idle time.Duration
		want bool
	}{
		{0, false},
		{2500 * time.Millisecond, false},
		{3500 * time.Millisecond, true},
		{30 * time.Second, true},
	}
	for _, c := range cases {
		kb := NewKeyboardState(LayoutQWERTY)
		kb.AltTabHeld = true
		kb.AltTabLastTab = time.Now().Add(-c.idle)
		got := kb.CheckAltTabTimeout(nil)
		if got != c.want || kb.AltTabHeld == c.want {
			t.Errorf("idle %v: released=%v held=%v, want released=%v", c.idle, got, kb.AltTabHeld, c.want)
		}
	}
	// Not held: nothing to release, however old the last Tab.
	kb := NewKeyboardState(LayoutQWERTY)
	kb.AltTabLastTab = time.Now().Add(-time.Hour)
	if kb.CheckAltTabTimeout(nil) {
		t.Error("released Alt although it was not held")
	}
}

// U36: a Tab press stamps the time, so a second Tab 2 s later keeps Alt held
// and the idle time counts from the second one.
func TestAltTabPressRestartsTimeout(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	cursorOn(t, kb, "AltTab")
	kb.PressCurrent(nil)
	if !kb.AltTabHeld {
		t.Fatal("AltTab did not hold Alt")
	}
	kb.AltTabLastTab = time.Now().Add(-2500 * time.Millisecond)
	kb.PressCurrent(nil)
	if time.Since(kb.AltTabLastTab) > time.Second {
		t.Error("a second AltTab press did not restart the idle time")
	}
	if kb.CheckAltTabTimeout(nil) {
		t.Error("Alt released 0 s after the latest Tab")
	}
}

// U37: the long press opens the popup after the threshold only, only on a key
// with accents, and only once.
func TestCheckLongPressEdges(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	cursorOn(t, kb, "a")
	kb.StartLongPress()
	if !kb.LongPressActive {
		t.Fatal("StartLongPress on a did not start the timer")
	}
	if kb.CheckLongPress(500) || kb.AccentPopup != nil {
		t.Error("popup opened at 0 ms with a 500 ms threshold")
	}
	kb.LongPressStart = time.Now().Add(-400 * time.Millisecond)
	if kb.CheckLongPress(500) {
		t.Error("popup opened at 400 ms with a 500 ms threshold")
	}
	kb.LongPressStart = time.Now().Add(-600 * time.Millisecond)
	if !kb.CheckLongPress(500) || kb.AccentPopup == nil || kb.LongPressActive {
		t.Errorf("no popup at 600 ms: popup %v, active %v", kb.AccentPopup, kb.LongPressActive)
	}
	if kb.CheckLongPress(500) {
		t.Error("CheckLongPress reported a second open for one press")
	}
	if kb.AccentPopup == nil {
		t.Fatal("no popup to check")
	}
	if len(kb.AccentPopup.Accents) != len(accentA) || kb.AccentPopup.Selected != 0 {
		t.Errorf("popup holds %d accents, selected %d; want %d and 0", len(kb.AccentPopup.Accents), kb.AccentPopup.Selected, len(accentA))
	}

	// A key without accents never starts the timer.
	kb2 := NewKeyboardState(LayoutQWERTY)
	cursorOn(t, kb2, "q")
	kb2.StartLongPress()
	if kb2.LongPressActive {
		t.Error("StartLongPress on q (no accents) started the timer")
	}
	// A cancelled press never opens.
	kb.CloseAccentPopup()
	cursorOn(t, kb, "a")
	kb.StartLongPress()
	kb.LongPressStart = time.Now().Add(-time.Second)
	kb.CancelLongPress()
	if kb.CheckLongPress(500) {
		t.Error("popup opened after CancelLongPress")
	}
}

// U38: the long press checks the key under the cursor when the time is up. If the
// cursor moved to a key without accents in the meantime, nothing opens.
func TestCheckLongPressCursorMovedOff(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	cursorOn(t, kb, "a")
	kb.StartLongPress()
	kb.LongPressStart = time.Now().Add(-time.Second)
	cursorOn(t, kb, "q")
	if kb.CheckLongPress(500) || kb.AccentPopup != nil {
		t.Error("popup opened for a key without accents")
	}
}

// U39: any key press but a modifier uses up the armed Ctrl, Alt and Super,
// including the app keys (Paste, Theme, Alt+Tab, Shift+Up for mouse speed).
// Owner's rule, 2026-11-21: an armed modifier never carries over to a later key.
func TestNonTypingKeysClearArmedModifiers(t *testing.T) {
	for _, c := range []struct {
		label string
		shift bool
	}{
		{"Paste", false}, {"Paste", true}, {themeKeyLabel, false}, {themeKeyLabel, true},
		{"AltTab", false}, {"↑", true}, {"↓", true}, {"q", false}, {"Undo", false}, {"Undo", true},
	} {
		kb := NewKeyboardState(LayoutQWERTY)
		kb.CtrlActive, kb.AltActive, kb.MetaActive, kb.ShiftActive = true, true, true, c.shift
		cursorOn(t, kb, c.label)
		kb.PressCurrent(nil)
		kb.ReleaseAltTab(nil)
		if kb.CtrlActive || kb.AltActive || kb.MetaActive {
			t.Errorf("%s (shift %v): still armed ctrl=%v alt=%v super=%v", c.label, c.shift, kb.CtrlActive, kb.AltActive, kb.MetaActive)
		}
	}
	// A modifier key adds to them instead: Ctrl then Alt are both armed.
	kb := NewKeyboardState(LayoutQWERTY)
	cursorOn(t, kb, "Ctrl")
	kb.PressCurrent(nil)
	cursorOn(t, kb, "Alt")
	kb.PressCurrent(nil)
	if !kb.CtrlActive || !kb.AltActive {
		t.Errorf("Ctrl then Alt: ctrl=%v alt=%v, want both armed", kb.CtrlActive, kb.AltActive)
	}
}

// U43: Shift reaches Left and Right (text selection, Ctrl+Shift+Left for a
// word), but not from Caps alone, and never Esc or Delete.
func TestShiftOnArrowsEscDelete(t *testing.T) {
	old := keyHoldDuration
	keyHoldDuration = 0
	t.Cleanup(func() { keyHoldDuration = old })
	names := keyNames()
	for _, c := range []struct {
		state       int
		label, want string
	}{
		{1, "←", "+Shift +← -← -Shift"},                // shift
		{2, "→", "+Shift +→ -→ -Shift"},                // shift held
		{9, "←", "+Shift +Ctrl +← -← -Shift -Ctrl"},    // ctrl+shift: select a word
		{3, "←", "+← -←"},                              // caps alone moves
		{1, "Del", "+Del -Del"},                        // shift
		{11, "Del", "+Ctrl +Alt +Del -Del -Ctrl -Alt"}, // ctrl+alt+shift
		{1, "Esc", "+Esc -Esc"},                        // shift
	} {
		row := comboRow(t, names, c.state, c.label)
		got := strings.TrimSpace(strings.SplitN(strings.SplitN(row, " | ", 2)[0], c.label, 2)[1])
		if got != c.want {
			t.Errorf("%s in state %s: sent %q, want %q", c.label, comboStates[c.state].name, got, c.want)
		}
	}
}

// U40: hiding the keyboard with the accent popup open, a long press pending and
// SkipPress set leaves a clean state for the next show (fixed 2026-11-21: before,
// the popup survived to the next show and the next A typed the accent).
func TestHideClearsAccentPopupAndLongPress(t *testing.T) {
	l := newTestLoop(t)
	app, kb := l.app, l.app.kb
	cursorOn(t, kb, "a")
	kb.AccentPopup = &AccentPopupState{Accents: accentA, Selected: 2}
	kb.LongPressActive = true
	kb.SkipPress = true
	app.togglePending = true
	app.applyToggle() // hide
	if app.visible {
		t.Fatal("did not hide")
	}
	if kb.AccentPopup != nil || kb.LongPressActive || kb.SkipPress {
		t.Errorf("after hide: popup %v, long press %v, skip %v; want all clear", kb.AccentPopup, kb.LongPressActive, kb.SkipPress)
	}
}

// U41: a pending long press must not turn into a popup after a hide.
func TestHidePendingLongPressDoesNotOpenPopup(t *testing.T) {
	l := newTestLoop(t)
	app, kb := l.app, l.app.kb
	cursorOn(t, kb, "a")
	kb.StartLongPress()
	app.togglePending = true
	app.applyToggle()                                // hide
	kb.LongPressStart = time.Now().Add(-time.Second) // LongPressStart uses the real clock, not app.now
	app.checkTimers()
	if kb.AccentPopup != nil {
		t.Error("the popup opened while the keyboard was hidden")
	}
}

// U48: no long press fires while hidden, even one left active.
func TestNoLongPressWhileHidden(t *testing.T) {
	l := newTestLoop(t)
	app, kb := l.app, l.app.kb
	cursorOn(t, kb, "a")
	app.visible = false
	kb.LongPressActive = true
	kb.LongPressStart = time.Now().Add(-time.Second)
	app.checkTimers()
	if kb.AccentPopup != nil {
		t.Error("the popup opened while the keyboard was hidden")
	}
}

