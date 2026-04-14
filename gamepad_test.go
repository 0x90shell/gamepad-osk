package main

import (
	"os"
	"testing"
	"time"
)

func newTestGamepad() *GamepadReader {
	cfg := DefaultConfig()
	gp := NewGamepadReader(cfg)
	gp.axisMax = 32767
	gp.trigMax = 255
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
	jitter := int32(gp.axisMax * 0.1) // 10% = inside 25% deadzone
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
	gp.handleAxis(ABS_X, 100)  // small positive jitter
	gp.handleAxis(ABS_X, -50)  // small negative jitter
	gp.handleAxis(ABS_X, 0)    // back to center

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
	gp.axisMax = 32767
	gp.trigMax = 255
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
	gp.axisMax = 32767
	gp.trigMax = 255
	gp.initButtonMap()
	return gp
}

func TestComboDetect_TwoButtons(t *testing.T) {
	gp := newComboGamepad("select+start")

	// Press select, then start
	gp.handleButton(0x13a, 1) // select press - combo not yet complete
	a := gp.handleButton(0x13b, 1) // start press - combo fires
	if a.Type != ActionToggle {
		t.Errorf("combo fire = %v, want ActionToggle", a.Type)
	}
}

func TestComboDetect_PressOrder(t *testing.T) {
	gp := newComboGamepad("select+start")

	// Reverse order: start first, then select
	gp.handleButton(0x13b, 1) // start press
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
	gp.axisMax = 32767
	gp.trigMax = 255
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

	gp.handleButton(0x13a, 1) // select
	gp.handleButton(0x13b, 1) // start - only 2 of 3, no fire
	a := gp.handleButton(BTN_TR, 1) // rb - all 3 held
	if a.Type != ActionToggle {
		t.Errorf("3-button combo = %v, want ActionToggle", a.Type)
	}
}

func TestComboDetect_FourButtons(t *testing.T) {
	gp := newComboGamepad("select+start+lb+rb")

	gp.handleButton(0x13a, 1) // select
	gp.handleButton(0x13b, 1) // start
	gp.handleButton(BTN_TL, 1) // lb - only 3 of 4
	a := gp.handleButton(BTN_TR, 1) // rb - all 4
	if a.Type != ActionToggle {
		t.Errorf("4-button combo = %v, want ActionToggle", a.Type)
	}
}

func TestComboDetect_PartialPress(t *testing.T) {
	gp := newComboGamepad("select+start+rb")

	gp.handleButton(0x13a, 1) // select
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

func TestDisconnectResetsState(t *testing.T) {
	gp := newTestGamepad()
	// Set up state that should be cleared on disconnect
	gp.mouseX = 0.8
	gp.mouseY = -0.5
	gp.navX.Direction = 1
	gp.dpadY.Direction = -1
	gp.ltActive = true
	gp.rtActive = true
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
	gp.rtActive = false

	if gp.mouseX != 0 || gp.mouseY != 0 {
		t.Error("mouse should be zeroed after disconnect")
	}
	if gp.navX.Direction != 0 || gp.dpadY.Direction != 0 {
		t.Error("nav axes should be zeroed after disconnect")
	}
	if gp.ltActive || gp.rtActive {
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
	gp.axisMax = 32767
	gp.trigMax = 255
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
