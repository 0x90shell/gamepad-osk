package main

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Theme.Name != "dark" {
		t.Errorf("default theme = %q, want dark", cfg.Theme.Name)
	}
	if cfg.Window.Position != "bottom" {
		t.Errorf("default position = %q, want bottom", cfg.Window.Position)
	}
	if cfg.Window.Opacity != 0.95 {
		t.Errorf("default opacity = %f, want 0.95", cfg.Window.Opacity)
	}
	if cfg.Gamepad.Deadzone != 0.25 {
		t.Errorf("default deadzone = %f, want 0.25", cfg.Gamepad.Deadzone)
	}
	if cfg.Gamepad.SwapXY != "auto" {
		t.Errorf("default swap_xy = %q, want auto", cfg.Gamepad.SwapXY)
	}
	if cfg.Gamepad.Buttons.Press != "a" {
		t.Errorf("default press = %q, want a", cfg.Gamepad.Buttons.Press)
	}
	if cfg.Gamepad.Buttons.LeftClick != "rb" {
		t.Errorf("default left_click = %q, want rb", cfg.Gamepad.Buttons.LeftClick)
	}
	if cfg.Gamepad.Buttons.PositionToggle != "start" {
		t.Errorf("default position_toggle = %q, want start", cfg.Gamepad.Buttons.PositionToggle)
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name     string
		modify   func(*Config)
		checkFn  func(*Config) bool
		desc     string
	}{
		{"deadzone too high", func(c *Config) { c.Gamepad.Deadzone = 2.0 },
			func(c *Config) bool { return c.Gamepad.Deadzone == 0.25 }, "should reset to 0.25"},
		{"deadzone negative", func(c *Config) { c.Gamepad.Deadzone = -0.5 },
			func(c *Config) bool { return c.Gamepad.Deadzone == 0.25 }, "should reset to 0.25"},
		{"deadzone valid", func(c *Config) { c.Gamepad.Deadzone = 0.5 },
			func(c *Config) bool { return c.Gamepad.Deadzone == 0.5 }, "should keep 0.5"},
		{"sensitivity too low", func(c *Config) { c.Mouse.Sensitivity = 0 },
			func(c *Config) bool { return c.Mouse.Sensitivity == 8 }, "should reset to 8"},
		{"sensitivity too high", func(c *Config) { c.Mouse.Sensitivity = 100 },
			func(c *Config) bool { return c.Mouse.Sensitivity == 8 }, "should reset to 8"},
		{"long_press_ms too low", func(c *Config) { c.Gamepad.LongPressMs = 10 },
			func(c *Config) bool { return c.Gamepad.LongPressMs == 500 }, "should reset to 500"},
		{"unknown theme", func(c *Config) { c.Theme.Name = "nonexistent" },
			func(c *Config) bool { return c.Theme.Name == "dark" }, "should reset to dark"},
		{"valid theme", func(c *Config) { c.Theme.Name = "nord" },
			func(c *Config) bool { return c.Theme.Name == "nord" }, "should keep nord"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(&cfg)
			ValidateConfig(&cfg)
			if !tt.checkFn(&cfg) {
				t.Errorf("%s: %s", tt.name, tt.desc)
			}
		})
	}
}

func TestButtonTable(t *testing.T) {
	// Without swap
	table := buttonTable(false)
	if table["x"].EvdevBtn != BTN_WEST {
		t.Errorf("x without swap = %d, want BTN_WEST(%d)", table["x"].EvdevBtn, BTN_WEST)
	}
	if table["y"].EvdevBtn != BTN_NORTH {
		t.Errorf("y without swap = %d, want BTN_NORTH(%d)", table["y"].EvdevBtn, BTN_NORTH)
	}

	// With swap
	table = buttonTable(true)
	if table["x"].EvdevBtn != BTN_NORTH {
		t.Errorf("x with swap = %d, want BTN_NORTH(%d)", table["x"].EvdevBtn, BTN_NORTH)
	}
	if table["y"].EvdevBtn != BTN_WEST {
		t.Errorf("y with swap = %d, want BTN_WEST(%d)", table["y"].EvdevBtn, BTN_WEST)
	}
}

func TestBuildActionMap(t *testing.T) {
	cfg := DefaultConfig()
	am := BuildActionMap(cfg.Gamepad)

	// Check that A button maps to press (special handling)
	if _, ok := am.BtnPress[BTN_SOUTH]; !ok {
		t.Error("A button (BTN_SOUTH) not in press map")
	}

	// Check mouse buttons (default: left_click=rb, right_click=lb)
	if am.BtnPress[BTN_TR] != ActionLeftClick {
		t.Errorf("RB = %v, want ActionLeftClick", am.BtnPress[BTN_TR])
	}
	if am.BtnRelease[BTN_TR] != ActionLeftClickRelease {
		t.Errorf("RB release = %v, want ActionLeftClickRelease", am.BtnRelease[BTN_TR])
	}

	// Check position toggle
	if am.BtnPress[0x13b] != ActionPositionToggle {
		t.Errorf("Start = %v, want ActionPositionToggle", am.BtnPress[0x13b])
	}
}

func TestBuildKeyGlyphs(t *testing.T) {
	cfg := DefaultConfig()
	glyphs := BuildKeyGlyphs(cfg.Gamepad)

	if glyphs[KEY_BACKSPACE] == "" {
		t.Error("backspace should have a glyph")
	}
	if glyphs[KEY_SPACE] == "" {
		t.Error("space should have a glyph")
	}
	if glyphs[KEY_ENTER] == "" {
		t.Error("enter should have a glyph")
	}
	if glyphs[0] != "\u2699" {
		t.Errorf("cfg key glyph = %q, want gear icon", glyphs[0])
	}
}

func TestAllThemesHaveRequiredFields(t *testing.T) {
	zero := [4]uint8{0, 0, 0, 0}
	for name, theme := range Themes {
		if theme.Name != name {
			t.Errorf("theme %q: Name field = %q", name, theme.Name)
		}
		// ModifierActiveText should not be zero (invisible)
		mat := theme.ModifierActiveText
		if mat.R == zero[0] && mat.G == zero[1] && mat.B == zero[2] && mat.A == zero[3] {
			t.Errorf("theme %q: ModifierActiveText is zero (invisible)", name)
		}
	}
}

func TestIsSwapXY(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gamepad.SwapXY = "true"
	if !isSwapXY(cfg.Gamepad) {
		t.Error("swap_xy=true should return true")
	}
	cfg.Gamepad.SwapXY = "false"
	if isSwapXY(cfg.Gamepad) {
		t.Error("swap_xy=false should return false")
	}
	cfg.Gamepad.SwapXY = "auto"
	if isSwapXY(cfg.Gamepad) {
		t.Error("swap_xy=auto should return false (before detection)")
	}
}
