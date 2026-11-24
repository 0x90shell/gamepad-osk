package main

import (
	"os"
	"os/exec"
	"testing"
)

// x11Child runs body in a child process on its own Xvfb, 1600x600: the left
// 800 px are the game's monitor, the right half "monitor 2". A child, because
// an Xlib error can end the process and hasX11 is global.
func x11Child(t *testing.T, name string, body func(t *testing.T)) {
	t.Helper()
	if os.Getenv("OSK_X11_CHILD") == name {
		hasX11 = true
		body(t)
		return
	}
	display := startXvfb(t, "1600x600x24")
	cmd := exec.Command(os.Args[0], "-test.run=^"+name+"$", "-test.v") //nolint:gosec,noctx // the test binary itself
	cmd.Env = append(os.Environ(), "OSK_X11_CHILD="+name, "DISPLAY="+display)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// game is a window that has focus and is the active window, as a window
// manager would have it after a click. It sits on the right half ("monitor
// 2"), away from 0,0, so a warp to the wrong origin shows.
func game(parent uint64, x, y, w, h int, fullscreen bool) uint64 {
	testAtoms()
	win := testWindow(parent, x, y, w, h)
	if fullscreen {
		testFullscreen(win)
	}
	testFocus(win)
	setRootWindowsForTest("_NET_ACTIVE_WINDOW", win)
	return win
}

func pointerAt(t *testing.T, x, y int, why string) {
	t.Helper()
	if px, py := testPointer(); px != x || py != y {
		t.Errorf("%s: pointer at %d,%d, want %d,%d", why, px, py, x, y)
	}
}

// U85: on hide the pointer comes back to the middle of a fullscreen game
// when it was outside it (on the other monitor, moved there with the right
// stick: Kingdom Rush was minimized by a click there, 2026-04); inside the
// game it stays where it is, up to the game's last pixel. A windowed game does
// not read as fullscreen (hide only warps for one that does).
func TestWarpPointerIfOutside(t *testing.T) {
	x11Child(t, "TestWarpPointerIfOutside", func(t *testing.T) {
		game(0, 800, 100, 640, 480, true)
		SaveFocusedWindow()
		if !IsSavedWindowFullscreen() {
			t.Fatal("the fullscreen game does not read as fullscreen")
		}
		testMovePointer(100, 300) // the other monitor
		if !WarpPointerIfOutside() {
			t.Error("pointer on the other monitor: no warp")
		}
		pointerAt(t, 800+320, 100+240, "after the warp")
		testMovePointer(800+640-1, 100+480-1) // the game's last pixel
		if WarpPointerIfOutside() {
			t.Error("pointer on the game's last pixel: warped")
		}
		pointerAt(t, 800+640-1, 100+480-1, "pointer inside the game")
		testMovePointer(800+640, 300) // one past its right edge
		if !WarpPointerIfOutside() {
			t.Error("pointer one past the game's right edge: no warp")
		}

		game(0, 800, 100, 600, 400, false)
		SaveFocusedWindow()
		if IsSavedWindowFullscreen() {
			t.Error("a windowed game reads as fullscreen")
		}
	})
}

// U86: hide gives focus back to the window that had it at show; if that
// window is gone, or unmapped (Wine hides windows: BadMatch), the process
// survives (with Xlib's default error handler it would end here).
func TestRestoreFocus(t *testing.T) {
	x11Child(t, "TestRestoreFocus", func(t *testing.T) {
		g := game(0, 800, 0, 800, 600, true)
		SaveFocusedWindow()
		other := testWindow(0, 0, 0, 800, 600)
		testFocus(other)
		RestoreFocus()
		if f := testFocused(); f != g {
			t.Errorf("focus on 0x%x after RestoreFocus, want the game 0x%x", f, g)
		}
		testFocus(g)
		SaveFocusedWindow()
		testFocus(other)
		testUnmap(g)
		RestoreFocus()
		testDestroy(g)
		RestoreFocus()
	})
}

// U87: under a window manager the game sits in a frame, and the focus can be
// on a window inside the game (Wine). The fullscreen state is on the game,
// the frame is the top-level: the game still reads as fullscreen (through
// _NET_ACTIVE_WINDOW) and the pointer goes to the middle of the frame.
func TestWarpPointerFramedGame(t *testing.T) {
	x11Child(t, "TestWarpPointerFramedGame", func(t *testing.T) {
		frame := testWindow(0, 800, 100, 640, 480)
		client := game(frame, 0, 0, 640, 480, true)
		for _, focus := range []uint64{client, testWindow(client, 0, 0, 10, 10)} {
			testFocus(focus)
			SaveFocusedWindow()
			if !IsSavedWindowFullscreen() {
				t.Fatalf("focus 0x%x in the framed fullscreen game: not fullscreen", focus)
			}
			testMovePointer(100, 300)
			if !WarpPointerIfOutside() {
				t.Error("pointer on the other monitor: no warp")
			}
			pointerAt(t, 800+320, 100+240, "after the warp")
		}
	})
}

// U88: a windowed game had focus at show; another window, fullscreen on the
// other monitor, is the active one at hide. The game is not fullscreen, so
// the pointer must not be sent into it (fixed 2026-11-22: the active window
// counted whatever it was).
func TestNoWarpForAnotherFullscreenWindow(t *testing.T) {
	x11Child(t, "TestNoWarpForAnotherFullscreenWindow", func(t *testing.T) {
		game(0, 0, 0, 600, 400, false)
		SaveFocusedWindow()
		video := testWindow(0, 800, 0, 800, 600)
		testFullscreen(video)
		setRootWindowsForTest("_NET_ACTIVE_WINDOW", video)
		if IsSavedWindowFullscreen() {
			t.Error("the windowed game reads as fullscreen because another window is")
		}
	})
}

// U89: the focus is on a separate top-level window of the game (a Wine or SDL
// helper, not inside the game's window) while the game is the active window:
// the game still reads as fullscreen, as in April 2026.
func TestFullscreenWithHelperFocus(t *testing.T) {
	x11Child(t, "TestFullscreenWithHelperFocus", func(t *testing.T) {
		game(0, 800, 0, 800, 600, true)
		testFocus(testWindow(0, 0, 0, 10, 10)) // the helper
		SaveFocusedWindow()
		if !IsSavedWindowFullscreen() {
			t.Error("the active fullscreen game is not found through its helper's focus")
		}
		testMovePointer(100, 300)
		WarpPointerIfOutside()
		pointerAt(t, 800+400, 300, "the warp goes to the game, not the helper")
	})
}

// U90: the keyboard started during a terminal session, then a game was
// launched: at hide the active window is the game, which was not open at the
// save, so hide leaves it alone (fixed 2026-11-22: focus went back to the
// terminal and the game lost it, like BallxPit's pad in 2026-04).
func TestNewProgramKeepsFocus(t *testing.T) {
	x11Child(t, "TestNewProgramKeepsFocus", func(t *testing.T) {
		terminal := game(0, 0, 0, 800, 600, false)
		setRootWindowsForTest("_NET_CLIENT_LIST", terminal)
		SaveFocusedWindow()
		launched := game(0, 800, 0, 800, 600, true)
		setRootWindowsForTest("_NET_CLIENT_LIST", terminal, launched)
		if !ActiveWindowIsNew() {
			t.Error("a game launched after the save does not read as new")
		}
	})
}

// U91: the keyboard started (or shown) during a game; a stray click made an
// already open window active. It is not new: hide gives the game its focus
// back, as before. Nothing active, or the saved window: not new either.
func TestStrayClickIsNotNew(t *testing.T) {
	x11Child(t, "TestStrayClickIsNotNew", func(t *testing.T) {
		g := game(0, 0, 0, 800, 600, true)
		other := testWindow(0, 800, 0, 800, 600)
		setRootWindowsForTest("_NET_CLIENT_LIST", g, other)
		SaveFocusedWindow()
		if ActiveWindowIsNew() {
			t.Error("the saved game reads as new")
		}
		testFocus(other)
		setRootWindowsForTest("_NET_ACTIVE_WINDOW", other)
		if ActiveWindowIsNew() {
			t.Error("a window open at the save reads as new")
		}
		setRootWindowsForTest("_NET_ACTIVE_WINDOW", 0)
		if ActiveWindowIsNew() {
			t.Error("no active window reads as new")
		}
	})
}

// U92: the Kingdom Rush case (2026-04): a framed fullscreen game, the pointer
// taken to monitor 2 and a click on a window there (it becomes active). On
// hide the game still reads as fullscreen and the pointer comes back to the
// middle of its frame (fixed 2026-11-22: the check looked at the frame and at
// the clicked window only, so the pointer stayed on monitor 2).
func TestWarpAfterStrayClick(t *testing.T) {
	x11Child(t, "TestWarpAfterStrayClick", func(t *testing.T) {
		frame := testWindow(0, 0, 0, 800, 600)
		game(frame, 0, 0, 800, 600, true)
		SaveFocusedWindow()
		other := testWindow(0, 1000, 100, 400, 300)
		testFocus(other) // the click
		setRootWindowsForTest("_NET_ACTIVE_WINDOW", other)
		testMovePointer(1200, 250)
		if !IsSavedWindowFullscreen() {
			t.Fatal("after a click elsewhere the framed fullscreen game does not read as fullscreen")
		}
		if !WarpPointerIfOutside() {
			t.Error("pointer on the clicked window: no warp")
		}
		pointerAt(t, 400, 300, "after the warp")
	})
}

// U93: the game minimized itself when the click took its focus (SDL does in
// exclusive fullscreen). X refuses focus to an unmapped window, so hide asks
// the window manager to activate the game, as Alt+Tab would (fixed
// 2026-11-22: it stayed minimized). A game still on screen just gets focus.
func TestRestoreMinimizedGame(t *testing.T) {
	x11Child(t, "TestRestoreMinimizedGame", func(t *testing.T) {
		g := game(0, 0, 0, 800, 600, true)
		SaveFocusedWindow()
		other := testWindow(0, 1000, 100, 400, 300)
		testWatchRoot()
		testFocus(other)
		testUnmap(g) // minimized
		RestoreFocus()
		if !testSawActivate(g) {
			t.Error("no request to the window manager to activate the minimized game")
		}
		testWindowMap(g)
		testFocus(other)
		RestoreFocus()
		if testSawActivate(g) {
			t.Error("asked the window manager although the game is on screen")
		}
		if f := testFocused(); f != g {
			t.Errorf("focus 0x%x, want the game 0x%x", f, g)
		}
	})
}

// U94: the keyboard was shown over a terminal, then a game was launched (it
// becomes active): the game becomes the window hide returns to. After that a
// stray click on a window already open is not followed, and hide gives the
// focus back to the game (fixed 2026-11-22: it went back to the terminal).
func TestFollowNewProgram(t *testing.T) {
	x11Child(t, "TestFollowNewProgram", func(t *testing.T) {
		terminal := game(0, 0, 0, 800, 600, false)
		other := testWindow(0, 1000, 100, 400, 300)
		setRootWindowsForTest("_NET_CLIENT_LIST", terminal, other)
		SaveFocusedWindow()
		if FollowNewProgram() {
			t.Error("followed with nothing new")
		}
		launched := game(0, 800, 0, 800, 600, true)
		setRootWindowsForTest("_NET_CLIENT_LIST", terminal, other, launched)
		if !FollowNewProgram() {
			t.Fatal("the launched game was not followed")
		}
		testFocus(other) // the stray click
		setRootWindowsForTest("_NET_ACTIVE_WINDOW", other)
		if FollowNewProgram() {
			t.Error("followed a window that was already open")
		}
		RestoreFocus()
		if f := testFocused(); f != launched {
			t.Errorf("focus 0x%x after hide, want the launched game 0x%x", f, launched)
		}
	})
}

// U96: hide within one follow poll of a new program (FollowNewProgram has not
// saved it yet). The keyboard was shown over a fullscreen game, then another
// program started on monitor 2 and took the focus: ReturnFocus leaves the
// focus and the pointer with it, not back on the game.
func TestReturnFocusLeavesNewProgram(t *testing.T) {
	x11Child(t, "TestReturnFocusLeavesNewProgram", func(t *testing.T) {
		g := game(0, 0, 0, 800, 600, true)
		setRootWindowsForTest("_NET_CLIENT_LIST", g)
		SaveFocusedWindow()
		launched := game(0, 800, 0, 800, 600, false)
		setRootWindowsForTest("_NET_CLIENT_LIST", g, launched)
		testMovePointer(1000, 300)
		ReturnFocus()
		if f := testFocused(); f != launched {
			t.Errorf("focus on 0x%x, want the new program 0x%x", f, launched)
		}
		pointerAt(t, 1000, 300, "pointer on the new program")
	})
}

// U97: the usual hide: the window that had the focus at show was open
// already, so ReturnFocus gives the fullscreen game its focus back and brings
// the pointer to its middle.
func TestReturnFocusToSavedGame(t *testing.T) {
	x11Child(t, "TestReturnFocusToSavedGame", func(t *testing.T) {
		g := game(0, 800, 0, 800, 600, true)
		other := testWindow(0, 0, 0, 800, 600)
		setRootWindowsForTest("_NET_CLIENT_LIST", g, other)
		SaveFocusedWindow()
		testFocus(other)
		setRootWindowsForTest("_NET_ACTIVE_WINDOW", other)
		testMovePointer(100, 300)
		ReturnFocus()
		if f := testFocused(); f != g {
			t.Errorf("focus on 0x%x, want the game 0x%x", f, g)
		}
		pointerAt(t, 1200, 300, "pointer back in the game")
	})
}

// U98: hide back to a windowed game: the focus goes back, the pointer stays
// where the user left it (only a fullscreen game gets it back).
func TestReturnFocusWindowedNoWarp(t *testing.T) {
	x11Child(t, "TestReturnFocusWindowedNoWarp", func(t *testing.T) {
		g := game(0, 800, 0, 600, 400, false)
		other := testWindow(0, 0, 0, 800, 600)
		setRootWindowsForTest("_NET_CLIENT_LIST", g, other)
		SaveFocusedWindow()
		testFocus(other)
		setRootWindowsForTest("_NET_ACTIVE_WINDOW", other)
		testMovePointer(100, 300)
		ReturnFocus()
		if f := testFocused(); f != g {
			t.Errorf("focus on 0x%x, want the game 0x%x", f, g)
		}
		pointerAt(t, 100, 300, "pointer left alone for a windowed game")
	})
}
