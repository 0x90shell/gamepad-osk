package main

/*
#include <linux/uinput.h>
#include <sys/ioctl.h>
#include <fcntl.h>
#include <unistd.h>
#include <string.h>
#include <stdio.h>

int create_uinput_device() {
    int fd = open("/dev/uinput", O_RDWR | O_NONBLOCK);
    if (fd < 0) return -1;

    ioctl(fd, UI_SET_EVBIT, EV_KEY);
    ioctl(fd, UI_SET_EVBIT, EV_REL);
    ioctl(fd, UI_SET_EVBIT, EV_SYN);

    // Keyboard keys
    for (int i = 1; i < 249; i++)
        ioctl(fd, UI_SET_KEYBIT, i);

    // Mouse buttons
    ioctl(fd, UI_SET_KEYBIT, BTN_LEFT);
    ioctl(fd, UI_SET_KEYBIT, BTN_RIGHT);
    ioctl(fd, UI_SET_KEYBIT, BTN_MIDDLE);

    // Mouse axes
    ioctl(fd, UI_SET_RELBIT, REL_X);
    ioctl(fd, UI_SET_RELBIT, REL_Y);

    struct uinput_setup usetup;
    memset(&usetup, 0, sizeof(usetup));
    strncpy(usetup.name, "gamepad-osk", sizeof(usetup.name) - 1);
    usetup.id.bustype = BUS_USB;
    usetup.id.vendor  = 0x1234;
    usetup.id.product = 0x5678;
    usetup.id.version = 1;

    if (ioctl(fd, UI_DEV_SETUP, &usetup) < 0) {
        close(fd);
        return -2;
    }
    if (ioctl(fd, UI_DEV_CREATE) < 0) {
        close(fd);
        return -3;
    }
    return fd;
}

void destroy_uinput_device(int fd) {
    ioctl(fd, UI_DEV_DESTROY);
    close(fd);
}
*/
import "C"

import (
	"encoding/binary"
	"fmt"
	"os"
	"time"
)

const (
	uinputPath = "/dev/uinput"
	evKey      = 0x01
	evRel      = 0x02
	evSyn      = 0x00
	relX       = 0x00
	relY       = 0x01
	synReport  = 0x00
)

type Injector struct {
	fd *os.File
}

func NewInjector() (*Injector, error) {
	cfd := C.create_uinput_device()
	if cfd < 0 {
		return nil, fmt.Errorf("create uinput device failed (code %d) — are you in the 'input' group?", cfd)
	}

	fd := os.NewFile(uintptr(cfd), uinputPath)
	time.Sleep(50 * time.Millisecond)

	return &Injector{fd: fd}, nil
}

func (inj *Injector) PressKey(code int, modifiers []int) {
	Debugf("Inject key=%d mods=%v", code, modifiers)
	for _, m := range modifiers {
		inj.writeEvent(evKey, uint16(m), 1)
	}
	inj.syn()
	inj.writeEvent(evKey, uint16(code), 1)
	inj.syn()
	inj.writeEvent(evKey, uint16(code), 0)
	inj.syn()
	for _, m := range modifiers {
		inj.writeEvent(evKey, uint16(m), 0)
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
		inj.writeEvent(evRel, relX, int32(dx))
	}
	if dy != 0 {
		inj.writeEvent(evRel, relY, int32(dy))
	}
	if dx != 0 || dy != 0 {
		inj.syn()
	}
}

func (inj *Injector) Close() {
	if inj.fd != nil {
		C.destroy_uinput_device(C.int(inj.fd.Fd()))
		inj.fd = nil
	}
}

func (inj *Injector) writeEvent(evType uint16, code uint16, value int32) {
	// struct input_event on x86_64: sec(8) + usec(8) + type(2) + code(2) + value(4) = 24 bytes
	var buf [24]byte
	binary.LittleEndian.PutUint16(buf[16:], evType)
	binary.LittleEndian.PutUint16(buf[18:], code)
	binary.LittleEndian.PutUint32(buf[20:], uint32(value))
	inj.fd.Write(buf[:])
}

func (inj *Injector) syn() {
	inj.writeEvent(evSyn, synReport, 0)
}

