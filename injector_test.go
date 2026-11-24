package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unsafe"
)

func TestUinputError_PermissionDenied_NotInGroup(t *testing.T) {
	err := uinputError(-1)
	if err == nil {
		t.Fatal("uinputError(-1) returned nil")
	}
	msg := err.Error()
	if msg == "" {
		t.Error("uinputError returned empty error message")
	}
}

func TestUinputError_SetupFailed(t *testing.T) {
	err := uinputError(-2)
	if err == nil {
		t.Fatal("uinputError(-2) returned nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "uinput") {
		t.Errorf("error message should mention uinput: %q", msg)
	}
}

func TestUinputError_CreateFailed(t *testing.T) {
	err := uinputError(-3)
	if err == nil {
		t.Fatal("uinputError(-3) returned nil")
	}
}

func TestUinputError_SingleLine(t *testing.T) {
	// All error codes must return single-line errors (no embedded \n)
	// to ensure proper log formatting with timestamps on each line
	for _, code := range []int{-1, -2, -3} {
		err := uinputError(code)
		if err == nil {
			t.Fatalf("uinputError(%d) returned nil", code)
		}
		if strings.Contains(err.Error(), "\n") {
			t.Errorf("uinputError(%d) contains newline: %q", code, err.Error())
		}
	}
}

func TestLogPermissionFix_NoPanic(_ *testing.T) {
	// logPermissionFix should not panic regardless of group membership
	logPermissionFix()
}

func TestColorHelpers(t *testing.T) {
	// Verify color functions return non-empty strings
	msg := "test message"
	if colorRed(msg) == "" {
		t.Error("colorRed returned empty")
	}
	if colorYellow(msg) == "" {
		t.Error("colorYellow returned empty")
	}
	if colorGreen(msg) == "" {
		t.Error("colorGreen returned empty")
	}
	// All must contain the original message
	if !strings.Contains(colorRed(msg), msg) {
		t.Error("colorRed dropped the message content")
	}
	if !strings.Contains(colorYellow(msg), msg) {
		t.Error("colorYellow dropped the message content")
	}
	if !strings.Contains(colorGreen(msg), msg) {
		t.Error("colorGreen dropped the message content")
	}
	if colorDim(msg) == "" {
		t.Error("colorDim returned empty")
	}
	if !strings.Contains(colorDim(msg), msg) {
		t.Error("colorDim dropped the message content")
	}
}

func TestNilInjector_PressCurrent(_ *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	kb.PressCurrent(nil) // should not panic
}

func TestNilInjector_PressCurrentAccent(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	kb.AccentPopup = &AccentPopupState{
		Accents:  []AccentDef{{"á", 0xe1}},
		Selected: 0,
	}
	kb.PressCurrent(nil) // should not panic, should close popup
	if kb.AccentPopup != nil {
		t.Error("accent popup should be closed after PressCurrent")
	}
}

func TestNilInjector_PressCurrentCombo(_ *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	kb.CursorRow = 0
	kb.CursorCol = 0
	kb.PressCurrent(nil) // should not panic
}

func TestNilInjector_PressCurrentPaste(t *testing.T) {
	kb := NewKeyboardState(LayoutQWERTY)
	for r, row := range LayoutQWERTY {
		for c, key := range row {
			if key.Label == "Paste" {
				kb.CursorRow = r
				kb.CursorCol = c
				kb.PressCurrent(nil) // should not panic
				return
			}
		}
	}
	t.Skip("Paste key not found in layout")
}

// The device the kernel made matches what createUinputDevice asked for:
// name, ids, event types, every key and mouse button, both mouse axes.
// It is gone after Close. Skips without access to /dev/uinput.
func TestUinputDevice(t *testing.T) {
	f, err := os.OpenFile(uinputPath, os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no uinput: %v", err)
	}
	_ = f.Close()
	inj, err := NewInjector()
	if err != nil {
		t.Fatal(err)
	}
	defer inj.Close()
	var name [64]byte
	if err := ioctlPtr(int(inj.fd.Fd()), 0x8040552c, unsafe.Pointer(&name[0])); err != nil { //nolint:gosec // G103: UI_GET_SYSNAME(64) fills a buffer
		t.Fatalf("UI_GET_SYSNAME: %v", err)
	}
	dir := "/sys/devices/virtual/input/" + strings.TrimRight(string(name[:]), "\x00")
	read := func(f string) string {
		b, err := os.ReadFile(dir + "/" + f) //nolint:gosec // G304: the device's own sysfs files
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(b))
	}
	for f, want := range map[string]string{
		"name": "gamepad-osk", "id/bustype": "0003", "id/vendor": "1234", "id/product": "5678", "id/version": "0001",
		"capabilities/ev": "7", "capabilities/rel": "3",
	} {
		if got := read(f); got != want {
			t.Errorf("%s = %q, want %q", f, got, want)
		}
	}
	// capabilities/key: hex words, highest first, each 64 bits
	words := strings.Fields(read("capabilities/key"))
	has := func(code int) bool {
		i := len(words) - 1 - code/64
		if i < 0 {
			return false
		}
		var w uint64
		if _, err := fmt.Sscanf(words[i], "%x", &w); err != nil {
			t.Fatal(err)
		}
		return w&(1<<(code%64)) != 0
	}
	for code := range 0x120 {
		want := (code >= 1 && code < 249) || (code >= 0x110 && code <= 0x112)
		if has(code) != want {
			t.Errorf("key %#x declared %v, want %v", code, has(code), want)
		}
	}
	inj.Close()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("%s still there after Close (err %v)", dir, err)
	}
}
