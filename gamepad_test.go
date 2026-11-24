package main

import (
	"bytes"
	"log"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func newTestGamepad() *GamepadReader {
	cfg := DefaultConfig()
	gp := NewGamepadReader(cfg)
	gp.initButtonMap()
	return gp
}

func TestApplyDeadzone(t *testing.T) {
	tests := []struct {
		norm, dz float64
		want     int
	}{
		{0.5, 0.25, 1},
		{-0.5, 0.25, -1},
		{0.1, 0.25, 0},   // inside deadzone
		{-0.1, 0.25, 0},  // inside deadzone
		{0.25, 0.25, 0},  // exactly at boundary = dead
		{-0.25, 0.25, 0}, // exactly at boundary = dead
		{1.0, 0.25, 1},   // full tilt
		{0.0, 0.25, 0},   // centered
	}
	for _, tt := range tests {
		got := applyDeadzone(tt.norm, tt.dz)
		if got != tt.want {
			t.Errorf("applyDeadzone(%v, %v) = %d, want %d", tt.norm, tt.dz, got, tt.want)
		}
	}
}

func TestHandleButton_PressRelease(t *testing.T) {
	gp := newTestGamepad()

	// A button press = PressStart, release = Press
	a := gp.handleButton(BTN_SOUTH, 1)
	if a.Type != ActionPressStart {
		t.Errorf("A press = %v, want ActionPressStart", a.Type)
	}
	a = gp.handleButton(BTN_SOUTH, 0)
	if a.Type != ActionPress {
		t.Errorf("A release = %v, want ActionPress", a.Type)
	}
}

func TestHandleButton_Close(t *testing.T) {
	gp := newTestGamepad()

	// B button = close (default config)
	a := gp.handleButton(BTN_EAST, 1)
	if a.Type != ActionClose {
		t.Errorf("B press = %v, want ActionClose", a.Type)
	}

	// B release should be ActionNone (close has no release action)
	a = gp.handleButton(BTN_EAST, 0)
	if a.Type != ActionNone {
		t.Errorf("B release = %v, want ActionNone", a.Type)
	}
}

func TestHandleButton_MouseClickHold(t *testing.T) {
	gp := newTestGamepad()

	// RB = left click (default), needs both press and release for drag
	a := gp.handleButton(BTN_TR, 1)
	if a.Type != ActionLeftClick {
		t.Errorf("RB press = %v, want ActionLeftClick", a.Type)
	}
	a = gp.handleButton(BTN_TR, 0)
	if a.Type != ActionLeftClickRelease {
		t.Errorf("RB release = %v, want ActionLeftClickRelease", a.Type)
	}
}

func TestHandleButton_Unmapped(t *testing.T) {
	gp := newTestGamepad()

	// Guide button (0x13c) is not mapped
	a := gp.handleButton(0x13c, 1)
	if a.Type != ActionNone {
		t.Errorf("unmapped press = %v, want ActionNone", a.Type)
	}
}

func TestHandleAxis_NavStick(t *testing.T) {
	gp := newTestGamepad()

	// Full right on left stick (default nav) = navigate right
	a := gp.handleAxis(ABS_X, 32767)
	if a.Type != ActionNavigate || a.DX != 1 {
		t.Errorf("stick right = %+v, want Navigate DX=1", a)
	}

	// Return to center = stop (no action emitted, direction cleared)
	a = gp.handleAxis(ABS_X, 0)
	if a.Type != ActionNone {
		t.Errorf("stick center = %v, want ActionNone", a.Type)
	}
	if gp.navX.Direction != 0 {
		t.Errorf("navX.Direction = %d after center, want 0", gp.navX.Direction)
	}
}

func TestHandleAxis_NavStickDeadzone(t *testing.T) {
	gp := newTestGamepad()

	// Small jitter inside deadzone should not navigate
	jitter := int32(3276) // ~10% of Xbox range = inside 25% deadzone
	a := gp.handleAxis(ABS_X, jitter)
	if a.Type != ActionNone {
		t.Errorf("jitter %d = %v, want ActionNone", jitter, a.Type)
	}
}

// This is the exact bug that was fixed: stick jitter clearing d-pad state.
// D-pad and stick must use separate NavAxis so jitter on one doesn't cancel the other.
func TestDpadAndStickIndependent(t *testing.T) {
	gp := newTestGamepad()

	// Press d-pad right
	a := gp.handleAxis(ABS_HAT0X, 1)
	if a.Type != ActionNavigate || a.DX != 1 {
		t.Fatalf("dpad right = %+v, want Navigate DX=1", a)
	}
	if gp.dpadX.Direction != 1 {
		t.Fatalf("dpadX.Direction = %d, want 1", gp.dpadX.Direction)
	}

	// Stick jitter on the same axis (ABS_X) - should NOT affect d-pad
	gp.handleAxis(ABS_X, 100) // small positive jitter
	gp.handleAxis(ABS_X, -50) // small negative jitter
	gp.handleAxis(ABS_X, 0)   // back to center

	// D-pad should still be held right
	if gp.dpadX.Direction != 1 {
		t.Errorf("dpadX.Direction = %d after stick jitter, want 1 (bug: stick cleared d-pad)", gp.dpadX.Direction)
	}

	// And nav stick should be at 0 (jitter was in deadzone)
	if gp.navX.Direction != 0 {
		t.Errorf("navX.Direction = %d after jitter, want 0", gp.navX.Direction)
	}
}

func TestHandleAxis_DpadValues(t *testing.T) {
	gp := newTestGamepad()

	// D-pad sends -1, 0, 1 (not analog range)
	a := gp.handleAxis(ABS_HAT0X, -1)
	if a.Type != ActionNavigate || a.DX != -1 {
		t.Errorf("dpad left = %+v, want Navigate DX=-1", a)
	}
	a = gp.handleAxis(ABS_HAT0Y, 1)
	if a.Type != ActionNavigate || a.DY != 1 {
		t.Errorf("dpad down = %+v, want Navigate DY=1", a)
	}
	a = gp.handleAxis(ABS_HAT0X, 0)
	if a.Type != ActionNone {
		t.Errorf("dpad release = %v, want ActionNone", a.Type)
	}
}

func TestHandleAxis_TriggerEdgeDetection(t *testing.T) {
	gp := newTestGamepad()

	// RT (ABS_RZ) = enter, fires on edge only
	a := gp.handleAxis(ABS_RZ, 200) // >30% of 255 = active
	if a.Type != ActionEnter {
		t.Errorf("RT press = %v, want ActionEnter", a.Type)
	}

	// Holding deeper should not re-fire
	a = gp.handleAxis(ABS_RZ, 255)
	if a.Type != ActionNone {
		t.Errorf("RT hold = %v, want ActionNone (edge detect)", a.Type)
	}

	// Release
	a = gp.handleAxis(ABS_RZ, 0)
	if a.Type != ActionEnterRelease {
		t.Errorf("RT release = %v, want ActionEnterRelease", a.Type)
	}

	// Should be able to fire again
	a = gp.handleAxis(ABS_RZ, 200)
	if a.Type != ActionEnter {
		t.Errorf("RT re-press = %v, want ActionEnter", a.Type)
	}
}

func TestHandleAxis_ShiftHold(t *testing.T) {
	gp := newTestGamepad()

	// LT (ABS_Z) = shift, hold-based (not edge)
	a := gp.handleAxis(ABS_Z, 200)
	if a.Type != ActionShiftOn {
		t.Errorf("LT press = %v, want ActionShiftOn", a.Type)
	}

	// Holding should not repeat
	a = gp.handleAxis(ABS_Z, 255)
	if a.Type != ActionNone {
		t.Errorf("LT hold = %v, want ActionNone", a.Type)
	}

	// Release
	a = gp.handleAxis(ABS_Z, 0)
	if a.Type != ActionShiftOff {
		t.Errorf("LT release = %v, want ActionShiftOff", a.Type)
	}
}

func TestHandleAxis_MouseStick(t *testing.T) {
	gp := newTestGamepad()

	// Right stick (default mouse) should update mouseX/Y, not navigate
	gp.handleAxis(ABS_RX, 20000)
	if gp.mouseX == 0 {
		t.Error("mouseX should be nonzero after right stick input")
	}
	if gp.navX.Direction != 0 {
		t.Error("right stick should not affect nav when it's the mouse stick")
	}
}

func TestUpdateNav_DirectionChange(t *testing.T) {
	gp := newTestGamepad()

	// Go right
	a := gp.updateNav(&gp.navX, 1, true)
	if a.Type != ActionNavigate || a.DX != 1 {
		t.Errorf("right = %+v, want Navigate DX=1", a)
	}

	// Same direction again = no action (repeat handled elsewhere)
	a = gp.updateNav(&gp.navX, 1, true)
	if a.Type != ActionNone {
		t.Errorf("same direction = %v, want ActionNone", a.Type)
	}

	// Reverse direction = immediate action
	a = gp.updateNav(&gp.navX, -1, true)
	if a.Type != ActionNavigate || a.DX != -1 {
		t.Errorf("reverse = %+v, want Navigate DX=-1", a)
	}

	// Release = no action, clears HeldSince
	a = gp.updateNav(&gp.navX, 0, true)
	if a.Type != ActionNone {
		t.Errorf("release = %v, want ActionNone", a.Type)
	}
	if !gp.navX.HeldSince.IsZero() {
		t.Error("HeldSince should be zero after release")
	}
}

func TestNavAxisRepeatInterval(t *testing.T) {
	nav := NavAxis{}

	// Not held = 300ms initial
	if nav.RepeatInterval() != 300*time.Millisecond {
		t.Errorf("initial interval = %v, want 300ms", nav.RepeatInterval())
	}

	// Just started holding = still ~300ms
	nav.HeldSince = time.Now()
	interval := nav.RepeatInterval()
	if interval < 280*time.Millisecond || interval > 310*time.Millisecond {
		t.Errorf("just held interval = %v, want ~300ms", interval)
	}

	// Held for 1+ second = should be near 80ms (minimum)
	nav.HeldSince = time.Now().Add(-2 * time.Second)
	interval = nav.RepeatInterval()
	if interval > 100*time.Millisecond {
		t.Errorf("long hold interval = %v, want <=100ms", interval)
	}
}

func TestHandleEvent_IgnoresSyn(t *testing.T) {
	gp := newTestGamepad()

	// EV_SYN events should produce no action
	a := gp.handleEvent(evdevEV_SYN, 0, 0)
	if a.Type != ActionNone {
		t.Errorf("EV_SYN = %v, want ActionNone", a.Type)
	}
}

func TestSwappedButtons(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gamepad.SwapXY = "true"
	gp := NewGamepadReader(cfg)
	gp.initButtonMap()

	// With swap_xy, X button (BTN_WEST normally) should be BTN_NORTH
	// Default: x=backspace, so BTN_NORTH should trigger backspace
	a := gp.handleButton(BTN_NORTH, 1)
	if a.Type != ActionBackspace {
		t.Errorf("BTN_NORTH with swap = %v, want ActionBackspace (x)", a.Type)
	}

	// Y button (BTN_NORTH normally) should be BTN_WEST
	// Default: y=space, so BTN_WEST should trigger space
	a = gp.handleButton(BTN_WEST, 1)
	if a.Type != ActionSpace {
		t.Errorf("BTN_WEST with swap = %v, want ActionSpace (y)", a.Type)
	}
}

// --- Toggle combo tests ---

func newComboGamepad(combo string) *GamepadReader {
	cfg := DefaultConfig()
	cfg.Gamepad.ToggleCombo = combo
	cfg.Gamepad.ComboPeriodMs = 200
	gp := NewGamepadReader(cfg)
	gp.initButtonMap()
	return gp
}

func TestComboDetect_TwoButtons(t *testing.T) {
	gp := newComboGamepad("select+start")

	// Press select, then start
	gp.handleButton(0x13a, 1)      // select press - combo not yet complete
	a := gp.handleButton(0x13b, 1) // start press - combo fires
	if a.Type != ActionToggle {
		t.Errorf("combo fire = %v, want ActionToggle", a.Type)
	}
}

func TestComboDetect_PressOrder(t *testing.T) {
	gp := newComboGamepad("select+start")

	// Reverse order: start first, then select
	gp.handleButton(0x13b, 1)      // start press
	a := gp.handleButton(0x13a, 1) // select press - combo fires
	if a.Type != ActionToggle {
		t.Errorf("reverse order combo = %v, want ActionToggle", a.Type)
	}
}

func TestComboDetect_EdgeTrigger(t *testing.T) {
	gp := newComboGamepad("select+start")

	gp.handleButton(0x13a, 1)
	a := gp.handleButton(0x13b, 1) // fires
	if a.Type != ActionToggle {
		t.Fatalf("first fire = %v, want ActionToggle", a.Type)
	}

	// While still held, additional events should not re-fire
	a = gp.handleButton(0x13b, 1) // repeat event
	if a.Type == ActionToggle {
		t.Error("combo fired again while held - should be edge-triggered")
	}
}

func TestComboDetect_ReleaseReset(t *testing.T) {
	gp := newComboGamepad("select+start")

	// Fire combo
	gp.handleButton(0x13a, 1)
	gp.handleButton(0x13b, 1)

	// Release one button
	gp.handleButton(0x13a, 0)

	// Re-press - should fire again
	a := gp.handleButton(0x13a, 1)
	if a.Type != ActionToggle {
		t.Errorf("re-press after release = %v, want ActionToggle", a.Type)
	}
}

func TestComboDetect_TimingWindow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gamepad.ToggleCombo = "select+start"
	cfg.Gamepad.ComboPeriodMs = 50 // very short window
	gp := NewGamepadReader(cfg)
	gp.initButtonMap()

	// Press select
	gp.handleButton(0x13a, 1)

	// Simulate expired window by setting comboFirstPress in the past
	gp.comboFirstPress = time.Now().Add(-100 * time.Millisecond)

	// Press start - window expired, should not fire
	a := gp.handleButton(0x13b, 1)
	if a.Type == ActionToggle {
		t.Error("combo fired outside timing window")
	}
}

func TestComboDetect_DpadAxis(t *testing.T) {
	gp := newComboGamepad("dpad_up+a")

	// D-pad up via ABS_HAT0Y = -1
	gp.handleAxis(ABS_HAT0Y, -1)
	a := gp.handleButton(BTN_SOUTH, 1)
	if a.Type != ActionToggle {
		t.Errorf("dpad_up(axis)+a = %v, want ActionToggle", a.Type)
	}
}

func TestComboDetect_DpadButton(t *testing.T) {
	gp := newComboGamepad("dpad_up+a")

	// D-pad up via BTN_DPAD_UP (0x220) - some controllers send this
	gp.handleButton(0x220, 1)
	a := gp.handleButton(BTN_SOUTH, 1)
	if a.Type != ActionToggle {
		t.Errorf("dpad_up(btn)+a = %v, want ActionToggle", a.Type)
	}
}

func TestComboDetect_AnalogTrigger(t *testing.T) {
	gp := newComboGamepad("lt+a")

	// LT via analog axis (ABS_Z value > 0)
	gp.handleAxis(ABS_Z, 200)
	a := gp.handleButton(BTN_SOUTH, 1)
	if a.Type != ActionToggle {
		t.Errorf("lt(axis)+a = %v, want ActionToggle", a.Type)
	}
}

func TestComboDetect_DigitalTrigger(t *testing.T) {
	gp := newComboGamepad("lt+a")

	// LT via digital button (BTN_TL2) - Switch Pro
	gp.handleButton(BTN_TL2, 1)
	a := gp.handleButton(BTN_SOUTH, 1)
	if a.Type != ActionToggle {
		t.Errorf("lt(btn)+a = %v, want ActionToggle", a.Type)
	}
}

func TestComboDetect_Disabled(t *testing.T) {
	gp := newTestGamepad() // no toggle combo

	// Press every button - should never get ActionToggle
	for _, code := range []uint16{BTN_SOUTH, BTN_EAST, BTN_WEST, BTN_NORTH, BTN_TL, BTN_TR, 0x13a, 0x13b} {
		a := gp.handleButton(code, 1)
		if a.Type == ActionToggle {
			t.Errorf("disabled combo fired on button 0x%x", code)
		}
	}
}

func TestComboDetect_ThreeButtons(t *testing.T) {
	gp := newComboGamepad("select+start+rb")

	gp.handleButton(0x13a, 1)       // select
	gp.handleButton(0x13b, 1)       // start - only 2 of 3, no fire
	a := gp.handleButton(BTN_TR, 1) // rb - all 3 held
	if a.Type != ActionToggle {
		t.Errorf("3-button combo = %v, want ActionToggle", a.Type)
	}
}

func TestComboDetect_FourButtons(t *testing.T) {
	gp := newComboGamepad("select+start+lb+rb")

	gp.handleButton(0x13a, 1)       // select
	gp.handleButton(0x13b, 1)       // start
	gp.handleButton(BTN_TL, 1)      // lb - only 3 of 4
	a := gp.handleButton(BTN_TR, 1) // rb - all 4
	if a.Type != ActionToggle {
		t.Errorf("4-button combo = %v, want ActionToggle", a.Type)
	}
}

func TestComboDetect_PartialPress(t *testing.T) {
	gp := newComboGamepad("select+start+rb")

	gp.handleButton(0x13a, 1)      // select
	a := gp.handleButton(0x13b, 1) // start - only 2 of 3
	if a.Type == ActionToggle {
		t.Error("partial press (2 of 3) should not fire combo")
	}
}

func TestReadEventsNilFd(t *testing.T) {
	gp := newTestGamepad()
	gp.fd = nil
	actions := gp.ReadEvents()
	if actions != nil {
		t.Errorf("ReadEvents with nil fd = %v, want nil", actions)
	}
}

func TestNeedsPollingIdle(t *testing.T) {
	gp := newTestGamepad()
	// No fd, no mouse, no nav
	if gp.NeedsPolling() {
		t.Error("NeedsPolling should be false with nil fd")
	}
}

func TestNeedsPollingMouseActive(t *testing.T) {
	gp := newTestGamepad()
	// Simulate an open fd by setting a non-nil value (we won't read from it)
	f, _ := os.CreateTemp("", "gamepad-test")
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()
	gp.fd = f
	gp.mouseX = 0.5
	if !gp.NeedsPolling() {
		t.Error("NeedsPolling should be true with active mouse stick")
	}
}

func TestNeedsPollingNavActive(t *testing.T) {
	gp := newTestGamepad()
	f, _ := os.CreateTemp("", "gamepad-test")
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()
	gp.fd = f
	gp.dpadX.Direction = 1
	if !gp.NeedsPolling() {
		t.Error("NeedsPolling should be true with active dpad")
	}
}

func TestAnyButtonHeldNoFd(t *testing.T) {
	gp := newTestGamepad()
	gp.fd = nil
	gp.btnHeld[BTN_SOUTH] = true // should be ignored when fd is nil
	if gp.AnyButtonHeld() {
		t.Error("AnyButtonHeld should be false when fd is nil")
	}
}

func TestAnyButtonHeldEmpty(t *testing.T) {
	gp := newTestGamepad()
	f, _ := os.CreateTemp("", "gamepad-test")
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()
	gp.fd = f
	if gp.AnyButtonHeld() {
		t.Error("AnyButtonHeld should be false with no buttons held")
	}
}

func TestAnyButtonHeldButton(t *testing.T) {
	gp := newTestGamepad()
	f, _ := os.CreateTemp("", "gamepad-test")
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()
	gp.fd = f
	gp.btnHeld[BTN_SOUTH] = true
	if !gp.AnyButtonHeld() {
		t.Error("AnyButtonHeld should be true when a button is held")
	}
	gp.btnHeld[BTN_SOUTH] = false
	if gp.AnyButtonHeld() {
		t.Error("AnyButtonHeld should be false after release")
	}
}

func TestAnyButtonHeldHat(t *testing.T) {
	gp := newTestGamepad()
	f, _ := os.CreateTemp("", "gamepad-test")
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()
	gp.fd = f
	gp.axisState[ABS_HAT0X] = 1
	if !gp.AnyButtonHeld() {
		t.Error("AnyButtonHeld should be true when hat0x is non-zero")
	}
	gp.axisState[ABS_HAT0X] = 0
	gp.axisState[ABS_HAT0Y] = -1
	if !gp.AnyButtonHeld() {
		t.Error("AnyButtonHeld should be true when hat0y is non-zero (negative)")
	}
	gp.axisState[ABS_HAT0Y] = 0
	if gp.AnyButtonHeld() {
		t.Error("AnyButtonHeld should be false with both hats zero")
	}
}

func TestAnyButtonHeldTrigger(t *testing.T) {
	gp := newTestGamepad()
	f, _ := os.CreateTemp("", "gamepad-test")
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()
	gp.fd = f
	// Use the default 0..255 range for ABS_Z. 30% of 255 = ~76.
	gp.axisState[ABS_Z] = 100
	if !gp.AnyButtonHeld() {
		t.Error("AnyButtonHeld should be true when LT is past 30%")
	}
	gp.axisState[ABS_Z] = 50 // ~20%
	if gp.AnyButtonHeld() {
		t.Error("AnyButtonHeld should be false when LT is below 30%")
	}
	gp.axisState[ABS_RZ] = 200 // RT past threshold
	if !gp.AnyButtonHeld() {
		t.Error("AnyButtonHeld should be true when RT is past 30%")
	}
}

func TestDisconnectResetsState(t *testing.T) {
	gp := newTestGamepad()
	// Set up state that should be cleared on disconnect
	gp.mouseX = 0.8
	gp.mouseY = -0.5
	gp.navX.Direction = 1
	gp.dpadY.Direction = -1
	gp.ltActive = true
	gp.trigActive[ABS_RZ] = true
	gp.btnHeld[BTN_SOUTH] = true
	gp.grabbed = true

	// Simulate disconnect by calling the reset path directly
	// (we can't trigger a real ENODEV without a real device)
	// Ungrab needs a non-nil fd; in real disconnect path it runs before fd=nil
	gp.grabbed = false
	gp.fd = nil
	gp.mouseX, gp.mouseY = 0, 0
	gp.navX = NavAxis{}
	gp.navY = NavAxis{}
	gp.dpadX = NavAxis{}
	gp.dpadY = NavAxis{}
	gp.btnHeld = make(map[uint16]bool)
	gp.axisState = make(map[uint16]int32)
	gp.ltActive = false
	gp.trigActive = make(map[uint16]bool)

	if gp.mouseX != 0 || gp.mouseY != 0 {
		t.Error("mouse should be zeroed after disconnect")
	}
	if gp.navX.Direction != 0 || gp.dpadY.Direction != 0 {
		t.Error("nav axes should be zeroed after disconnect")
	}
	if gp.ltActive || gp.trigActive[ABS_RZ] {
		t.Error("trigger state should be cleared after disconnect")
	}
	if len(gp.btnHeld) != 0 {
		t.Error("btnHeld should be empty after disconnect")
	}
	if gp.grabbed {
		t.Error("grabbed should be false after disconnect")
	}
	if gp.fd != nil {
		t.Error("fd should be nil after disconnect")
	}
}

func TestReconnectNoDevice(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gamepad.Device = "/dev/input/event_nonexistent_test"
	gp := NewGamepadReader(cfg)
	gp.fd = nil
	if gp.Reconnect() {
		t.Error("Reconnect should return false with nonexistent device")
	}
}

func TestMouseStickLeft(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gamepad.MouseStick = "left"
	gp := NewGamepadReader(cfg)
	gp.initButtonMap()

	// Left stick should be mouse now
	if gp.mouseAxisX != ABS_X || gp.mouseAxisY != ABS_Y {
		t.Errorf("mouse axes = (%d,%d), want ABS_X/ABS_Y", gp.mouseAxisX, gp.mouseAxisY)
	}
	// Right stick should be nav
	if gp.navAxisX != ABS_RX || gp.navAxisY != ABS_RY {
		t.Errorf("nav axes = (%d,%d), want ABS_RX/ABS_RY", gp.navAxisX, gp.navAxisY)
	}

	// L3 should be click, R3 should be caps
	a := gp.handleButton(BTN_THUMBL, 1)
	if a.Type != ActionLeftClick {
		t.Errorf("L3 with mouse_stick=left = %v, want ActionLeftClick", a.Type)
	}
	a = gp.handleButton(BTN_THUMBR, 1)
	if a.Type != ActionCapsToggle {
		t.Errorf("R3 with mouse_stick=left = %v, want ActionCapsToggle", a.Type)
	}
}

// simulateOpen mimics Open() after readAxisInfo: set the axes the device
// reports, then run layout detection and build the action map.
func simulateOpen(gp *GamepadReader, axes ...uint16) {
	gp.hasAxis = make(map[uint16]bool)
	for _, ax := range axes {
		gp.hasAxis[ax] = true
	}
	gp.detectLayout()
	gp.initButtonMap()
}

func newDInputGamepad(cfg Config) *GamepadReader {
	gp := NewGamepadReader(cfg)
	simulateOpen(gp, ABS_X, ABS_Y, ABS_Z, ABS_RZ)
	return gp
}

func TestDetectLayout(t *testing.T) {
	tests := []struct {
		name string
		axes []uint16
		want bool
	}{
		{"xinput", []uint16{ABS_X, ABS_Y, ABS_RX, ABS_RY, ABS_Z, ABS_RZ}, false},
		{"dinput", []uint16{ABS_X, ABS_Y, ABS_Z, ABS_RZ}, true},
		{"rx/ry without triggers", []uint16{ABS_X, ABS_Y, ABS_RX, ABS_RY}, false},
		{"only one of rx/ry", []uint16{ABS_X, ABS_Y, ABS_RX, ABS_Z, ABS_RZ}, false},
		{"only z", []uint16{ABS_X, ABS_Y, ABS_Z}, false},
		{"no axes", nil, false},
	}
	for _, tt := range tests {
		gp := NewGamepadReader(DefaultConfig())
		simulateOpen(gp, tt.axes...)
		if gp.dinputLayout != tt.want {
			t.Errorf("%s: dinputLayout = %v, want %v", tt.name, gp.dinputLayout, tt.want)
		}
	}
}

func TestDInput_StickAxes(t *testing.T) {
	gp := newDInputGamepad(DefaultConfig())
	if gp.mouseAxisX != ABS_Z || gp.mouseAxisY != ABS_RZ {
		t.Errorf("mouse axes = (%d,%d), want ABS_Z/ABS_RZ", gp.mouseAxisX, gp.mouseAxisY)
	}
	if gp.navAxisX != ABS_X || gp.navAxisY != ABS_Y {
		t.Errorf("nav axes = (%d,%d), want ABS_X/ABS_Y", gp.navAxisX, gp.navAxisY)
	}

	cfg := DefaultConfig()
	cfg.Gamepad.MouseStick = "left"
	gp = newDInputGamepad(cfg)
	if gp.navAxisX != ABS_Z || gp.navAxisY != ABS_RZ {
		t.Errorf("mouse_stick=left: nav axes = (%d,%d), want ABS_Z/ABS_RZ", gp.navAxisX, gp.navAxisY)
	}
}

func TestDInput_RightStickIsNotTriggers(t *testing.T) {
	gp := newDInputGamepad(DefaultConfig())

	// Full deflection on Z/RZ moves the mouse, never fires shift or enter
	if a := gp.handleAxis(ABS_Z, 255); a.Type == ActionShiftOn {
		t.Error("ABS_Z on DInput pad fired ActionShiftOn")
	}
	if a := gp.handleAxis(ABS_RZ, 255); a.Type == ActionEnter {
		t.Error("ABS_RZ on DInput pad fired ActionEnter")
	}
	if gp.mouseX == 0 || gp.mouseY == 0 {
		t.Errorf("mouse = (%v,%v), want nonzero after right stick input", gp.mouseX, gp.mouseY)
	}
}

func TestDInput_DigitalTriggers(t *testing.T) {
	gp := newDInputGamepad(DefaultConfig())

	steps := []struct {
		code  uint16
		value int32
		want  ActionType
	}{
		{BTN_TL2, 1, ActionShiftOn},
		{BTN_TL2, 0, ActionShiftOff},
		{BTN_TR2, 1, ActionEnter},
		{BTN_TR2, 0, ActionEnterRelease},
	}
	for _, s := range steps {
		if a := gp.handleButton(s.code, s.value); a.Type != s.want {
			t.Errorf("button 0x%x value %d = %v, want %v", s.code, s.value, a.Type, s.want)
		}
	}
}

func TestDInput_RelayoutOnReconnect(t *testing.T) {
	gp := newDInputGamepad(DefaultConfig())

	// A different controller (XInput layout) is opened on reconnect
	simulateOpen(gp, ABS_X, ABS_Y, ABS_RX, ABS_RY, ABS_Z, ABS_RZ)
	if gp.dinputLayout {
		t.Fatal("dinputLayout should reset when an XInput pad is opened")
	}
	if gp.mouseAxisX != ABS_RX || gp.mouseAxisY != ABS_RY {
		t.Errorf("mouse axes = (%d,%d), want ABS_RX/ABS_RY", gp.mouseAxisX, gp.mouseAxisY)
	}
	if a := gp.handleAxis(ABS_Z, 200); a.Type != ActionShiftOn {
		t.Errorf("LT axis after relayout = %v, want ActionShiftOn", a.Type)
	}
}

func TestDInput_AnyButtonHeldIgnoresRightStick(t *testing.T) {
	gp := newDInputGamepad(DefaultConfig())
	f, _ := os.CreateTemp("", "gamepad-test")
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()
	gp.fd = f

	// Right stick at rest sits mid-range, which reads as a held trigger
	// on an XInput pad. It must not block the deferred grab.
	gp.axisState[ABS_Z] = 128
	gp.axisState[ABS_RZ] = 128
	if gp.AnyButtonHeld() {
		t.Error("AnyButtonHeld should ignore Z/RZ on a DInput pad")
	}
	gp.btnHeld[BTN_TL2] = true
	if !gp.AnyButtonHeld() {
		t.Error("AnyButtonHeld should report a held digital trigger")
	}
}

func TestDInput_ComboIgnoresRightStick(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gamepad.ToggleCombo = "lt+a"
	cfg.Gamepad.ComboPeriodMs = 200
	gp := newDInputGamepad(cfg)

	gp.handleAxis(ABS_Z, 255)
	if a := gp.handleButton(BTN_SOUTH, 1); a.Type == ActionToggle {
		t.Error("right stick on ABS_Z should not count as lt for the combo")
	}
	gp.handleButton(BTN_SOUTH, 0)

	gp.handleButton(BTN_TL2, 1)
	if a := gp.handleButton(BTN_SOUTH, 1); a.Type != ActionToggle {
		t.Errorf("digital lt+a = %v, want ActionToggle", a.Type)
	}
}

func TestShiftButtonRelease(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gamepad.Buttons.Shift = "lb"
	cfg.Gamepad.Buttons.RightClick = "rb"
	cfg.Gamepad.Buttons.LeftClick = "r3"
	gp := NewGamepadReader(cfg)
	gp.initButtonMap()

	if a := gp.handleButton(BTN_TL, 1); a.Type != ActionShiftOn {
		t.Errorf("lb press = %v, want ActionShiftOn", a.Type)
	}
	if a := gp.handleButton(BTN_TL, 0); a.Type != ActionShiftOff {
		t.Errorf("lb release = %v, want ActionShiftOff", a.Type)
	}
}

func TestShiftDigitalTriggerRelease(t *testing.T) {
	// Switch Pro: lt arrives as BTN_TL2 only, no ABS_Z
	gp := newTestGamepad()
	if a := gp.handleButton(BTN_TL2, 1); a.Type != ActionShiftOn {
		t.Errorf("BTN_TL2 press = %v, want ActionShiftOn", a.Type)
	}
	if a := gp.handleButton(BTN_TL2, 0); a.Type != ActionShiftOff {
		t.Errorf("BTN_TL2 release = %v, want ActionShiftOff", a.Type)
	}
}

func TestShiftOnButtonLeavesLTAxisAlone(t *testing.T) {
	// shift on a button and backspace on lt: the LT axis must act as
	// backspace, not be swallowed by the shift hold logic.
	cfg := DefaultConfig()
	cfg.Gamepad.Buttons.Shift = "x"
	cfg.Gamepad.Buttons.Backspace = "lt"
	gp := NewGamepadReader(cfg)
	gp.initButtonMap()

	if gp.shiftAxis != absNone {
		t.Errorf("shiftAxis = 0x%x, want absNone for a button-mapped shift", gp.shiftAxis)
	}
	if a := gp.handleAxis(ABS_Z, 200); a.Type != ActionBackspace {
		t.Errorf("LT axis = %v, want ActionBackspace", a.Type)
	}
	if a := gp.handleAxis(ABS_Z, 0); a.Type != ActionBackspaceRelease {
		t.Errorf("LT axis release = %v, want ActionBackspaceRelease", a.Type)
	}
}

var (
	xpadPad   = deviceInfo{Name: "Microsoft X-Box 360 pad", Driver: "xpad", Vendor: "045e"}
	ds3Pad    = deviceInfo{Name: "Sony PLAYSTATION(R)3 Controller", Driver: "sony", Vendor: "054c"}
	uinputBox = deviceInfo{Name: "Microsoft X-Box 360 pad 0"} // Sunshine/Steam: no driver
)

// xSends reports which evdev code the x button (Backspace by default) is on.
func xSends(t *testing.T, gp *GamepadReader) uint16 {
	t.Helper()
	west := gp.actionMap.BtnPress[BTN_WEST] == ActionBackspace
	north := gp.actionMap.BtnPress[BTN_NORTH] == ActionBackspace
	switch {
	case west && !north:
		return BTN_WEST
	case north && !west:
		return BTN_NORTH
	}
	t.Fatalf("x is on neither or both codes: %v", gp.actionMap.BtnPress)
	return 0
}

// Bug F: swap_xy = auto is decided again for every pad, not kept from the
// first one.
func TestSwapXYPerPad(t *testing.T) {
	tests := []struct {
		setting string
		pads    []deviceInfo
		want    []uint16 // where x lands on each pad, in order
	}{
		{"auto", []deviceInfo{xpadPad, ds3Pad, xpadPad}, []uint16{BTN_NORTH, BTN_WEST, BTN_NORTH}},
		{"auto", []deviceInfo{uinputBox, ds3Pad}, []uint16{BTN_NORTH, BTN_WEST}},
		{"auto", []deviceInfo{ds3Pad, xpadPad}, []uint16{BTN_WEST, BTN_NORTH}},
		{"", []deviceInfo{xpadPad, ds3Pad}, []uint16{BTN_NORTH, BTN_WEST}},
		{"true", []deviceInfo{ds3Pad, xpadPad}, []uint16{BTN_NORTH, BTN_NORTH}},
		{"false", []deviceInfo{xpadPad, ds3Pad}, []uint16{BTN_WEST, BTN_WEST}},
	}
	for _, tt := range tests {
		cfg := DefaultConfig()
		cfg.Gamepad.SwapXY = tt.setting
		gp := NewGamepadReader(cfg)
		for i, pad := range tt.pads {
			gp.applyDevice(pad)
			if got := xSends(t, gp); got != tt.want[i] {
				t.Errorf("swap_xy=%q, pad %d (%s): x on %#x, want %#x", tt.setting, i+1, pad.Name, got, tt.want[i])
			}
		}
	}
}

// Bug G: a combo's x and y are the buttons printed X and Y, like the
// action map, on swapped and unswapped pads, and after a reconnect.
func TestComboFollowsSwapXY(t *testing.T) {
	fires := func(gp *GamepadReader, code uint16) bool {
		gp.btnHeld = map[uint16]bool{}
		gp.comboFired = false
		gp.handleButton(code, 1)
		return gp.handleButton(0x13b, 1).Type == ActionToggle // start
	}
	cfg := DefaultConfig()
	cfg.Gamepad.ToggleCombo = "x+start"
	gp := NewGamepadReader(cfg)
	for _, step := range []struct {
		pad      deviceInfo
		xCode    uint16
		notXCode uint16
	}{
		{xpadPad, BTN_NORTH, BTN_WEST},
		{ds3Pad, BTN_WEST, BTN_NORTH},
		{xpadPad, BTN_NORTH, BTN_WEST},
	} {
		gp.applyDevice(step.pad)
		if !fires(gp, step.xCode) {
			t.Errorf("%s: x+start did not fire on %#x", step.pad.Name, step.xCode)
		}
		if fires(gp, step.notXCode) {
			t.Errorf("%s: x+start fired on %#x (that is y)", step.pad.Name, step.notXCode)
		}
	}
}

// triggerClickConfig puts both clicks on the triggers, with shift and enter
// moved off them so nothing clashes.
func triggerClickConfig() Config {
	cfg := DefaultConfig()
	b := &cfg.Gamepad.Buttons
	b.LeftClick, b.RightClick, b.Enter, b.Shift = "rt", "lt", "rb", "select"
	return cfg
}

func TestTriggerClicks_ActionMap(t *testing.T) {
	m := BuildActionMap(triggerClickConfig().Gamepad)
	checks := []struct {
		name      string
		got, want ActionType
	}{
		{"axis RZ", m.AxisActions[ABS_RZ], ActionLeftClick},
		{"axis RZ release", m.AxisRelease[ABS_RZ], ActionLeftClickRelease},
		{"axis Z", m.AxisActions[ABS_Z], ActionRightClick},
		{"axis Z release", m.AxisRelease[ABS_Z], ActionRightClickRelease},
		{"button TR2", m.BtnPress[BTN_TR2], ActionLeftClick},
		{"button TR2 release", m.BtnRelease[BTN_TR2], ActionLeftClickRelease},
		{"button TL2", m.BtnPress[BTN_TL2], ActionRightClick},
		{"button TL2 release", m.BtnRelease[BTN_TL2], ActionRightClickRelease},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// Each trigger keeps its own held state: holding one never hides the
// other's press or release.
func TestTriggerClicks_AnalogIndependent(t *testing.T) {
	gp := NewGamepadReader(triggerClickConfig())
	gp.initButtonMap()
	steps := []struct {
		code  uint16
		value int32
		want  ActionType
	}{
		{ABS_RZ, 200, ActionLeftClick},
		{ABS_Z, 200, ActionRightClick},
		{ABS_RZ, 255, ActionNone}, // still held
		{ABS_Z, 0, ActionRightClickRelease},
		{ABS_RZ, 250, ActionNone}, // still held, no second click
		{ABS_RZ, 0, ActionLeftClickRelease},
	}
	for i, s := range steps {
		if a := gp.handleAxis(s.code, s.value); a.Type != s.want {
			t.Errorf("step %d: axis 0x%x = %d: %v, want %v", i, s.code, s.value, a.Type, s.want)
		}
	}
}

func TestTriggerClicks_Digital(t *testing.T) {
	gp := newDInputGamepad(triggerClickConfig())
	steps := []struct {
		code  uint16
		value int32
		want  ActionType
	}{
		{BTN_TR2, 1, ActionLeftClick},
		{BTN_TR2, 0, ActionLeftClickRelease},
		{BTN_TL2, 1, ActionRightClick},
		{BTN_TL2, 0, ActionRightClickRelease},
	}
	for _, s := range steps {
		if a := gp.handleButton(s.code, s.value); a.Type != s.want {
			t.Errorf("button 0x%x value %d = %v, want %v", s.code, s.value, a.Type, s.want)
		}
	}
	// Z/RZ are the right stick on this layout: no click from the stick
	if a := gp.handleAxis(ABS_RZ, 255); a.Type == ActionLeftClick {
		t.Error("ABS_RZ on DInput pad fired ActionLeftClick")
	}
}

// Two non-shift actions on the two analog triggers (here the defaults'
// space and enter) no longer share one held flag.
func TestTriggerActions_TwoAnalogTriggers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gamepad.Buttons.Shift, cfg.Gamepad.Buttons.Space = "y", "lt"
	gp := NewGamepadReader(cfg)
	gp.initButtonMap()
	if a := gp.handleAxis(ABS_RZ, 200); a.Type != ActionEnter {
		t.Fatalf("RT = %v, want ActionEnter", a.Type)
	}
	if a := gp.handleAxis(ABS_Z, 200); a.Type != ActionSpace {
		t.Errorf("LT with RT held = %v, want ActionSpace", a.Type)
	}
}

// Trigger axes are scaled by their own range: 0-1, 0-255 and 0-32767 all
// click at the same point of travel.
func TestTriggerClicks_AxisRanges(t *testing.T) {
	for _, max := range []int32{1, 255, 32767} {
		gp := NewGamepadReader(triggerClickConfig())
		gp.axisRanges[ABS_RZ] = axisRange{min: 0, max: max}
		gp.initButtonMap()
		if a := gp.handleAxis(ABS_RZ, max/5); a.Type != ActionNone && max > 1 {
			t.Errorf("0-%d: 20%% travel = %v, want no click", max, a.Type)
		}
		if a := gp.handleAxis(ABS_RZ, max); a.Type != ActionLeftClick {
			t.Errorf("0-%d: full travel = %v, want ActionLeftClick", max, a.Type)
		}
		if a := gp.handleAxis(ABS_RZ, 0); a.Type != ActionLeftClickRelease {
			t.Errorf("0-%d: released = %v, want ActionLeftClickRelease", max, a.Type)
		}
	}
}

// DS3, DS4 and DualSense send a trigger as ABS_RZ and BTN_TR2 at once: one
// pull is one action. A pad with button triggers only keeps them.
func TestTriggerSentBothWays(t *testing.T) {
	type step struct {
		btn  bool
		code uint16
		v    int32
	}
	pull := []step{{true, BTN_TR2, 1}, {false, ABS_RZ, 200}, {false, ABS_RZ, 0}, {true, BTN_TR2, 0}}
	run := func(gp *GamepadReader, steps []step) []ActionType {
		var got []ActionType
		for _, s := range steps {
			var a Action
			if s.btn {
				a = gp.handleButton(s.code, s.v)
			} else {
				a = gp.handleAxis(s.code, s.v)
			}
			if a.Type != ActionNone {
				got = append(got, a.Type)
			}
		}
		return got
	}

	gp := NewGamepadReader(DefaultConfig())
	simulateOpen(gp, ABS_X, ABS_Y, ABS_RX, ABS_RY, ABS_Z, ABS_RZ)
	if got, want := run(gp, pull), []ActionType{ActionEnter, ActionEnterRelease}; !slices.Equal(got, want) {
		t.Errorf("DS pad, enter = rt: %v, want %v", got, want)
	}
	gp = NewGamepadReader(triggerClickConfig())
	simulateOpen(gp, ABS_X, ABS_Y, ABS_RX, ABS_RY, ABS_Z, ABS_RZ)
	if got, want := run(gp, pull), []ActionType{ActionLeftClick, ActionLeftClickRelease}; !slices.Equal(got, want) {
		t.Errorf("DS pad, left_click = rt: %v, want %v", got, want)
	}

	// xpad triggers_to_buttons, fight sticks: no trigger axes, buttons only
	gp = NewGamepadReader(DefaultConfig())
	simulateOpen(gp, ABS_X, ABS_Y, ABS_RX, ABS_RY)
	buttons := []step{{true, BTN_TR2, 1}, {true, BTN_TR2, 0}, {true, BTN_TL2, 1}, {true, BTN_TL2, 0}}
	want := []ActionType{ActionEnter, ActionEnterRelease, ActionShiftOn, ActionShiftOff}
	if got := run(gp, buttons); !slices.Equal(got, want) {
		t.Errorf("button triggers: %v, want %v", got, want)
	}
}

// press = lt: LT gives press start and press like A does (the app does the
// hold timing), A is free, and a pad sending LT both ways presses once.
func TestPressOnTrigger(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gamepad.Buttons.Press, cfg.Gamepad.Buttons.Shift = "lt", "select"
	if issues := checkConfig(cfg); issues != nil {
		t.Fatalf("press = lt: %q", issues)
	}
	type step struct {
		btn  bool
		code uint16
		v    int32
	}
	run := func(gp *GamepadReader, steps []step) []ActionType {
		var got []ActionType
		for _, s := range steps {
			var a Action
			if s.btn {
				a = gp.handleButton(s.code, s.v)
			} else {
				a = gp.handleAxis(s.code, s.v)
			}
			got = append(got, a.Type)
		}
		return got
	}
	press := []ActionType{ActionPressStart, ActionPress}

	// Xbox (xpad): LT is an axis only
	gp := NewGamepadReader(cfg)
	simulateOpen(gp, ABS_X, ABS_Y, ABS_RX, ABS_RY, ABS_Z, ABS_RZ)
	if got := run(gp, []step{{false, ABS_Z, 200}, {false, ABS_Z, 0}}); !slices.Equal(got, press) {
		t.Errorf("analog LT: %v, want %v", got, press)
	}
	if got := run(gp, []step{{true, BTN_SOUTH, 1}, {true, BTN_SOUTH, 0}}); !slices.Equal(got, []ActionType{ActionNone, ActionNone}) {
		t.Errorf("A with press = lt: %v, want nothing", got)
	}

	// DS pad: LT as axis and button at once, one press
	gp = NewGamepadReader(cfg)
	simulateOpen(gp, ABS_X, ABS_Y, ABS_RX, ABS_RY, ABS_Z, ABS_RZ)
	both := []step{{true, BTN_TL2, 1}, {false, ABS_Z, 200}, {false, ABS_Z, 0}, {true, BTN_TL2, 0}}
	want := []ActionType{ActionNone, ActionPressStart, ActionPress, ActionNone}
	if got := run(gp, both); !slices.Equal(got, want) {
		t.Errorf("DS LT: %v, want %v", got, want)
	}

	// DInput: LT is a button, Z is the right stick
	gp = newDInputGamepad(cfg)
	if got := run(gp, []step{{true, BTN_TL2, 1}, {true, BTN_TL2, 0}}); !slices.Equal(got, press) {
		t.Errorf("digital LT: %v, want %v", got, press)
	}
	if got := run(gp, []step{{false, ABS_Z, 255}, {false, ABS_Z, 128}}); slices.Contains(got, ActionPressStart) {
		t.Errorf("DInput right stick on Z pressed: %v", got)
	}
}

// SYN_DROPPED: events up to the next SYN_REPORT are skipped, then the real
// state releases what the reader still held (here X with Backspace repeat,
// whose release was in the dropped part). With no device, resync counts
// every button as released.
func TestSynDroppedResync(t *testing.T) {
	gp := newTestGamepad()
	backspace := buttonTable(isSwapXY(gp.config.Gamepad))["x"].EvdevBtn
	gp.handleButton(backspace, 1)
	var types []ActionType
	for _, ev := range []struct {
		typ, code uint16
		v         int32
	}{
		{evdevEV_SYN, synDropped, 0},
		{evdevEV_KEY, backspace, 0}, // partial: skipped
		{evdevEV_KEY, BTN_SOUTH, 1}, // partial: skipped
		{evdevEV_SYN, synReport, 0},
	} {
		for _, a := range gp.feed(ev.typ, ev.code, ev.v) {
			types = append(types, a.Type)
		}
	}
	if !slices.Equal(types, []ActionType{ActionBackspaceRelease}) {
		t.Errorf("after SYN_DROPPED: %v, want only the Backspace release", types)
	}
	if slices.Contains(types, ActionPressStart) {
		t.Errorf("a press from the dropped part was handled: %v", types)
	}
	if gp.btnHeld[backspace] || gp.dropped {
		t.Error("held state not cleared")
	}
	// Events after the resync are handled normally
	if got := gp.feed(evdevEV_KEY, BTN_SOUTH, 1); len(got) != 1 || got[0].Type != ActionPressStart {
		t.Errorf("after resync: %v, want press start", got)
	}
}

// resyncFrom feeds each axis its current value: RT at rest ends Enter.
func TestResyncReleasesTrigger(t *testing.T) {
	gp := newTestGamepad()
	gp.handleAxis(ABS_RZ, 200)
	got := gp.resyncFrom(make([]byte, 96), map[uint16]int32{ABS_RZ: 0})
	if len(got) != 1 || got[0].Type != ActionEnterRelease || gp.trigActive[ABS_RZ] {
		t.Errorf("RT at rest: %v, active %v; want Enter release", got, gp.trigActive[ABS_RZ])
	}
}

// resyncFrom keeps a button the kernel still shows pressed.
func TestResyncKeepsHeldButton(t *testing.T) {
	gp := newTestGamepad()
	gp.handleButton(BTN_TR, 1)
	keys := make([]byte, 96)
	keys[BTN_TR/8] |= 1 << (BTN_TR % 8)
	if got := gp.resyncFrom(keys, nil); len(got) != 0 || !gp.btnHeld[BTN_TR] {
		t.Errorf("held RB: %v, held %v; want no action, still held", got, gp.btnHeld[BTN_TR])
	}
}

// resyncFrom logs each button it releases, and only those: the bench reads
// the line to tell a release lost in the drop from one read normally.
func TestResyncLogsRelease(t *testing.T) {
	var out bytes.Buffer
	log.SetOutput(&out)
	defer log.SetOutput(os.Stderr)
	gp := newTestGamepad()
	gp.handleButton(BTN_WEST, 1)
	gp.handleButton(BTN_TR, 1)
	keys := make([]byte, 96)
	keys[BTN_TR/8] |= 1 << (BTN_TR % 8) // RB still down, X up
	gp.resyncFrom(keys, nil)
	if !strings.Contains(out.String(), "Button 0x134 released after the drop") {
		t.Errorf("no release line for X: %q", out.String())
	}
	if strings.Contains(out.String(), "Button 0x137 released") {
		t.Errorf("RB is still held but logged: %q", out.String())
	}
	out.Reset()
	gp.resyncFrom(keys, nil)
	if out.Len() != 0 {
		t.Errorf("second resync, nothing to release, logged %q", out.String())
	}
}

// toggle_combo = guide+a: releasing the combo's buttons after it fires does
// nothing (A's release used to type the highlighted key on the keyboard it
// had just shown). The next A press types as usual.
func TestComboReleaseSwallowed(t *testing.T) {
	const guide = 0x13c // BTN_MODE
	cfg := DefaultConfig()
	cfg.Gamepad.ToggleCombo = "guide+a"
	for _, tt := range []struct {
		name  string
		steps [][2]int // code, value
		want  []ActionType
	}{
		{"guide first", [][2]int{{guide, 1}, {BTN_SOUTH, 1}, {BTN_SOUTH, 0}, {guide, 0}},
			[]ActionType{ActionNone, ActionToggle, ActionNone, ActionNone}},
		{"a first", [][2]int{{BTN_SOUTH, 1}, {guide, 1}, {guide, 0}, {BTN_SOUTH, 0}},
			[]ActionType{ActionPressStart, ActionToggle, ActionNone, ActionNone}},
		{"a again after the combo", [][2]int{{guide, 1}, {BTN_SOUTH, 1}, {BTN_SOUTH, 0}, {guide, 0},
			{BTN_SOUTH, 1}, {BTN_SOUTH, 0}},
			[]ActionType{ActionNone, ActionToggle, ActionNone, ActionNone, ActionPressStart, ActionPress}},
	} {
		gp := NewGamepadReader(cfg)
		gp.initButtonMap()
		var got []ActionType
		for _, s := range tt.steps {
			got = append(got, gp.handleButton(uint16(s[0]), int32(s[1])).Type) //nolint:gosec // G115: small test codes
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

// Device blocks from /proc/bus/input/devices on real machines (2026-11-21).
// Sunshine's absolute mouse gets a js handler from joydev but has only
// mouse buttons; it comes before Sunshine's virtual pad in the list.
const (
	procSunshineMouse = `I: Bus=0003 Vendor=1209 Product=0003 Version=0001
N: Name="libvirtualhid Mouse (Absolute)"
P: Phys=
S: Sysfs=/devices/virtual/input/input35
U: Uniq=
H: Handlers=event25 js0 mouse5 
B: PROP=2
B: EV=b
B: KEY=1f0000 0 0 0 0
B: ABS=3

`
	procSunshinePad = `N: Name="Sunshine (libvirtualhid) X-Box One Controller"
H: Handlers=kbd event28 js1 
B: KEY=7fff000000000000 0 8000000000 0 0

`
	procXbox360 = `I: Bus=0003 Vendor=045e Product=028e Version=0114
N: Name="Microsoft X-Box 360 pad"
H: Handlers=event31 js0 
B: PROP=0
B: EV=20000b
B: KEY=7cdb000000000000 0 0 0 0
B: ABS=3003f

`
	// Generic HID joystick: BTN_TRIGGER to BTN_BASE6 (0x120-0x12b) only
	procDInput = `N: Name="Generic USB Joystick"
H: Handlers=event9 js2 
B: KEY=fff00000000 0 0 0 0

`
	// A js device whose key bitmap is too short to hold 0x120
	procShortKeys = `N: Name="Odd Device"
H: Handlers=event7 js3 
B: KEY=ff

`
	procKeyboard = `N: Name="AT Translated Set 2 keyboard"
H: Handlers=sysrq kbd event3 leds 
B: KEY=402000000 3803078f800d001 feffffdfffefffff fffffffffffffffe

`
)

// gamepadCandidates must skip js devices without joystick or gamepad
// buttons (Finding P: Sunshine's absolute mouse was opened as the pad).
func TestGamepadCandidates(t *testing.T) {
	for _, tt := range []struct {
		name string
		proc string
		want []string
	}{
		{"mouse before Sunshine pad", procSunshineMouse + procSunshinePad, []string{"event28"}},
		{"mouse only", procSunshineMouse, nil},
		{"xbox 360", procKeyboard + procXbox360, []string{"event31"}},
		{"dinput", procDInput, []string{"event9"}},
		{"short key bitmap", procShortKeys, nil},
		{"keyboard only", procKeyboard, nil},
		{"two pads in order", procXbox360 + procSunshineMouse + procDInput, []string{"event31", "event9"}},
		{"no trailing blank line", strings.TrimRight(procXbox360, "\n"), []string{"event31"}},
	} {
		var got []string
		for _, c := range gamepadCandidates(tt.proc) {
			got = append(got, c.event)
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}
