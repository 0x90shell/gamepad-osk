package main

import (
	"os"
	"testing"
)

// TestRestoreFocusInvalidWindow verifies that RestoreFocus does not crash when
// the saved window ID is invalid (e.g., a Wine/Bottles fullscreen game window
// that is not viewable). Previously this caused a BadMatch X error that killed
// the process.
func TestRestoreFocusInvalidWindow(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("no X11 display available")
	}
	// Force the X11 path -hasX11 is normally set by initX11Detection() after
	// SDL init, which never runs in tests.
	prev := hasX11
	hasX11 = true
	defer func() { hasX11 = prev }()

	// 0xDEADBEEF is a window ID that cannot exist -XSetInputFocus will return
	// BadMatch, which restore_focus() must handle without crashing.
	setPrevFocusedForTest(0xDEADBEEF)
	RestoreFocus() // must not panic or call os.Exit
}

func TestIsFullscreenActiveNoX11(t *testing.T) {
	prev := hasX11
	hasX11 = false
	defer func() { hasX11 = prev }()

	if IsFullscreenActive() {
		t.Error("IsFullscreenActive should return false when hasX11 is false")
	}
}

func TestIsFullscreenActiveWithDisplay(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("no X11 display available")
	}
	prev := hasX11
	hasX11 = true
	defer func() { hasX11 = prev }()

	// Result depends on desktop state -just verify no crash
	result := IsFullscreenActive()
	t.Logf("IsFullscreenActive() = %v (depends on current desktop state)", result)
}
