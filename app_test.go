package main

import (
	"math"
	"testing"
)

func TestCloseCancelsAccentPopup(t *testing.T) {
	app := &App{visible: true, running: true}
	rend := &Renderer{}
	kb := NewKeyboardState(LayoutQWERTY)
	kb.CtrlActive = true // any real key press clears it
	kb.AccentPopup = &AccentPopupState{Accents: accentE}

	app.handleAction(Action{Type: ActionClose}, kb, nil, rend)
	if kb.AccentPopup != nil {
		t.Fatal("close should cancel the open popup")
	}
	if !app.running {
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
	if app.running {
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
		{"layer-shell without alpha modifier", 0.8, true, false, opacityNone},
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
