package main

import "testing"

// U47: hasJoystickButtons looks at BTN_JOYSTICK to BTN_THUMBR (0x120 to 0x13f)
// only. A touchpad node (the DS4 and DualSense Touchpad, a laptop pad) has
// BTN_TOUCH (0x14a) and BTN_TOOL_* (0x145 and up), and must not count as a
// pad. The bitmaps are built by hand from the kernel's key-bit layout: the
// words print most significant first, 64 bits a word.
func TestHasJoystickButtonsByKeyBits(t *testing.T) {
	for _, tt := range []struct {
		name string
		keys string
		want bool
	}{
		{"touchpad: BTN_TOUCH only", "400 0 0 0 0 0", false},
		{"touchpad: BTN_TOUCH and BTN_TOOL_FINGER", "420 0 0 0 0 0", false}, // 0x145, 0x14a
		{"gamepad: A B X Y", "1b000000000000 0 0 0 0", true},                // 0x130 to 0x134
		{"gamepad: BTN_TRIGGER (0x120) only", "100000000 0 0 0 0", true},
		{"gamepad: BTN_THUMBR (0x13e) only", "4000000000000000 0 0 0 0", true},
		{"mouse buttons only", "1f0000 0 0 0 0", false}, // BTN_LEFT 0x110 to 0x114
		{"one past the range (0x140)", "1 0 0 0 0 0", false},
		{"no key bits", "0", false},
		{"empty", "", false},
	} {
		if got := hasJoystickButtons(tt.keys); got != tt.want {
			t.Errorf("%s (%q): %v, want %v", tt.name, tt.keys, got, tt.want)
		}
	}
}
