package main

import "testing"

func TestNavigate_Horizontal(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	startCol := kb.CursorCol

	kb.Navigate(1, 0)
	if kb.CursorCol != startCol+1 {
		t.Errorf("right: col = %d, want %d", kb.CursorCol, startCol+1)
	}

	kb.Navigate(-1, 0)
	if kb.CursorCol != startCol {
		t.Errorf("left back: col = %d, want %d", kb.CursorCol, startCol)
	}
}

func TestNavigate_HorizontalWrap(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	kb.CursorRow = 1
	kb.CursorCol = 0

	// Wrap left from first column
	kb.Navigate(-1, 0)
	rowLen := len(LayoutQWERTY[1])
	if kb.CursorCol != rowLen-1 {
		t.Errorf("wrap left: col = %d, want %d", kb.CursorCol, rowLen-1)
	}

	// Wrap right from last column
	kb.Navigate(1, 0)
	if kb.CursorCol != 0 {
		t.Errorf("wrap right: col = %d, want 0", kb.CursorCol)
	}
}

func TestNavigate_VerticalWrap(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	kb.CursorRow = 0
	kb.CursorCol = 0

	// Wrap up from top row
	kb.Navigate(0, -1)
	if kb.CursorRow != len(LayoutQWERTY)-1 {
		t.Errorf("wrap up: row = %d, want %d", kb.CursorRow, len(LayoutQWERTY)-1)
	}
}

func TestNavigate_VerticalTargetX(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)

	// Start on a wide key, move down, the target X should find the closest key
	kb.CursorRow = 3 // home row
	kb.CursorCol = 0 // Caps (1.75 wide)

	// Moving down should find a key near the center of Caps
	kb.Navigate(0, 1)
	// Should land on row 4 (bottom alpha), near the left side
	if kb.CursorRow != 4 {
		t.Errorf("down: row = %d, want 4", kb.CursorRow)
	}

	// Moving down again and back up should return to roughly the same spot
	startCol := kb.CursorCol
	kb.Navigate(0, 1)
	kb.Navigate(0, -1)
	if kb.CursorCol != startCol {
		t.Errorf("down+up: col = %d, want %d (targetX drift)", kb.CursorCol, startCol)
	}
}

func TestNavigate_HorizontalClearsTargetX(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	kb.CursorRow = 2
	kb.CursorCol = 3

	// Vertical sets targetX
	kb.Navigate(0, 1)
	if !kb.targetXSet {
		t.Error("targetXSet should be true after vertical move")
	}

	// Horizontal clears it
	kb.Navigate(1, 0)
	if kb.targetXSet {
		t.Error("targetXSet should be false after horizontal move")
	}
}

func TestNavigate_AccentPopup(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	kb.AccentPopup = &AccentPopupState{
		Accents:  accentE,
		Selected: 0,
	}

	// Navigation in popup mode moves selection, not cursor
	startRow, startCol := kb.CursorRow, kb.CursorCol
	kb.Navigate(1, 0)
	if kb.AccentPopup.Selected != 1 {
		t.Errorf("popup right: selected = %d, want 1", kb.AccentPopup.Selected)
	}
	if kb.CursorRow != startRow || kb.CursorCol != startCol {
		t.Error("popup navigation should not move keyboard cursor")
	}

	// Can't go past bounds
	kb.AccentPopup.Selected = len(accentE) - 1
	kb.Navigate(1, 0)
	if kb.AccentPopup.Selected != len(accentE)-1 {
		t.Error("popup should not go past last accent")
	}

	kb.AccentPopup.Selected = 0
	kb.Navigate(-1, 0)
	if kb.AccentPopup.Selected != 0 {
		t.Error("popup should not go before first accent")
	}
}

func TestCurrentKey_ClampCol(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	// Force col out of range - CurrentKey should clamp
	kb.CursorCol = 999
	key := kb.CurrentKey()
	if key.Label == "" {
		t.Error("CurrentKey with out-of-range col should return last key, not panic")
	}
}

func TestDisplayLabel_ShiftCaps(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)

	// Find the 'a' key
	kb.CursorRow = 3
	kb.CursorCol = 1 // 'a' on home row

	key := kb.CurrentKey()
	if key.Label != "a" {
		t.Fatalf("expected 'a' at row 3 col 1, got %q", key.Label)
	}

	// Normal = lowercase
	if kb.DisplayLabel(key) != "a" {
		t.Errorf("normal = %q, want a", kb.DisplayLabel(key))
	}

	// Shift only = uppercase
	kb.ShiftActive = true
	if kb.DisplayLabel(key) != "A" {
		t.Errorf("shift = %q, want A", kb.DisplayLabel(key))
	}

	// Shift + caps = cancel out = lowercase
	kb.CapsActive = true
	if kb.DisplayLabel(key) != "a" {
		t.Errorf("shift+caps = %q, want a", kb.DisplayLabel(key))
	}

	// Caps only = uppercase
	kb.ShiftActive = false
	if kb.DisplayLabel(key) != "A" {
		t.Errorf("caps = %q, want A", kb.DisplayLabel(key))
	}
}

func TestToggleModifiers(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)

	for _, tt := range []struct {
		modType string
		check   func() bool
	}{
		{"shift", func() bool { return kb.ShiftActive }},
		{"caps", func() bool { return kb.CapsActive }},
		{"ctrl", func() bool { return kb.CtrlActive }},
		{"alt", func() bool { return kb.AltActive }},
		{"meta", func() bool { return kb.MetaActive }},
	} {
		key := KeyDef{IsModifier: true, ModifierType: tt.modType}
		kb.toggleModifier(key)
		if !tt.check() {
			t.Errorf("%s toggle on failed", tt.modType)
		}
		kb.toggleModifier(key)
		if tt.check() {
			t.Errorf("%s toggle off failed", tt.modType)
		}
	}
}

func TestAltTabCycling(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)

	// Find AltTab key position
	altTabRow, altTabCol := -1, -1
	for r, row := range kb.Layout {
		for c, key := range row {
			if key.Label == "AltTab" {
				altTabRow, altTabCol = r, c
				break
			}
		}
	}
	if altTabRow < 0 {
		t.Fatal("AltTab key not found in layout")
	}

	// Navigate to AltTab key
	kb.CursorRow = altTabRow
	kb.CursorCol = altTabCol

	// First press: should set AltTabHeld
	kb.PressCurrent(nil) // nil injector = no actual key events
	if !kb.AltTabHeld {
		t.Error("first AltTab press should set AltTabHeld")
	}

	// Second press: should stay held
	kb.PressCurrent(nil)
	if !kb.AltTabHeld {
		t.Error("second AltTab press should keep AltTabHeld")
	}

	// Navigate away and press different key: should release
	kb.CursorRow = 2
	kb.CursorCol = 1 // 'q'
	kb.PressCurrent(nil)
	if kb.AltTabHeld {
		t.Error("pressing non-AltTab key should release AltTabHeld")
	}
}

func TestAltTabShiftBypass(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)

	// Find AltTab key position
	for r, row := range kb.Layout {
		for c, key := range row {
			if key.Label == "AltTab" {
				kb.CursorRow = r
				kb.CursorCol = c
			}
		}
	}

	// With shift active, AltTab should send F5 (ShiftCode), not enter cycling mode
	kb.ShiftActive = true
	kb.PressCurrent(nil)
	if kb.AltTabHeld {
		t.Error("Shift+AltTab should send F5, not enter alt-tab cycling")
	}
}

func TestReleaseAltTabOnHide(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	kb.AltTabHeld = true
	kb.ReleaseAltTab(nil)
	if kb.AltTabHeld {
		t.Error("ReleaseAltTab should clear AltTabHeld")
	}
}

func TestSensitivityUpDown(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)

	// Find ↑ key
	upRow, upCol := -1, -1
	for r, row := range kb.Layout {
		for c, key := range row {
			if key.Code == KEY_UP {
				upRow, upCol = r, c
			}
		}
	}
	if upRow < 0 {
		t.Fatal("UP key not found in layout")
	}

	upCalled := false
	kb.OnSensitivityUp = func() { upCalled = true }

	// Without shift: should NOT call sensitivity callback
	kb.CursorRow = upRow
	kb.CursorCol = upCol
	kb.PressCurrent(nil)
	if upCalled {
		t.Error("sensitivity callback should not fire without shift")
	}

	// With shift: should call sensitivity callback
	kb.ShiftActive = true
	kb.PressCurrent(nil)
	if !upCalled {
		t.Error("Shift+↑ should fire OnSensitivityUp")
	}
	if kb.ShiftActive {
		t.Error("Shift should be consumed after Shift+↑")
	}

	// Down key
	downRow, downCol := -1, -1
	for r, row := range kb.Layout {
		for c, key := range row {
			if key.Code == KEY_DOWN {
				downRow, downCol = r, c
			}
		}
	}

	downCalled := false
	kb.OnSensitivityDown = func() { downCalled = true }
	kb.CursorRow = downRow
	kb.CursorCol = downCol
	kb.ShiftActive = true
	kb.PressCurrent(nil)
	if !downCalled {
		t.Error("Shift+↓ should fire OnSensitivityDown")
	}
}

func TestSensitivityClamp(t *testing.T) {
	// Simulate the clamping logic from app.go callbacks
	sensitivity := 49
	sensitivity = min(50, sensitivity+2)
	if sensitivity != 50 {
		t.Errorf("clamp up: got %d, want 50", sensitivity)
	}
	sensitivity = min(50, sensitivity+2)
	if sensitivity != 50 {
		t.Errorf("clamp up at max: got %d, want 50", sensitivity)
	}

	sensitivity = 2
	sensitivity = max(1, sensitivity-2)
	if sensitivity != 1 {
		t.Errorf("clamp down near min: got %d, want 1", sensitivity)
	}
	sensitivity = max(1, sensitivity-2)
	if sensitivity != 1 {
		t.Errorf("clamp down at min: got %d, want 1", sensitivity)
	}
}
