package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestDetectProfile(t *testing.T) {
	tests := []struct {
		name string
		info deviceInfo
		want string
	}{
		{"xpad driver", deviceInfo{Name: "Generic X-Box pad", Driver: "xpad", Vendor: "045e"}, ProfileXbox},
		{"xpadneo", deviceInfo{Name: "Xbox Wireless Controller", Driver: "xpadneo"}, ProfileXbox},
		{"xone", deviceInfo{Driver: "xone"}, ProfileXbox},
		{"8bitdo in xinput mode", deviceInfo{Name: "8BitDo Ultimate", Driver: "xpad", Vendor: "2dc8"}, ProfileXbox},
		{"uinput xbox by name", deviceInfo{Name: "Microsoft X-Box 360 pad"}, ProfileXbox},
		{"dualsense driver", deviceInfo{Name: "Sony Interactive Entertainment DualSense Wireless Controller", Driver: "playstation", Vendor: "054c"}, ProfilePlayStation},
		{"ds3 hid-sony", deviceInfo{Name: "Sony PLAYSTATION(R)3 Controller", Driver: "sony"}, ProfilePlayStation},
		{"sony vendor only", deviceInfo{Name: "Wireless Controller", Vendor: "054c"}, ProfilePlayStation},
		{"uinput dualshock by name", deviceInfo{Name: "DualShock 4 (virtual)"}, ProfilePlayStation},
		{"switch pro driver", deviceInfo{Name: "Nintendo Switch Pro Controller", Driver: "nintendo"}, ProfileNintendo},
		{"nintendo vendor", deviceInfo{Name: "Some pad", Vendor: "057e"}, ProfileNintendo},
		{"joy-con by name", deviceInfo{Name: "Joy-Con (L)"}, ProfileNintendo},
		{"8bitdo bluetooth dinput", deviceInfo{Name: "8BitDo Pro 2", Driver: "hid-generic", Vendor: "2dc8"}, Profile8BitDo},
		{"8bitdo by name", deviceInfo{Name: "8BitDo SN30 Pro"}, Profile8BitDo},
		{"microsoft vendor without xpad", deviceInfo{Name: "Controller", Driver: "microsoft", Vendor: "045e"}, ProfileGeneric},
		{"unknown hid-generic", deviceInfo{Name: "USB Gamepad", Driver: "hid-generic", Vendor: "0079"}, ProfileGeneric},
		{"nothing known", deviceInfo{}, ProfileGeneric},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectProfile(tt.info); got != tt.want {
				t.Errorf("detectProfile(%+v) = %q, want %q", tt.info, got, tt.want)
			}
		})
	}
}

// The profile is Xbox exactly when the swap_xy auto rule fires.
func TestDetectProfileAgreesWithSwapXY(t *testing.T) {
	infos := []deviceInfo{
		{Driver: "xpad"}, {Driver: "xpadneo"}, {Driver: "xone"},
		{Name: "Xbox Wireless Controller"}, {Name: "Microsoft X-Box One pad"},
		{Driver: "playstation"}, {Vendor: "045e"}, {Name: "USB Gamepad"}, {},
	}
	for _, info := range infos {
		swap := isXpadDriver(info.Driver) || isXboxName(info.Name)
		if xbox := detectProfile(info) == ProfileXbox; xbox != swap {
			t.Errorf("%+v: profile xbox = %v, swap_xy rule = %v", info, xbox, swap)
		}
	}
}

func TestProfilesHaveAllLabels(t *testing.T) {
	buttons := []string{"a", "b", "x", "y", "lb", "rb", "lt", "rt", "l3", "r3", "select", "start", "guide"}
	for id, p := range ControllerProfiles {
		if p.ID != id {
			t.Errorf("profile %q has ID %q", id, p.ID)
		}
		for _, b := range buttons {
			if p.Labels[b] == "" {
				t.Errorf("profile %q: no label for %q", id, b)
			}
		}
	}
}

func TestProfileLabel(t *testing.T) {
	ps := GetControllerProfile(ProfilePlayStation)
	if got := ps.Label("a"); got != "✕" {
		t.Errorf("playstation a = %q, want ✕", got)
	}
	if got := GetControllerProfile(ProfileNintendo).Label("a"); got != "B" {
		t.Errorf("nintendo a = %q, want B", got)
	}
	if got := ps.Label("dpad_up"); got != "↑" {
		t.Errorf("dpad_up = %q, want ↑", got)
	}
	if got := GetControllerProfile("bogus").ID; got != ProfileGeneric {
		t.Errorf("unknown profile falls back to %q, want generic", got)
	}
	if got := GetControllerProfile("XBOX").ID; got != ProfileXbox {
		t.Errorf("profile lookup is case-sensitive: got %q", got)
	}
}

// A profile only changes names. The input mapping must not depend on it.
func TestActionMapSameForEveryProfile(t *testing.T) {
	build := func(controller string) *GamepadReader {
		cfg := DefaultConfig()
		cfg.Gamepad.Controller = controller
		cfg.Gamepad.SwapXY = "true"
		gp := NewGamepadReader(cfg)
		gp.initButtonMap()
		return gp
	}
	ref := build(ProfileAuto)
	for id := range ControllerProfiles {
		gp := build(id)
		if !reflect.DeepEqual(gp.actionMap, ref.actionMap) {
			t.Errorf("profile %q changes the action map", id)
		}
		if gp.pressBtn != ref.pressBtn || gp.shiftAxis != ref.shiftAxis {
			t.Errorf("profile %q changes press or shift", id)
		}
	}
}

func TestControllerConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Gamepad.Controller != ProfileAuto {
		t.Errorf("default controller = %q, want auto", cfg.Gamepad.Controller)
	}

	if err := parseINI(strings.NewReader("[gamepad]\ncontroller = PlayStation\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Gamepad.Controller != ProfilePlayStation {
		t.Errorf("parsed controller = %q, want playstation", cfg.Gamepad.Controller)
	}

	var buf strings.Builder
	if err := writeINI(&buf, cfg); err != nil {
		t.Fatal(err)
	}
	cfg2 := DefaultConfig()
	if err := parseINI(strings.NewReader(buf.String()), &cfg2); err != nil {
		t.Fatal(err)
	}
	if cfg2.Gamepad.Controller != ProfilePlayStation {
		t.Errorf("round-trip controller = %q, want playstation", cfg2.Gamepad.Controller)
	}

	cfg.Gamepad.Controller = "sega"
	if issues := checkConfig(cfg); !containsSubstr(issues, "controller") {
		t.Errorf("checkConfig did not flag controller: %v", issues)
	}
	ValidateConfig(&cfg)
	if cfg.Gamepad.Controller != ProfileAuto {
		t.Errorf("invalid controller became %q, want auto", cfg.Gamepad.Controller)
	}
}

func containsSubstr(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
