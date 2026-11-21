package main

import (
	"encoding/binary"
	"log"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"strconv"
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
	synDropped  = 0x03 // synReport (0x00) is in injector.go
	// EVIOCGKEY(96): the pressed-key bitmap, KEY_MAX 0x2ff = 768 bits
	eviocgkey96 = 0x80604518

	// Standard evdev button codes
	BTN_SOUTH      = 0x130 // 304 - Xbox A, PS Cross
	BTN_EAST       = 0x131 // 305 - Xbox B, PS Circle
	BTN_NORTH      = 0x133 // 307 - Xbox Y/X (varies!)
	BTN_WEST       = 0x134 // 308 - Xbox X/Y (varies!)
	BTN_TL         = 0x136 // 310 - LB/L1
	BTN_TR         = 0x137 // 311 - RB/R1
	BTN_TL2        = 0x138 // 312 - LT/L2 (digital, Switch Pro)
	BTN_TR2        = 0x139 // 313 - RT/R2 (digital, Switch Pro)
	BTN_THUMBL     = 0x13d // 317 - L3
	BTN_THUMBR     = 0x13e // 318 - R3
	BTN_DPAD_UP    = 0x220 // 544 - DS3 D-pad (button, not axis)
	BTN_DPAD_DOWN  = 0x221 // 545
	BTN_DPAD_LEFT  = 0x222 // 546
	BTN_DPAD_RIGHT = 0x223 // 547

	ABS_X     = 0x00
	ABS_Y     = 0x01
	ABS_Z     = 0x02 // LT
	ABS_RX    = 0x03
	ABS_RY    = 0x04
	ABS_RZ    = 0x05 // RT
	ABS_HAT0X = 0x10
	ABS_HAT0Y = 0x11

	absNone = 0xffff // no axis: never matches an evdev ABS code (max 0x3f)

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
	config     Config
	fd         *os.File
	grabbed    bool
	navX       NavAxis
	navY       NavAxis
	dpadX      NavAxis
	dpadY      NavAxis
	mouseX     float64
	mouseY     float64
	ltActive   bool                 // shift axis held
	trigActive map[uint16]bool      // other trigger axes held, per axis
	axisRanges map[uint16]axisRange // per-axis min/max from EVIOCGABS
	actionMap  ActionMap
	pressBtn   uint16 // "press" button evdev code
	shiftAxis  uint16 // "shift" axis for hold behavior
	// Configurable stick axis codes
	navAxisX   uint16
	navAxisY   uint16
	mouseAxisX uint16
	mouseAxisY uint16
	deviceName string // currently unused; stored for future logging/diagnostics
	profile    string // controller profile ID, detected on every Open unless set in config

	// swap_xy as configured (auto, true, false). config.Gamepad.SwapXY holds
	// the value in effect for the open pad, decided again on every Open.
	swapXYSetting string

	// DInput/HID-generic layout (e.g. 8BitDo over Bluetooth): right stick on
	// ABS_Z/ABS_RZ, no ABS_RX/ABS_RY, triggers digital (BTN_TL2/BTN_TR2).
	hasAxis      map[uint16]bool
	dinputLayout bool

	// Toggle combo state
	comboButtons    []ComboButton    // parsed from config at startup (nil = disabled)
	comboPeriodMs   int              // timing window
	btnHeld         map[uint16]bool  // current button press state
	axisState       map[uint16]int32 // current axis values (for d-pad and triggers)
	comboFirstPress time.Time        // when the first combo button was pressed
	comboFired      bool             // edge-trigger flag (prevents repeat while held)
	comboSwallow    map[uint16]bool  // combo buttons held when it fired: their release does nothing

	// Last Open() error string. Suppresses repeat log lines while the
	// reconnect loop retries every 2s with the same error (e.g. no gamepad
	// plugged in). Cleared on successful open.
	lastOpenErr string
	dropped     bool // SYN_DROPPED seen: skip events until the next SYN_REPORT
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
		btnHeld:      make(map[uint16]bool),
		axisState:    make(map[uint16]int32),
		trigActive:   make(map[uint16]bool),
		hasAxis:      make(map[uint16]bool),
		comboSwallow: make(map[uint16]bool),

		swapXYSetting: config.Gamepad.SwapXY,
	}

	gp.setStickAxes()
	if gp.initCombo() && gp.comboButtons != nil {
		log.Printf("Toggle combo: %s (%dms window)", config.Gamepad.ToggleCombo, gp.comboPeriodMs)
	}

	return gp
}

// initCombo parses toggle_combo for the swap_xy in effect, so x and y in a
// combo are the buttons printed X and Y on the pad, as in the action map.
// It returns false if the combo is invalid (combo disabled).
func (gp *GamepadReader) initCombo() bool {
	gp.comboButtons = nil
	if gp.config.Gamepad.ToggleCombo == "" {
		return true
	}
	buttons, err := parseComboSwap(gp.config.Gamepad.ToggleCombo, isSwapXY(gp.config.Gamepad))
	if err != nil {
		log.Printf("Warning: invalid toggle_combo: %v", err)
		return false
	}
	gp.comboButtons = buttons
	gp.comboPeriodMs = gp.config.Gamepad.ComboPeriodMs
	if gp.comboPeriodMs <= 0 {
		gp.comboPeriodMs = 200
	}
	return true
}

// setStickAxes configures stick assignments (mouse_stick sets mouse, other
// stick = nav). On DInput-layout pads the right stick lives on ABS_Z/ABS_RZ.
func (gp *GamepadReader) setStickAxes() {
	rx, ry := uint16(ABS_RX), uint16(ABS_RY)
	if gp.dinputLayout {
		rx, ry = ABS_Z, ABS_RZ
	}
	if gp.config.Gamepad.MouseStick == "left" {
		gp.mouseAxisX, gp.mouseAxisY = ABS_X, ABS_Y
		gp.navAxisX, gp.navAxisY = rx, ry
	} else {
		gp.mouseAxisX, gp.mouseAxisY = rx, ry
		gp.navAxisX, gp.navAxisY = ABS_X, ABS_Y
	}
}

// detectLayout checks for a DInput/HID-generic axis layout: ABS_Z/ABS_RZ
// present but no ABS_RX/ABS_RY means Z/RZ are the right stick, not triggers.
func (gp *GamepadReader) detectLayout() {
	gp.dinputLayout = !gp.hasAxis[ABS_RX] && !gp.hasAxis[ABS_RY] &&
		gp.hasAxis[ABS_Z] && gp.hasAxis[ABS_RZ]
	if gp.dinputLayout {
		log.Printf("Detected DInput axis layout: right stick on ABS_Z/ABS_RZ, digital triggers")
	}
	gp.setStickAxes()
}

// initButtonMap builds the action map and finds special button codes.
// Called after device open so swap_xy auto-detect can take effect.
func (gp *GamepadReader) initButtonMap() {
	gp.actionMap = BuildActionMap(gp.config.Gamepad)
	table := buttonTable(isSwapXY(gp.config.Gamepad))

	gp.pressBtn = BTN_SOUTH
	if info, ok := table[gp.config.Gamepad.Buttons.Press]; ok {
		gp.pressBtn = info.EvdevBtn
	}
	// Only an axis-mapped shift gets hold-on-axis handling. A button-mapped
	// shift is released through BtnRelease (see BuildActionMap).
	gp.shiftAxis = absNone
	if info, ok := table[gp.config.Gamepad.Buttons.Shift]; ok && info.IsAxis {
		gp.shiftAxis = info.EvdevAxis
	}
	if gp.dinputLayout {
		// Z/RZ are the right stick here: drop trigger-axis actions, the
		// digital BTN_TL2/BTN_TR2 mappings (dual-mapped) remain active.
		for _, ax := range []uint16{ABS_Z, ABS_RZ} {
			delete(gp.actionMap.AxisActions, ax)
			delete(gp.actionMap.AxisRelease, ax)
		}
		gp.shiftAxis = absNone
	} else {
		// DS3, DS4 and DualSense send each trigger as an axis and as a
		// button: the axis alone drives it, or every action fires twice.
		for _, t := range []struct{ axis, btn uint16 }{{ABS_Z, BTN_TL2}, {ABS_RZ, BTN_TR2}} {
			if gp.hasAxis[t.axis] {
				delete(gp.actionMap.BtnPress, t.btn)
				delete(gp.actionMap.BtnRelease, t.btn)
				if gp.pressBtn == t.btn {
					gp.pressBtn = absNone
				}
			}
		}
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
		errStr := err.Error()
		if errStr != gp.lastOpenErr {
			gp.lastOpenErr = errStr
			if os.IsPermission(err) {
				log.Print(colorRed("Error: cannot read " + path + " - permission denied"))
				logPermissionFix()
			} else {
				log.Printf("Error opening %s: %v", path, err)
			}
		} else {
			Debugf("Error opening %s: %v (repeat suppressed)", path, err)
		}
		return false
	}
	gp.lastOpenErr = ""
	gp.fd = fd
	gp.readAxisInfo()
	gp.detectLayout()
	// Auto-detect swap_xy for Xbox controllers (xpad/xpadneo/xone drivers
	// emit BTN_X (=0x133, kernel alias for BTN_NORTH) for the physical X
	// button at the WEST position). SDL2's gamecontrollerdb compensates for
	// this in userspace; raw evdev readers like us have to do our own swap.
	//
	// Resolve symlinks so udev-aliased paths like /dev/input/gamepad0 work:
	// /sys/class/input/ and /proc/bus/input/devices both index by eventNN.
	resolvedPath := path
	if rp, err := filepath.EvalSymlinks(path); err == nil {
		resolvedPath = rp
	}
	gp.applyDevice(readDeviceInfo(resolvedPath))

	log.Printf("Opened gamepad: %s", path)
	return true
}

// applyDevice sets up everything that depends on which pad is open:
// swap_xy (when auto), the profile, the button map and the combo. Open
// calls it for every pad, so a reconnect to a different pad starts from
// the configured values, not from the last pad's.
func (gp *GamepadReader) applyDevice(info deviceInfo) {
	gp.deviceName = info.Name
	gp.config.Gamepad.SwapXY = gp.swapXYSetting
	if gp.swapXYSetting == "auto" || gp.swapXYSetting == "" {
		switch {
		case isXpadDriver(info.Driver):
			log.Printf("Auto-detected xpad driver, enabling swap_xy")
			gp.config.Gamepad.SwapXY = "true"
		case isXboxName(info.Name):
			// Fallback for uinput-created virtual gamepads (Steam, Sunshine)
			// that have no sysfs driver link
			log.Printf("Auto-detected Xbox pad by name, enabling swap_xy")
			gp.config.Gamepad.SwapXY = "true"
		default:
			Debugf("swap_xy auto: off for %q (driver %q)", info.Name, info.Driver)
		}
	}

	gp.profile = gp.config.Gamepad.Controller
	if gp.profile == "" || gp.profile == ProfileAuto {
		gp.profile = detectProfile(info)
		log.Printf("Controller profile: %s (detected)", ControllerProfiles[gp.profile].Name)
	}

	gp.initButtonMap()
	gp.initCombo()
}

// deviceInfo is what sysfs knows about an evdev node.
type deviceInfo struct {
	Name   string
	Driver string // kernel driver of the parent device, empty for uinput pads
	Vendor string // USB vendor ID, lower-case hex
}

// readDeviceInfo reads name, driver and vendor of an evdev node such as
// /dev/input/event5. Missing entries stay empty.
func readDeviceInfo(devPath string) deviceInfo {
	base := filepath.Join("/sys/class/input", filepath.Base(devPath), "device")
	read := func(p string) string {
		b, err := os.ReadFile(filepath.Join(base, p)) //nolint:gosec // G304: sysfs path built from device name
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	info := deviceInfo{
		Name:   read("name"),
		Vendor: strings.ToLower(read("id/vendor")),
	}
	if link, err := os.Readlink(filepath.Join(base, "device", "driver")); err == nil {
		info.Driver = filepath.Base(link)
	}
	return info
}

// isXpadDriver reports whether the driver is xpad, xpadneo or xone. These
// drivers emit BTN_X (=BTN_NORTH numerically) for the physical X button at
// the WEST position (legacy evdev convention).
func isXpadDriver(driver string) bool {
	return driver == "xpad" || driver == "xpadneo" || driver == "xone"
}

// isXboxName matches Xbox pads by name, for uinput pads without a driver.
func isXboxName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "x-box") || strings.Contains(n, "xbox")
}

// Profile returns the controller profile of the open pad. Before the first
// Open it is the configured one, or generic for auto.
func (gp *GamepadReader) Profile() ControllerProfile {
	id := gp.profile
	if id == "" {
		id = gp.config.Gamepad.Controller
	}
	return GetControllerProfile(id)
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
	for _, c := range gamepadCandidates(string(data)) {
		path := "/dev/input/" + c.event
		if _, err := os.Stat(path); err == nil { //nolint:gosec // G703: trusted /dev/input path
			log.Printf("Auto-detected gamepad: %s (%s)", c.name, path)
			return path
		}
	}
	return ""
}

type inputCandidate struct {
	name, event string
}

// gamepadCandidates lists the event nodes in /proc/bus/input/devices text
// that look like gamepads, in file order.
func gamepadCandidates(data string) []inputCandidate {
	var out []inputCandidate
	var name, handler, keys string
	flush := func() {
		// joydev also takes absolute mice and tablets (a js handler), so
		// the device must have joystick or gamepad buttons too
		if name != "" && strings.Contains(handler, "js") && hasJoystickButtons(keys) {
			for part := range strings.FieldsSeq(handler) {
				if strings.HasPrefix(part, "event") {
					out = append(out, inputCandidate{name, part})
				}
			}
		}
		name, handler, keys = "", "", ""
	}
	for line := range strings.SplitSeq(data, "\n") {
		switch {
		case strings.HasPrefix(line, "N: Name="):
			name = strings.Trim(line[8:], "\"")
		case strings.HasPrefix(line, "H: Handlers="):
			handler = line[12:]
		case strings.HasPrefix(line, "B: KEY="):
			keys = line[7:]
		case line == "":
			flush()
		}
	}
	flush()
	return out
}

// hasJoystickButtons reports whether a "B: KEY=" bitmap sets any code from
// BTN_JOYSTICK (0x120, DInput pads) to 0x13f (BTN_SOUTH..BTN_THUMBR are
// 0x130-0x13e). The bitmap is hex words of the kernel's long size, highest
// word first.
func hasJoystickButtons(keys string) bool {
	words := strings.Fields(keys)
	for code := 0x120; code <= 0x13f; code++ {
		i := len(words) - 1 - code/bits.UintSize
		if i < 0 {
			return false
		}
		w, err := strconv.ParseUint(words[i], 16, bits.UintSize)
		if err == nil && w&(1<<(code%bits.UintSize)) != 0 {
			return true
		}
	}
	return false
}

// feed handles one raw evdev event. The kernel sends SYN_DROPPED when its
// buffer overflowed: what follows up to the next SYN_REPORT is partial, so
// it is skipped and the pad's real state is read instead.
func (gp *GamepadReader) feed(evType, evCode uint16, value int32) []Action {
	if evType == evdevEV_SYN && evCode == synDropped {
		gp.dropped = true
		return nil
	}
	if gp.dropped {
		if evType == evdevEV_SYN && evCode == synReport {
			gp.dropped = false
			return gp.resync()
		}
		return nil
	}
	if a := gp.handleEvent(evType, evCode, value); a.Type != ActionNone {
		return []Action{a}
	}
	return nil
}

// resync reads the pad's real button and axis state after SYN_DROPPED.
// Without a device (or if the ioctl fails) every button counts as released.
func (gp *GamepadReader) resync() []Action {
	log.Printf("Gamepad events dropped by the kernel, reading the pad state again")
	var keys [96]byte
	abs := make(map[uint16]int32)
	if gp.fd == nil {
		return gp.resyncFrom(keys[:], abs)
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, gp.fd.Fd(), eviocgkey96,
		uintptr(unsafe.Pointer(&keys[0]))) //nolint:gosec // G103: ioctl requires pointer to kernel struct
	if errno != 0 {
		keys = [96]byte{}
	}
	for _, axis := range []uint16{ABS_X, ABS_Y, ABS_RX, ABS_RY, ABS_Z, ABS_RZ, ABS_HAT0X, ABS_HAT0Y} {
		var info [6]int32
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, gp.fd.Fd(),
			uintptr(eviocgabsBase+uint32(axis)), //nolint:gosec // G115: axis fits in uint32
			uintptr(unsafe.Pointer(&info[0])))   //nolint:gosec // G103: ioctl requires pointer to kernel struct
		if errno == 0 {
			abs[axis] = info[0]
		}
	}
	return gp.resyncFrom(keys[:], abs)
}

// resyncFrom releases every button the reader holds that keys (an
// EVIOCGKEY bitmap) shows released, and feeds each axis its current value.
// Buttons pressed during the gap are not pressed here: their press is lost.
func (gp *GamepadReader) resyncFrom(keys []byte, abs map[uint16]int32) []Action {
	var actions []Action
	add := func(a Action) {
		if a.Type != ActionNone {
			actions = append(actions, a)
		}
	}
	for code, held := range gp.btnHeld {
		if held && (int(code/8) >= len(keys) || keys[code/8]&(1<<(code%8)) == 0) {
			add(gp.handleButton(code, 0))
		}
	}
	for axis, v := range abs {
		add(gp.handleAxis(axis, v))
	}
	return actions
}

// readAxisInfo queries EVIOCGABS for each axis to get the actual min/max range.
// This handles controllers with different ranges (DS4/DS5: 0-255, Xbox: -32768 to 32767).
func (gp *GamepadReader) readAxisInfo() {
	if gp.fd == nil {
		return
	}
	axes := []uint16{ABS_X, ABS_Y, ABS_RX, ABS_RY, ABS_Z, ABS_RZ}
	gp.hasAxis = make(map[uint16]bool)
	for _, axis := range axes {
		// input_absinfo: value, minimum, maximum, fuzz, flat, resolution (6 x int32)
		var info [6]int32
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
			gp.fd.Fd(),
			uintptr(eviocgabsBase+uint32(axis)), //nolint:gosec // G115: axis fits in uint32
			uintptr(unsafe.Pointer(&info[0])))   //nolint:gosec // G103: ioctl requires pointer to kernel struct
		if errno != 0 {
			continue
		}
		axMin, axMax := info[1], info[2]
		// EVIOCGABS succeeds with a zeroed range for axes the device lacks
		if axMax > axMin {
			gp.hasAxis[axis] = true
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
				gp.trigActive = make(map[uint16]bool)
				gp.comboFirstPress = time.Time{}
				gp.comboFired = false
				gp.comboSwallow = make(map[uint16]bool)
			}
			break
		}
		evType := binary.LittleEndian.Uint16(buf[16:])
		evCode := binary.LittleEndian.Uint16(buf[18:])
		evValue := int32(binary.LittleEndian.Uint32(buf[20:])) //nolint:gosec // G115: evdev value fits in int32

		actions = append(actions, gp.feed(evType, evCode, evValue)...)
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
	if !pressed && gp.comboSwallow[code] {
		delete(gp.comboSwallow, code)
		return Action{Type: ActionNone}
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
				if active && !gp.trigActive[code] {
					gp.trigActive[code] = true
					return Action{Type: action}
				} else if !active && gp.trigActive[code] {
					gp.trigActive[code] = false
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
		// The combo shows or hides the keyboard; the release of its
		// buttons must not act on it as well (A's release would type)
		for _, cb := range gp.comboButtons {
			for _, code := range cb.BtnCodes {
				if gp.btnHeld[code] {
					gp.comboSwallow[code] = true
				}
			}
		}
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
	if gp.dinputLayout && (cb.AxisCode == ABS_Z || cb.AxisCode == ABS_RZ) {
		return false // Z/RZ are the right stick on DInput pads
	}
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
	// On DInput pads Z/RZ are the right stick (resting mid-range) and the
	// digital triggers are already covered by btnHeld above.
	if gp.dinputLayout {
		return false
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
