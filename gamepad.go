package main

import (
	"encoding/binary"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

//nolint:revive // Match Linux evdev naming
const (
	evdevEV_KEY = 0x01
	evdevEV_ABS = 0x03
	evdevEV_SYN = 0x00

	// Standard evdev button codes
	BTN_SOUTH  = 0x130 // 304 - Xbox A, PS Cross
	BTN_EAST   = 0x131 // 305 - Xbox B, PS Circle
	BTN_NORTH  = 0x133 // 307 - Xbox Y/X (varies!)
	BTN_WEST   = 0x134 // 308 - Xbox X/Y (varies!)
	BTN_TL     = 0x136 // 310 - LB/L1
	BTN_TR     = 0x137 // 311 - RB/R1
	BTN_TL2    = 0x138 // 312 - LT/L2 (digital, Switch Pro)
	BTN_TR2    = 0x139 // 313 - RT/R2 (digital, Switch Pro)
	BTN_THUMBL    = 0x13d // 317 - L3
	BTN_THUMBR    = 0x13e // 318 - R3
	BTN_DPAD_UP   = 0x220 // 544 - DS3 D-pad (button, not axis)
	BTN_DPAD_DOWN = 0x221 // 545
	BTN_DPAD_LEFT = 0x222 // 546
	BTN_DPAD_RIGHT = 0x223 // 547

	ABS_X     = 0x00
	ABS_Y     = 0x01
	ABS_Z     = 0x02 // LT
	ABS_RX    = 0x03
	ABS_RY    = 0x04
	ABS_RZ    = 0x05 // RT
	ABS_HAT0X = 0x10
	ABS_HAT0Y = 0x11

	EVIOCGRAB = 0x40044590

	// EVIOCGABS(axis) = _IOR('E', 0x40 + axis, struct input_absinfo)
	// input_absinfo is 6 x int32 = 24 bytes, so _IOR size = 24
	// _IOR('E', 0x40+axis, 24) = 0x80184540 + axis
	eviocgabsBase = 0x80184540
)

// axisRange stores min/max for a single evdev axis.
type axisRange struct {
	min, max int32
}

type ActionType int

const (
	ActionNone ActionType = iota
	ActionNavigate
	ActionPress
	ActionPressStart
	ActionBackspace
	ActionSpace
	ActionEnter
	ActionShiftOn
	ActionShiftOff
	ActionCapsToggle
	ActionClose
	ActionMouseMove
	ActionLeftClick
	ActionLeftClickRelease
	ActionRightClick
	ActionRightClickRelease
	ActionPositionToggle
	ActionPressRepeat
	ActionBackspaceRelease
	ActionSpaceRelease
	ActionEnterRelease
	ActionToggle
)

type Action struct {
	Type ActionType
	DX   int
	DY   int
}

type NavAxis struct {
	Direction int
	HeldSince time.Time
	LastMove  time.Time
}

func (n *NavAxis) RepeatInterval() time.Duration {
	if n.HeldSince.IsZero() {
		return 300 * time.Millisecond
	}
	elapsed := time.Since(n.HeldSince).Seconds()
	t := math.Min(elapsed/1.0, 1.0)
	ms := 300.0 + (80.0-300.0)*t
	return time.Duration(ms) * time.Millisecond
}

type GamepadReader struct {
	config    Config
	fd        *os.File
	grabbed   bool
	navX      NavAxis
	navY      NavAxis
	dpadX     NavAxis
	dpadY     NavAxis
	mouseX    float64
	mouseY    float64
	ltActive  bool
	rtActive  bool
	axisRanges map[uint16]axisRange // per-axis min/max from EVIOCGABS
	actionMap ActionMap
	pressBtn  uint16 // "press" button evdev code
	shiftAxis uint16 // "shift" axis for hold behavior
	// Configurable stick axis codes
	navAxisX   uint16
	navAxisY   uint16
	mouseAxisX uint16
	mouseAxisY uint16
	deviceName string // currently unused; stored for future logging/diagnostics

	// Toggle combo state
	comboButtons    []ComboButton     // parsed from config at startup (nil = disabled)
	comboPeriodMs   int               // timing window
	btnHeld         map[uint16]bool   // current button press state
	axisState       map[uint16]int32  // current axis values (for d-pad and triggers)
	comboFirstPress time.Time         // when the first combo button was pressed
	comboFired      bool              // edge-trigger flag (prevents repeat while held)
}

func NewGamepadReader(config Config) *GamepadReader {
	gp := &GamepadReader{
		config: config,
		axisRanges: map[uint16]axisRange{
			// Defaults for Xbox 360/One (overridden by readAxisInfo if available)
			ABS_X:  {-32768, 32767},
			ABS_Y:  {-32768, 32767},
			ABS_RX: {-32768, 32767},
			ABS_RY: {-32768, 32767},
			ABS_Z:  {0, 255},
			ABS_RZ: {0, 255},
		},
		btnHeld:   make(map[uint16]bool),
		axisState: make(map[uint16]int32),
	}

	// Configure stick assignments (mouse_stick sets mouse, other stick = nav)
	mouse := config.Gamepad.MouseStick
	if mouse == "left" {
		gp.mouseAxisX, gp.mouseAxisY = ABS_X, ABS_Y
		gp.navAxisX, gp.navAxisY = ABS_RX, ABS_RY
	} else {
		gp.mouseAxisX, gp.mouseAxisY = ABS_RX, ABS_RY
		gp.navAxisX, gp.navAxisY = ABS_X, ABS_Y
	}

	// Parse toggle combo
	if config.Gamepad.ToggleCombo != "" {
		buttons, err := parseComboString(config.Gamepad.ToggleCombo)
		if err != nil {
			log.Printf("Warning: invalid toggle_combo: %v", err)
		} else {
			gp.comboButtons = buttons
			gp.comboPeriodMs = config.Gamepad.ComboPeriodMs
			if gp.comboPeriodMs <= 0 {
				gp.comboPeriodMs = 200
			}
			log.Printf("Toggle combo: %s (%dms window)", config.Gamepad.ToggleCombo, gp.comboPeriodMs)
		}
	}

	return gp
}

// initButtonMap builds the action map and finds special button codes.
// Called after device open so swap_xy auto-detect can take effect.
func (gp *GamepadReader) initButtonMap() {
	gp.actionMap = BuildActionMap(gp.config.Gamepad)
	table := buttonTable(isSwapXY(gp.config.Gamepad))

	gp.pressBtn = BTN_SOUTH
	if info, ok := table[gp.config.Gamepad.Buttons.Press]; ok && !info.IsAxis {
		gp.pressBtn = info.EvdevBtn
	}
	gp.shiftAxis = ABS_Z
	if info, ok := table[gp.config.Gamepad.Buttons.Shift]; ok && info.IsAxis {
		gp.shiftAxis = info.EvdevAxis
	}
}

func (gp *GamepadReader) Open(devicePath string) bool {
	path := devicePath
	if path == "" {
		path = gp.config.Gamepad.Device
	}
	if path == "" {
		path = gp.autoDetect()
	}
	if path == "" {
		return false
	}

	fd, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0) //nolint:gosec // G304: path is from config or /dev/input
	if err != nil {
		if os.IsPermission(err) {
			log.Print(colorRed("Error: cannot read " + path + " - permission denied"))
			logPermissionFix()
		} else {
			log.Printf("Error opening %s: %v", path, err)
		}
		return false
	}
	gp.fd = fd
	gp.readAxisInfo()
	// Auto-detect swap_xy for Xbox controllers (xpad/xpadneo/xone drivers swap BTN_NORTH/BTN_WEST)
	if gp.config.Gamepad.SwapXY == "auto" || gp.config.Gamepad.SwapXY == "" {
		name := gp.getDeviceName(path)
		gp.deviceName = name
		if gp.isXpadDriver(path) {
			log.Printf("Auto-detected xpad driver, enabling swap_xy")
			gp.config.Gamepad.SwapXY = "true"
		} else if strings.Contains(strings.ToLower(name), "x-box") ||
			strings.Contains(strings.ToLower(name), "xbox") {
			// Fallback for Steam virtual gamepads (uinput, no sysfs driver link)
			log.Printf("Auto-detected Xbox pad by name, enabling swap_xy")
			gp.config.Gamepad.SwapXY = "true"
		}
	}

	// Build button map (after swap_xy is resolved)
	gp.initButtonMap()

	log.Printf("Opened gamepad: %s", path)
	return true
}

// isXpadDriver checks if the evdev device uses xpad, xpadneo, or xone kernel driver.
// These drivers report BTN_X=BTN_NORTH and BTN_Y=BTN_WEST (swapped vs physical position).
func (gp *GamepadReader) isXpadDriver(devPath string) bool {
	// /dev/input/event18 -> event18
	base := filepath.Base(devPath)
	// Check /sys/class/input/event18/device/device/driver symlink
	link, err := os.Readlink(filepath.Join("/sys/class/input", base, "device", "device", "driver"))
	if err != nil {
		return false
	}
	driver := filepath.Base(link)
	return driver == "xpad" || driver == "xpadneo" || driver == "xone"
}

func (gp *GamepadReader) getDeviceName(devPath string) string {
	data, err := os.ReadFile("/proc/bus/input/devices")
	if err != nil {
		return ""
	}
	// Find the event handler matching our device path
	eventName := devPath[strings.LastIndex(devPath, "/")+1:]
	var name string
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(line, "N: Name=") {
			name = strings.Trim(line[8:], "\"")
		} else if strings.HasPrefix(line, "H: Handlers=") && strings.Contains(line, eventName) {
			return name
		}
	}
	return ""
}

func (gp *GamepadReader) autoDetect() string {
	// Check common symlinks first
	for _, path := range []string{"/dev/input/gamepad0"} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Parse /proc/bus/input/devices to find gamepads
	data, err := os.ReadFile("/proc/bus/input/devices")
	if err != nil {
		return ""
	}

	var name, handler string
	for line := range strings.SplitSeq(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "N: Name="):
			name = strings.Trim(line[8:], "\"")
		case strings.HasPrefix(line, "H: Handlers="):
			handler = line[12:]
		case line == "" && name != "":
			// Check if this device has a js* handler (joystick)
			if strings.Contains(handler, "js") {
				for part := range strings.FieldsSeq(handler) {
					if strings.HasPrefix(part, "event") {
						path := "/dev/input/" + part
						if _, err := os.Stat(path); err == nil { //nolint:gosec // G703: trusted /dev/input path
							log.Printf("Auto-detected gamepad: %s (%s)", name, path)
							return path
						}
					}
				}
			}
			name = ""
			handler = ""
		}
	}
	return ""
}

// readAxisInfo queries EVIOCGABS for each axis to get the actual min/max range.
// This handles controllers with different ranges (DS4/DS5: 0-255, Xbox: -32768 to 32767).
func (gp *GamepadReader) readAxisInfo() {
	if gp.fd == nil {
		return
	}
	axes := []uint16{ABS_X, ABS_Y, ABS_RX, ABS_RY, ABS_Z, ABS_RZ}
	for _, axis := range axes {
		// input_absinfo: value, minimum, maximum, fuzz, flat, resolution (6 x int32)
		var info [6]int32
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
			gp.fd.Fd(),
			uintptr(eviocgabsBase+uint32(axis)),                  //nolint:gosec // G115: axis fits in uint32
			uintptr(unsafe.Pointer(&info[0]))) //nolint:gosec // G103: ioctl requires pointer to kernel struct
		if errno != 0 {
			continue
		}
		axMin, axMax := info[1], info[2]
		if axMax > axMin {
			gp.axisRanges[axis] = axisRange{axMin, axMax}
			Debugf("Axis 0x%x range: %d to %d", axis, axMin, axMax)
		}
	}
}

func (gp *GamepadReader) Grab() {
	if gp.fd == nil || gp.grabbed {
		return
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, gp.fd.Fd(), EVIOCGRAB, 1) //nolint:gosec // G115: fd fits in int
	if errno != 0 {
		log.Printf("Warning: could not grab device: %v (another instance may hold the grab)", errno)
	} else {
		gp.grabbed = true
	}
}

func (gp *GamepadReader) Ungrab() {
	if gp.fd == nil || !gp.grabbed {
		return
	}
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, gp.fd.Fd(), EVIOCGRAB, 0)
	gp.grabbed = false
}

func (gp *GamepadReader) Fd() int {
	if gp.fd == nil {
		return -1
	}
	return int(gp.fd.Fd()) //nolint:gosec // G115: fd fits in int
}

func (gp *GamepadReader) ReadEvents() []Action {
	if gp.fd == nil {
		return nil
	}

	var actions []Action
	var buf [24]byte

	for {
		n, err := syscall.Read(int(gp.fd.Fd()), buf[:]) //nolint:gosec // G115: fd fits in int
		if n != 24 || err != nil {
			if err != nil && err != syscall.EAGAIN && err != syscall.EWOULDBLOCK {
				log.Printf("Gamepad disconnected: %v", err)
				gp.Ungrab()
				_ = gp.fd.Close()
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
				gp.comboFirstPress = time.Time{}
				gp.comboFired = false
			}
			break
		}
		evType := binary.LittleEndian.Uint16(buf[16:])
		evCode := binary.LittleEndian.Uint16(buf[18:])
		evValue := int32(binary.LittleEndian.Uint32(buf[20:])) //nolint:gosec // G115: evdev value fits in int32

		if a := gp.handleEvent(evType, evCode, evValue); a.Type != ActionNone {
			actions = append(actions, a)
		}
	}

	// Adaptive repeat (stick + d-pad independently)
	now := time.Now()
	for _, pair := range []struct {
		nav *NavAxis
		isX bool
	}{
		{&gp.navX, true}, {&gp.navY, false},
		{&gp.dpadX, true}, {&gp.dpadY, false},
	} {
		if pair.nav.Direction != 0 && now.Sub(pair.nav.LastMove) >= pair.nav.RepeatInterval() {
			pair.nav.LastMove = now
			if pair.isX {
				actions = append(actions, Action{Type: ActionNavigate, DX: pair.nav.Direction})
			} else {
				actions = append(actions, Action{Type: ActionNavigate, DY: pair.nav.Direction})
			}
		}
	}

	// Mouse
	if gp.config.Mouse.Enabled && (math.Abs(gp.mouseX) > 0.01 || math.Abs(gp.mouseY) > 0.01) {
		s := float64(gp.config.Mouse.Sensitivity)
		actions = append(actions, Action{
			Type: ActionMouseMove,
			DX:   int(gp.mouseX * s),
			DY:   int(gp.mouseY * s),
		})
	}

	return actions
}

func (gp *GamepadReader) handleEvent(evType, evCode uint16, value int32) Action {
	switch evType {
	case evdevEV_KEY:
		return gp.handleButton(evCode, value)
	case evdevEV_ABS:
		return gp.handleAxis(evCode, value)
	}
	return Action{Type: ActionNone}
}

func (gp *GamepadReader) handleButton(code uint16, value int32) Action {
	pressed := value == 1

	// Track button state for combo detection
	gp.btnHeld[code] = pressed

	// Check combo on every button event
	if toggle := gp.checkCombo(); toggle.Type != ActionNone {
		return toggle
	}

	if code == gp.pressBtn {
		if pressed {
			Debugf("Button 0x%x pressed (press btn)", code)
			return Action{Type: ActionPressStart}
		}
		return Action{Type: ActionPress}
	}

	// DS3 reports D-pad as buttons instead of HAT axes
	switch code {
	case BTN_DPAD_UP:
		if pressed {
			return gp.updateNav(&gp.dpadY, -1, false)
		}
		return gp.updateNav(&gp.dpadY, 0, false)
	case BTN_DPAD_DOWN:
		if pressed {
			return gp.updateNav(&gp.dpadY, 1, false)
		}
		return gp.updateNav(&gp.dpadY, 0, false)
	case BTN_DPAD_LEFT:
		if pressed {
			return gp.updateNav(&gp.dpadX, -1, true)
		}
		return gp.updateNav(&gp.dpadX, 0, true)
	case BTN_DPAD_RIGHT:
		if pressed {
			return gp.updateNav(&gp.dpadX, 1, true)
		}
		return gp.updateNav(&gp.dpadX, 0, true)
	}

	if pressed {
		if action, ok := gp.actionMap.BtnPress[code]; ok {
			Debugf("Button 0x%x → action %d", code, action)
			return Action{Type: action}
		}
		Debugf("Button 0x%x pressed (unmapped)", code)
	} else {
		if action, ok := gp.actionMap.BtnRelease[code]; ok {
			return Action{Type: action}
		}
	}
	return Action{Type: ActionNone}
}

// normalizeAxis maps a raw axis value to -1.0..1.0 using the axis's actual range.
func (gp *GamepadReader) normalizeAxis(code uint16, value int32) float64 {
	r, ok := gp.axisRanges[code]
	if !ok || r.max <= r.min {
		return 0
	}
	// Map [min, max] to [-1.0, 1.0]
	return (float64(value-r.min)/float64(r.max-r.min))*2.0 - 1.0
}

func (gp *GamepadReader) handleAxis(code uint16, value int32) Action {
	dz := gp.config.Gamepad.Deadzone
	norm := gp.normalizeAxis(code, value)

	// Track axis state for combo detection (d-pad and triggers)
	gp.axisState[code] = value

	// Check combo on axis events that could be combo-relevant (d-pad, triggers)
	if code == ABS_HAT0X || code == ABS_HAT0Y || code == ABS_Z || code == ABS_RZ {
		if toggle := gp.checkCombo(); toggle.Type != ActionNone {
			return toggle
		}
	}

	switch code {
	case gp.navAxisX: // Nav stick X
		return gp.updateNav(&gp.navX, applyDeadzone(norm, dz), true)
	case gp.navAxisY: // Nav stick Y
		return gp.updateNav(&gp.navY, applyDeadzone(norm, dz), false)
	case ABS_HAT0X: // D-pad - separate axis to avoid stick jitter interference
		return gp.updateNav(&gp.dpadX, int(value), true)
	case ABS_HAT0Y:
		return gp.updateNav(&gp.dpadY, int(value), false)
	case gp.mouseAxisX: // Mouse stick X
		if math.Abs(norm) < dz {
			gp.mouseX = 0
		} else {
			gp.mouseX = norm
		}
	case gp.mouseAxisY: // Mouse stick Y
		if math.Abs(norm) < dz {
			gp.mouseY = 0
		} else {
			gp.mouseY = norm
		}
	default:
		// Check if this axis is mapped to an action (triggers)
		if action, ok := gp.actionMap.AxisActions[code]; ok {
			active := gp.normalizeAxis(code, value) > -0.4 // triggers: -1.0 (released) to 1.0 (full), fire at ~30%
			if code == gp.shiftAxis {
				// Shift is hold-based: on/off
				if active != gp.ltActive {
					gp.ltActive = active
					if active {
						return Action{Type: ActionShiftOn}
					}
					return Action{Type: ActionShiftOff}
				}
			} else {
				// Other triggers: fire once on press (edge detection)
				if active && !gp.rtActive {
					gp.rtActive = true
					return Action{Type: action}
				} else if !active && gp.rtActive {
					gp.rtActive = false
					if releaseAction, ok := gp.actionMap.AxisRelease[code]; ok {
						return Action{Type: releaseAction}
					}
				}
			}
		}
	}
	return Action{Type: ActionNone}
}

func (gp *GamepadReader) updateNav(nav *NavAxis, direction int, isX bool) Action {
	if direction != nav.Direction {
		nav.Direction = direction
		if direction != 0 {
			now := time.Now()
			nav.HeldSince = now
			nav.LastMove = now
			if isX {
				return Action{Type: ActionNavigate, DX: direction}
			}
			return Action{Type: ActionNavigate, DY: direction}
		}
		nav.HeldSince = time.Time{}
	}
	return Action{Type: ActionNone}
}

// checkCombo checks if the toggle combo is fully satisfied and returns ActionToggle if so.
// Implements edge-triggering (fires once per press) and timing window.
// Timer starts on the first button press, not when all buttons are held.
// Window expiry only takes effect on the next press cycle (requires button release to reset).
func (gp *GamepadReader) checkCombo() Action {
	if len(gp.comboButtons) == 0 {
		return Action{Type: ActionNone}
	}

	allHeld := true
	anyHeld := false
	for _, cb := range gp.comboButtons {
		if gp.isComboButtonHeld(cb) {
			anyHeld = true
		} else {
			allHeld = false
		}
	}

	if !anyHeld {
		// Full release - reset everything
		gp.comboFired = false
		gp.comboFirstPress = time.Time{}
		return Action{Type: ActionNone}
	}

	if !allHeld {
		// Partial hold - start timer on first press, reset edge trigger
		gp.comboFired = false
		if gp.comboFirstPress.IsZero() {
			gp.comboFirstPress = time.Now()
		}
		return Action{Type: ActionNone}
	}

	// All held
	if gp.comboFired {
		return Action{Type: ActionNone}
	}
	if gp.comboFirstPress.IsZero() {
		gp.comboFirstPress = time.Now()
	}
	if time.Since(gp.comboFirstPress) <= time.Duration(gp.comboPeriodMs)*time.Millisecond {
		gp.comboFired = true
		Debugf("Toggle combo fired: %s", gp.config.Gamepad.ToggleCombo)
		return Action{Type: ActionToggle}
	}
	// Window expired while buttons still held - wait for release before allowing retry
	return Action{Type: ActionNone}
}

// isComboButtonHeld checks if a single combo button is currently satisfied.
// A button is satisfied if any of its BtnCodes are held OR its axis matches.
// Analog triggers use the same 30% threshold as action detection to avoid noise.
func (gp *GamepadReader) isComboButtonHeld(cb ComboButton) bool {
	// Check digital button codes
	for _, code := range cb.BtnCodes {
		if gp.btnHeld[code] {
			return true
		}
	}
	// Check axis (d-pad or analog trigger)
	if cb.AxisCode != 0 {
		axisVal := gp.axisState[cb.AxisCode]
		if cb.AxisVal < 0 && axisVal < 0 {
			return true // d-pad up or left
		}
		if cb.AxisVal > 0 {
			// Triggers (ABS_Z, ABS_RZ): use threshold to avoid noise near zero
			// D-pad down/right (ABS_HAT0X/Y = 1): any positive value suffices
			if cb.AxisCode == ABS_Z || cb.AxisCode == ABS_RZ {
				return gp.normalizeAxis(cb.AxisCode, axisVal) > -0.4
			}
			return axisVal > 0
		}
	}
	return false
}

func (gp *GamepadReader) SetSensitivity(value int) {
	gp.config.Mouse.Sensitivity = value
}

// NeedsPolling returns true when cached stick state requires continuous polling
// (mouse movement or nav repeat active).
func (gp *GamepadReader) NeedsPolling() bool {
	if gp.fd == nil {
		return false
	}
	if gp.config.Mouse.Enabled && (math.Abs(gp.mouseX) > 0.01 || math.Abs(gp.mouseY) > 0.01) {
		return true
	}
	for _, nav := range []*NavAxis{&gp.navX, &gp.navY, &gp.dpadX, &gp.dpadY} {
		if nav.Direction != 0 {
			return true
		}
	}
	return false
}

// AnyButtonHeld reports whether any digital button, d-pad direction, or
// analog trigger (over the 30% action threshold) is currently held. Used by
// the deferred-grab logic in app.go: gamepad-osk waits for trigger combos to
// release before calling EVIOCGRAB, so other readers of the same evdev node
// (evsieve hooks, gamepad mappers) observe the release events and don't end
// up with stuck per-button state. Mirrors evsieve's own grab=auto idiom.
func (gp *GamepadReader) AnyButtonHeld() bool {
	if gp.fd == nil {
		return false
	}
	for _, held := range gp.btnHeld {
		if held {
			return true
		}
	}
	if gp.axisState[ABS_HAT0X] != 0 || gp.axisState[ABS_HAT0Y] != 0 {
		return true
	}
	for _, axis := range []uint16{ABS_Z, ABS_RZ} {
		r, ok := gp.axisRanges[axis]
		if !ok {
			continue
		}
		span := float64(r.max - r.min)
		if span <= 0 {
			continue
		}
		norm := float64(gp.axisState[axis]-r.min) / span
		if norm > 0.3 {
			return true
		}
	}
	return false
}

// Reconnect attempts to re-open the gamepad after a disconnect.
// Tries the configured device path first, then auto-detect.
func (gp *GamepadReader) Reconnect() bool {
	if gp.fd != nil {
		return true
	}
	Debugf("Attempting gamepad reconnect...")
	path := gp.config.Gamepad.Device
	if path == "" {
		path = gp.autoDetect()
	}
	if path == "" {
		return false
	}
	return gp.Open(path)
}

func (gp *GamepadReader) Close() {
	gp.Ungrab()
	if gp.fd != nil {
		_ = gp.fd.Close()
	}
}

// isUserInGroup and logPermissionFix are in util.go

func applyDeadzone(norm, dz float64) int {
	if norm > dz {
		return 1
	} else if norm < -dz {
		return -1
	}
	return 0
}

