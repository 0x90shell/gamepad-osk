package main

import (
	"strings"
	"testing"
)

func TestUinputError_PermissionDenied_NotInGroup(t *testing.T) {
	err := uinputError(-1)
	if err == nil {
		t.Fatal("uinputError(-1) returned nil")
	}
	msg := err.Error()
	if msg == "" {
		t.Error("uinputError returned empty error message")
	}
}

func TestUinputError_SetupFailed(t *testing.T) {
	err := uinputError(-2)
	if err == nil {
		t.Fatal("uinputError(-2) returned nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "uinput") {
		t.Errorf("error message should mention uinput: %q", msg)
	}
}

func TestUinputError_CreateFailed(t *testing.T) {
	err := uinputError(-3)
	if err == nil {
		t.Fatal("uinputError(-3) returned nil")
	}
}

func TestUinputError_SingleLine(t *testing.T) {
	// All error codes must return single-line errors (no embedded \n)
	// to ensure proper log formatting with timestamps on each line
	for _, code := range []int{-1, -2, -3} {
		err := uinputError(code)
		if err == nil {
			t.Fatalf("uinputError(%d) returned nil", code)
		}
		if strings.Contains(err.Error(), "\n") {
			t.Errorf("uinputError(%d) contains newline: %q", code, err.Error())
		}
	}
}

func TestLogPermissionFix_NoPanic(_ *testing.T) {
	// logPermissionFix should not panic regardless of group membership
	logPermissionFix()
}

func TestColorHelpers(t *testing.T) {
	// Verify color functions return non-empty strings
	msg := "test message"
	if colorRed(msg) == "" {
		t.Error("colorRed returned empty")
	}
	if colorYellow(msg) == "" {
		t.Error("colorYellow returned empty")
	}
	if colorGreen(msg) == "" {
		t.Error("colorGreen returned empty")
	}
	// All must contain the original message
	if !strings.Contains(colorRed(msg), msg) {
		t.Error("colorRed dropped the message content")
	}
	if !strings.Contains(colorYellow(msg), msg) {
		t.Error("colorYellow dropped the message content")
	}
	if !strings.Contains(colorGreen(msg), msg) {
		t.Error("colorGreen dropped the message content")
	}
	if colorDim(msg) == "" {
		t.Error("colorDim returned empty")
	}
	if !strings.Contains(colorDim(msg), msg) {
		t.Error("colorDim dropped the message content")
	}
}

func TestNilInjector_PressCurrent(_ *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	kb.PressCurrent(nil) // should not panic
}

func TestNilInjector_PressCurrentAccent(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	kb.AccentPopup = &AccentPopupState{
		Accents:  []AccentDef{{"á", 0xe1}},
		Selected: 0,
	}
	kb.PressCurrent(nil) // should not panic, should close popup
	if kb.AccentPopup != nil {
		t.Error("accent popup should be closed after PressCurrent")
	}
}

func TestNilInjector_PressCurrentCombo(_ *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	kb.CursorRow = 0
	kb.CursorCol = 0
	kb.PressCurrent(nil) // should not panic
}

func TestNilInjector_PressCurrentPaste(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	for r, row := range LayoutQWERTY {
		for c, key := range row {
			if key.Label == "Paste" {
				kb.CursorRow = r
				kb.CursorCol = c
				kb.PressCurrent(nil) // should not panic
				return
			}
		}
	}
	t.Skip("Paste key not found in layout")
}
