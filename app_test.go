package main

import "testing"

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
