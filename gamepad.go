package main

import (
	"encoding/binary"
	"log"
	"math"
	"os"
	"strings"
	"syscall"
	"time"
)

// evdev constants
const (
	evdevEV_KEY = 0x01
	evdevEV_ABS = 0x03
	evdevEV_SYN = 0x00

	// Standard evdev button codes
	BTN_SOUTH  = 0x130 // 304 — Xbox A, PS Cross
	BTN_EAST   = 0x131 // 305 — Xbox B, PS Circle
	BTN_NORTH  = 0x133 // 307 — Xbox Y/X (varies!)
	BTN_WEST   = 0x134 // 308 — Xbox X/Y (varies!)
	BTN_TL     = 0x136 // 310 — LB/L1
	BTN_TR     = 0x137 // 311 — RB/R1
	BTN_TL2    = 0x138 // 312 — LT/L2 (digital, Switch Pro)
	BTN_TR2    = 0x139 // 313 — RT/R2 (digital, Switch Pro)
	BTN_THUMBL = 0x13d // 317 — L3
	BTN_THUMBR = 0x13e // 318 — R3

	ABS_X     = 0x00
	ABS_Y     = 0x01
	ABS_Z     = 0x02 // LT
	ABS_RX    = 0x03
	ABS_RY    = 0x04
	ABS_RZ    = 0x05 // RT
	ABS_HAT0X = 0x10
	ABS_HAT0Y = 0x11

	EVIOCGRAB = 0x40044590
)

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
	mouseX    float64
	mouseY    float64
	ltActive  bool
	rtActive  bool
	axisMax   float64
	trigMax   float64
	actionMap ActionMap
	pressBtn  uint16 // "press" button evdev code
	shiftAxis uint16 // "shift" axis for hold behavior
	// Configurable stick axis codes
	navAxisX   uint16
	navAxisY   uint16
	mouseAxisX uint16
	mouseAxisY uint16
	deviceName string
}

func NewGamepadReader(config Config) *GamepadReader {
	gp := &GamepadReader{
		config:  config,
		axisMax: 32767,
		trigMax: 255,
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

	fd, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		log.Printf("Error opening %s: %v", path, err)
		return false
	}
	gp.fd = fd
	gp.readAxisInfo()

	// Auto-detect swap_xy for Xbox 360 pads
	if gp.config.Gamepad.SwapXY == "auto" || gp.config.Gamepad.SwapXY == "" {
		name := gp.getDeviceName(path)
		gp.deviceName = name
		if strings.Contains(strings.ToLower(name), "x-box 360") ||
			strings.Contains(strings.ToLower(name), "xbox 360") {
			log.Printf("Auto-detected Xbox 360 pad, enabling swap_xy")
			gp.config.Gamepad.SwapXY = "true"
		}
	}

	// Build button map (after swap_xy is resolved)
	gp.initButtonMap()

	log.Printf("Opened gamepad: %s", path)
	return true
}

func (gp *GamepadReader) getDeviceName(devPath string) string {
	data, err := os.ReadFile("/proc/bus/input/devices")
	if err != nil {
		return ""
	}
	// Find the event handler matching our device path
	eventName := devPath[strings.LastIndex(devPath, "/")+1:]
	var name string
	for _, line := range strings.Split(string(data), "\n") {
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
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "N: Name=") {
			name = strings.Trim(line[8:], "\"")
		} else if strings.HasPrefix(line, "H: Handlers=") {
			handler = line[12:]
		} else if line == "" && name != "" {
			// Check if this device has a js* handler (joystick)
			if strings.Contains(handler, "js") {
				for _, part := range strings.Fields(handler) {
					if strings.HasPrefix(part, "event") {
						path := "/dev/input/" + part
						if _, err := os.Stat(path); err == nil {
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

func (gp *GamepadReader) readAxisInfo() {
	// Try to get axis ranges via EVIOCGABS
	// For now use defaults — most controllers are 32767 for sticks, 255 or 1023 for triggers
}

func (gp *GamepadReader) Grab() {
	if gp.fd == nil || gp.grabbed {
		return
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, gp.fd.Fd(), EVIOCGRAB, 1)
	if errno != 0 {
		log.Printf("Warning: could not grab device: %v", errno)
	} else {
		gp.grabbed = true
	}
}

func (gp *GamepadReader) Ungrab() {
	if gp.fd == nil || !gp.grabbed {
		return
	}
	syscall.Syscall(syscall.SYS_IOCTL, gp.fd.Fd(), EVIOCGRAB, 0)
	gp.grabbed = false
}

func (gp *GamepadReader) Fd() int {
	if gp.fd == nil {
		return -1
	}
	return int(gp.fd.Fd())
}

func (gp *GamepadReader) ReadEvents() []Action {
	if gp.fd == nil {
		return nil
	}

	var actions []Action
	var buf [24]byte

	for {
		n, err := syscall.Read(int(gp.fd.Fd()), buf[:])
		if n != 24 || err != nil {
			break
		}
		evType := binary.LittleEndian.Uint16(buf[16:])
		evCode := binary.LittleEndian.Uint16(buf[18:])
		evValue := int32(binary.LittleEndian.Uint32(buf[20:]))

		if a := gp.handleEvent(evType, evCode, evValue); a.Type != ActionNone {
			actions = append(actions, a)
		}
	}

	// Adaptive repeat
	now := time.Now()
	for _, pair := range []struct {
		nav *NavAxis
		isX bool
	}{{&gp.navX, true}, {&gp.navY, false}} {
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

	if code == gp.pressBtn {
		if pressed {
			Debugf("Button 0x%x pressed (press btn)", code)
			return Action{Type: ActionPressStart}
		}
		return Action{Type: ActionPress}
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

func (gp *GamepadReader) handleAxis(code uint16, value int32) Action {
	dz := gp.config.Gamepad.Deadzone
	norm := float64(value) / gp.axisMax

	switch code {
	case gp.navAxisX: // Nav stick X
		return gp.updateNav(&gp.navX, applyDeadzone(norm, dz), true)
	case gp.navAxisY: // Nav stick Y
		return gp.updateNav(&gp.navY, applyDeadzone(norm, dz), false)
	case ABS_HAT0X: // D-pad always navigates
		return gp.updateNav(&gp.navX, int(value), true)
	case ABS_HAT0Y:
		return gp.updateNav(&gp.navY, int(value), false)
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
			active := float64(value)/gp.trigMax > 0.3
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

func (gp *GamepadReader) Close() {
	gp.Ungrab()
	if gp.fd != nil {
		gp.fd.Close()
	}
}

func applyDeadzone(norm, dz float64) int {
	if norm > dz {
		return 1
	} else if norm < -dz {
		return -1
	}
	return 0
}

