package main

import (
	"math"
	"testing"
)

func TestCloseCancelsAccentPopup(t *testing.T) {
	app := &App{visible: true}
	app.running.Store(true)
	rend := &Renderer{}
	kb := NewKeyboardState(LayoutQWERTY)
	kb.CtrlActive = true // any real key press clears it
	kb.AccentPopup = &AccentPopupState{Accents: accentE}

	app.handleAction(Action{Type: ActionClose}, kb, nil, rend)
	if kb.AccentPopup != nil {
		t.Fatal("close should cancel the open popup")
	}
	if !app.running.Load() {
		t.Fatal("close with the popup open must not quit")
	}

	// The press button is still held; its release must type nothing
	app.handleAction(Action{Type: ActionPress}, kb, nil, rend)
	if !kb.CtrlActive || kb.SkipPress {
		t.Errorf("release after cancel typed a key (ctrl=%v skip=%v)", kb.CtrlActive, kb.SkipPress)
	}

	// The next press types normally
	app.handleAction(Action{Type: ActionPress}, kb, nil, rend)
	if kb.CtrlActive {
		t.Error("press after the skipped release should type")
	}

	// Without a popup, close still quits
	app.handleAction(Action{Type: ActionClose}, kb, nil, rend)
	if app.running.Load() {
		t.Error("close without a popup should quit")
	}
}

func TestChooseOpacityPath(t *testing.T) {
	tests := []struct {
		name          string
		opacity       float64
		layerShell    bool
		alphaModifier bool
		want          opacityPath
	}{
		{"opaque on X11", 1, false, false, opacityOpaque},
		{"opaque on layer-shell", 1, true, true, opacityOpaque},
		{"X11", 0.8, false, false, opacityWindow},
		{"Wayland without layer-shell", 0.8, false, true, opacityWindow},
		{"layer-shell with alpha modifier", 0.8, true, true, opacitySurface},
		{"layer-shell without alpha modifier", 0.8, true, false, opacityLayer},
	}
	for _, tt := range tests {
		if got := chooseOpacityPath(tt.opacity, tt.layerShell, tt.alphaModifier); got != tt.want {
			t.Errorf("%s: got %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestAlphaFactor(t *testing.T) {
	tests := []struct {
		opacity float64
		want    uint32
	}{
		{0, 0},
		{1, math.MaxUint32},
		{0.5, 2147483648},
		{-1, 0},
		{2, math.MaxUint32},
	}
	for _, tt := range tests {
		if got := alphaFactor(tt.opacity); got != tt.want {
			t.Errorf("alphaFactor(%v) = %d, want %d", tt.opacity, got, tt.want)
		}
	}
}

func TestChooseVideoDriver(t *testing.T) {
	tests := []struct {
		layerShell  int
		hasXWayland bool
		want        string
	}{
		{1, true, "wayland"},  // Sway, Hyprland, KDE
		{1, false, "wayland"}, // layer-shell, no XWayland
		{0, true, "x11"},      // GNOME: overlay through XWayland
		{0, false, "wayland"}, // GNOME without XWayland: normal window
		{-1, true, "wayland"}, // probe failed: let SDL report it
	}
	for _, tt := range tests {
		if got := chooseVideoDriver(tt.layerShell, tt.hasXWayland); got != tt.want {
			t.Errorf("chooseVideoDriver(%d, %v) = %q, want %q", tt.layerShell, tt.hasXWayland, got, tt.want)
		}
	}
}

// A pad that disconnects mid-hold never sends its releases: repeat, a held
// Shift and the accent popup end; toggled keys stay.
func TestReleasePadHolds(t *testing.T) {
	app := &App{visible: true}
	app.running.Store(true)
	kb := NewKeyboardState(LayoutQWERTY)
	app.startRepeat(ActionBackspace)
	kb.ShiftHeld, kb.ShiftActive = true, true
	kb.AccentPopup = &AccentPopupState{Accents: accentE}
	kb.SkipPress = true
	kb.CapsActive, kb.CtrlActive = true, true

	app.releasePadHolds(kb, nil)
	if app.repeatAction != ActionNone {
		t.Errorf("repeat still %v", app.repeatAction)
	}
	if kb.ShiftHeld || kb.ShiftActive {
		t.Error("held Shift not released")
	}
	if kb.AccentPopup != nil || kb.SkipPress {
		t.Errorf("popup %v, skip %v: want closed, no skip", kb.AccentPopup, kb.SkipPress)
	}
	if !kb.CapsActive || !kb.CtrlActive {
		t.Error("toggled Caps or Ctrl was cleared")
	}

	// Shift toggled with the Shift key (not held) stays on
	kb.ShiftActive, kb.ShiftHeld = true, false
	app.releasePadHolds(kb, nil)
	if !kb.ShiftActive {
		t.Error("toggled Shift was cleared")
	}
}

// LT is Shift only while the keyboard is shown: a press it never saw (made
// while hidden, for the game) does not shift the first keys after a show.
func TestShiftTriggerAcrossHide(t *testing.T) {
	app := &App{visible: true}
	app.running.Store(true)
	kb := NewKeyboardState(LayoutQWERTY)
	rend := &Renderer{}
	gp := newTestGamepad()
	lt := func(v int32) { app.handleAction(gp.handleAxis(ABS_Z, v), kb, nil, rend) }
	hide := func() { app.kb = kb; app.releaseHeld(); app.visible = false }
	show := func() { app.visible = true }

	lt(255)
	if !kb.ShiftActive {
		t.Fatal("LT while shown: no Shift")
	}
	hide()
	if kb.ShiftActive || kb.ShiftHeld {
		t.Fatal("Shift still on after hide")
	}
	lt(0) // let go while hidden
	show()
	lt(255)
	if !kb.ShiftActive {
		t.Error("LT let go while hidden, pressed after show: no Shift")
	}
	lt(0)

	lt(255) // held through a hide, let go after the show
	hide()
	show()
	lt(0)
	if kb.ShiftActive {
		t.Error("LT held through a hide and let go: Shift on")
	}
	lt(255)
	if !kb.ShiftActive {
		t.Error("LT held through a hide, let go and pressed: no Shift")
	}
	lt(0)

	hide()
	lt(255) // pressed for the game
	show()
	if kb.ShiftActive {
		t.Error("LT pressed while hidden shifts after show")
	}
	lt(0)
	lt(255)
	if !kb.ShiftActive {
		t.Error("LT let go and pressed again after show: no Shift")
	}
}
