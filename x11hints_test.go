package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
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

// startXvfb starts a private X server with one screen of size (WxHxD) and
// returns its DISPLAY, so a test can change root properties without touching
// the user's desktop. -noreset: by default Xvfb wipes properties and atoms
// when its last client disconnects.
func startXvfb(t *testing.T, size string) string {
	t.Helper()
	bin, err := exec.LookPath("Xvfb")
	if err != nil {
		t.Skip("Xvfb not installed")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "-displayfd", "3", "-screen", "0", size, "-nolisten", "tcp", "-noreset") //nolint:gosec,noctx // Xvfb from PATH, killed in Cleanup
	cmd.ExtraFiles = []*os.File{w}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil {
		t.Fatalf("Xvfb gave no display number: %v", err)
	}
	return ":" + strings.TrimSpace(line)
}

// B4: the active window or a listed client can close while the app reads
// it. Xlib's default error handler then ends the process. The reads run in
// a child process, so a crash fails this test instead of the whole run.
func TestX11BadWindowReads(t *testing.T) {
	const gone = 0x3fffff0 // a window ID that does not exist
	if os.Getenv("OSK_X11_BADWINDOW_CHILD") == "1" {
		hasX11 = true
		// A bare Xvfb lacks these atoms and the app would return before
		// the read: setting any property creates them
		for _, atom := range []string{"_NET_WM_STATE", "_NET_WM_STATE_FULLSCREEN", "_NET_WM_STRUT_PARTIAL"} {
			setRootWindowsForTest(atom, 0)
		}
		setRootWindowsForTest("_NET_ACTIVE_WINDOW", gone)
		setRootWindowsForTest("_NET_CLIENT_LIST", gone)
		if IsFullscreenActive() {
			t.Error("a window that does not exist reads as fullscreen")
		}
		if top, bottom, left, right := GetStrutInsets(MonitorRect{W: 800, H: 600}); top|bottom|left|right != 0 {
			t.Errorf("insets %d %d %d %d from a window that does not exist", top, bottom, left, right)
		}
		fmt.Println("survived")
		return
	}
	display := startXvfb(t, "800x600x24")
	cmd := exec.Command(os.Args[0], "-test.run=^TestX11BadWindowReads$") //nolint:gosec,noctx // the test binary itself
	cmd.Env = append(os.Environ(), "OSK_X11_BADWINDOW_CHILD=1", "DISPLAY="+display)
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "survived") {
		t.Fatalf("the reads ended the process (%v):\n%s", err, out)
	}
}
