package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

const (
	uinputPath = "/dev/uinput"
	evKey      = 0x01
	evRel      = 0x02
	evSyn      = 0x00
	relX       = 0x00
	relY       = 0x01
	synReport  = 0x00

	// uinput ioctls (linux/uinput.h)
	uiSetEvbit   = 0x40045564 // _IOW('U', 100, int)
	uiSetKeybit  = 0x40045565 // _IOW('U', 101, int)
	uiSetRelbit  = 0x40045566 // _IOW('U', 102, int)
	uiDevSetup   = 0x405c5503 // _IOW('U', 3, struct uinput_setup)
	uiDevCreate  = 0x5501     // _IO('U', 1)
	uiDevDestroy = 0x5502     // _IO('U', 2)
	busUSB       = 0x03
)

// uinputSetup is struct uinput_setup: struct input_id (bustype, vendor,
// product, version), char name[80], __u32 ff_effects_max. 92 bytes.
type uinputSetup struct {
	bustype, vendor, product, version uint16
	name                              [80]byte
	ffEffectsMax                      uint32
}

// uinputCaps lists the ioctls that declare what the device can send:
// keyboard keys 1-248, the three mouse buttons and the two mouse axes.
func uinputCaps() [][2]uintptr {
	caps := [][2]uintptr{{uiSetEvbit, evKey}, {uiSetEvbit, evRel}, {uiSetEvbit, evSyn}}
	for k := uintptr(1); k < 249; k++ {
		caps = append(caps, [2]uintptr{uiSetKeybit, k})
	}
	for _, b := range []uintptr{0x110, 0x111, 0x112} { // BTN_LEFT, BTN_RIGHT, BTN_MIDDLE
		caps = append(caps, [2]uintptr{uiSetKeybit, b})
	}
	return append(caps, [2]uintptr{uiSetRelbit, relX}, [2]uintptr{uiSetRelbit, relY})
}

func ioctl(fd int, req, arg uintptr) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, arg); errno != 0 {
		return errno
	}
	return nil
}

// ioctlPtr is ioctl with a pointer argument, converted inside the call so
// the pointer stays valid if the stack moves.
func ioctlPtr(fd int, req uintptr, arg unsafe.Pointer) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}

// createUinputDevice opens /dev/uinput and makes the virtual keyboard and
// mouse. Returns the fd, or -1 (open failed), -2 (setup) or -3 (create).
func createUinputDevice() int {
	fd, err := syscall.Open(uinputPath, syscall.O_RDWR|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return -1
	}
	for _, c := range uinputCaps() {
		_ = ioctl(fd, c[0], c[1])
	}
	setup := uinputSetup{bustype: busUSB, vendor: 0x1234, product: 0x5678, version: 1}
	copy(setup.name[:], "gamepad-osk")
	if ioctlPtr(fd, uiDevSetup, unsafe.Pointer(&setup)) != nil { //nolint:gosec // G103: ioctl requires pointer to kernel struct
		_ = syscall.Close(fd)
		return -2
	}
	if ioctl(fd, uiDevCreate, 0) != nil {
		_ = syscall.Close(fd)
		return -3
	}
	return fd
}

type Injector struct {
	fd *os.File
}

func NewInjector() (*Injector, error) {
	cfd := createUinputDevice()
	if cfd < 0 {
		return nil, uinputError(cfd)
	}

	fd := os.NewFile(uintptr(cfd), uinputPath)
	time.Sleep(50 * time.Millisecond)

	return &Injector{fd: fd}, nil
}

// uinputError returns a short single-line error for the injector failure.
// The caller is responsible for logging fix steps via logPermissionFix().
func uinputError(code int) error {
	if _, err := os.Stat(uinputPath); os.IsNotExist(err) {
		return errors.New("/dev/uinput not found - load the module: sudo modprobe uinput")
	}
	f, err := os.OpenFile(uinputPath, os.O_RDWR, 0) //nolint:gosec // G304: path is constant
	if err != nil && os.IsPermission(err) {
		return errors.New("cannot open /dev/uinput - permission denied")
	}
	if f != nil {
		_ = f.Close()
	}
	return fmt.Errorf("uinput device setup failed (code %d)", code)
}

// keyHoldDuration is the press-to-release delay. Some native Linux games
// (notably SDL apps) drop sub-millisecond key events for action keys like
// Enter, Backspace, and Esc. Letter keys often slip through via the
// IME/TextInput path, but action keys go through KeyDown/KeyUp only and
// need a real hold to register. xdotool defaults to 12ms for the same
// reason.
var keyHoldDuration = 12 * time.Millisecond // a var so tests can set 0

func (inj *Injector) PressKey(code int, modifiers []int) {
	Debugf("Inject key=%d mods=%v", code, modifiers)
	for _, m := range modifiers {
		inj.writeEvent(evKey, uint16(m), 1) //nolint:gosec // G115: evdev key codes fit in uint16
	}
	inj.syn()
	inj.writeEvent(evKey, uint16(code), 1) //nolint:gosec // G115: evdev key codes fit in uint16
	inj.syn()
	time.Sleep(keyHoldDuration)
	inj.writeEvent(evKey, uint16(code), 0) //nolint:gosec // G115: evdev key codes fit in uint16
	inj.syn()
	for _, m := range modifiers {
		inj.writeEvent(evKey, uint16(m), 0) //nolint:gosec // G115: evdev key codes fit in uint16
	}
	inj.syn()
}

func (inj *Injector) TypeUnicode(codepoint int) {
	// Ctrl+Shift+U, hex digits, Enter (GTK/Qt Unicode input)
	inj.writeEvent(evKey, KEY_LEFTCTRL, 1)
	inj.writeEvent(evKey, KEY_LEFTSHIFT, 1)
	inj.syn()
	inj.writeEvent(evKey, KEY_U, 1)
	inj.syn()
	inj.writeEvent(evKey, KEY_U, 0)
	inj.syn()
	inj.writeEvent(evKey, KEY_LEFTSHIFT, 0)
	inj.writeEvent(evKey, KEY_LEFTCTRL, 0)
	inj.syn()

	time.Sleep(20 * time.Millisecond)

	hexStr := fmt.Sprintf("%04x", codepoint)
	hexKeys := map[byte]uint16{
		'0': KEY_0, '1': KEY_1, '2': KEY_2, '3': KEY_3, '4': KEY_4,
		'5': KEY_5, '6': KEY_6, '7': KEY_7, '8': KEY_8, '9': KEY_9,
		'a': KEY_A, 'b': KEY_B, 'c': KEY_C, 'd': KEY_D, 'e': KEY_E, 'f': KEY_F,
	}
	for _, ch := range []byte(hexStr) {
		code := hexKeys[ch]
		inj.writeEvent(evKey, code, 1)
		inj.syn()
		inj.writeEvent(evKey, code, 0)
		inj.syn()
	}

	inj.writeEvent(evKey, KEY_ENTER, 1)
	inj.syn()
	inj.writeEvent(evKey, KEY_ENTER, 0)
	inj.syn()
}

func (inj *Injector) HoldKey(code int) {
	Debugf("Inject hold key=%d", code)
	inj.writeEvent(evKey, uint16(code), 1) //nolint:gosec // G115: evdev key codes fit in uint16
	inj.syn()
}

func (inj *Injector) ReleaseKey(code int) {
	Debugf("Inject release key=%d", code)
	inj.writeEvent(evKey, uint16(code), 0) //nolint:gosec // G115: evdev key codes fit in uint16
	inj.syn()
}

func (inj *Injector) ClickMouse(button uint16, pressed bool) {
	val := int32(0)
	if pressed {
		val = 1
	}
	inj.writeEvent(evKey, button, val)
	inj.syn()
}

func (inj *Injector) MoveMouse(dx, dy int) {
	if dx != 0 {
		inj.writeEvent(evRel, relX, int32(dx)) //nolint:gosec // G115: a few pixels per pass
	}
	if dy != 0 {
		inj.writeEvent(evRel, relY, int32(dy)) //nolint:gosec // G115: a few pixels per pass
	}
	if dx != 0 || dy != 0 {
		inj.syn()
	}
}

func (inj *Injector) Close() {
	if inj.fd != nil {
		_ = ioctl(int(inj.fd.Fd()), uiDevDestroy, 0)
		_ = inj.fd.Close()
		inj.fd = nil
	}
}

func (inj *Injector) writeEvent(evType uint16, code uint16, value int32) {
	// struct input_event on x86_64: sec(8) + usec(8) + type(2) + code(2) + value(4) = 24 bytes
	var buf [24]byte
	binary.LittleEndian.PutUint16(buf[16:], evType)
	binary.LittleEndian.PutUint16(buf[18:], code)
	binary.LittleEndian.PutUint32(buf[20:], uint32(value)) //nolint:gosec // G115: same bits, as the kernel reads them
	_, _ = inj.fd.Write(buf[:])
}

func (inj *Injector) syn() {
	inj.writeEvent(evSyn, synReport, 0)
}
