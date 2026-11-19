package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Theme.Name != "matrix" {
		t.Errorf("default theme = %q, want matrix", cfg.Theme.Name)
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
	if cfg.Theme.KeyStyle != "square" {
		t.Errorf("default key_style = %q, want square", cfg.Theme.KeyStyle)
	}
	if !cfg.Theme.KeyShadow {
		t.Error("default key_shadow = false, want true")
	}
}

func TestParseKeyStyleAndShadow(t *testing.T) {
	input := "[theme]\nname = light\nkey_style = rounded\nkey_shadow = false\n"
	cfg := DefaultConfig()
	if err := parseINI(strings.NewReader(input), &cfg); err != nil {
		t.Fatalf("parseINI error: %v", err)
	}
	if cfg.Theme.KeyStyle != "rounded" {
		t.Errorf("key_style = %q, want rounded", cfg.Theme.KeyStyle)
	}
	if cfg.Theme.KeyShadow {
		t.Error("key_shadow = true, want false")
	}

	// Invalid bool keeps the default
	cfg = DefaultConfig()
	if err := parseINI(strings.NewReader("[theme]\nkey_shadow = maybe\n"), &cfg); err != nil {
		t.Fatalf("parseINI error: %v", err)
	}
	if !cfg.Theme.KeyShadow {
		t.Error("key_shadow = false after invalid value, want the default true")
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		checkFn func(*Config) bool
		desc    string
	}{
		{"deadzone too high", func(c *Config) { c.Gamepad.Deadzone = 2.0 },
			func(c *Config) bool { return c.Gamepad.Deadzone == 0.25 }, "should reset to 0.25"},
		{"deadzone negative", func(c *Config) { c.Gamepad.Deadzone = -0.5 },
			func(c *Config) bool { return c.Gamepad.Deadzone == 0.25 }, "should reset to 0.25"},
		{"deadzone valid", func(c *Config) { c.Gamepad.Deadzone = 0.5 },
			func(c *Config) bool { return c.Gamepad.Deadzone == 0.5 }, "should keep 0.5"},
		{"sensitivity too low", func(c *Config) { c.Mouse.Sensitivity = 0 },
			func(c *Config) bool { return c.Mouse.Sensitivity == 10 }, "should reset to 10"},
		{"sensitivity too high", func(c *Config) { c.Mouse.Sensitivity = 100 },
			func(c *Config) bool { return c.Mouse.Sensitivity == 10 }, "should reset to 10"},
		{"long_press_ms too low", func(c *Config) { c.Gamepad.LongPressMs = 10 },
			func(c *Config) bool { return c.Gamepad.LongPressMs == 500 }, "should reset to 500"},
		{"unknown theme", func(c *Config) { c.Theme.Name = "nonexistent" },
			func(c *Config) bool { return c.Theme.Name == "matrix" }, "should reset to matrix"},
		{"valid theme", func(c *Config) { c.Theme.Name = "nord" },
			func(c *Config) bool { return c.Theme.Name == "nord" }, "should keep nord"},
		{"unknown key_style", func(c *Config) { c.Theme.KeyStyle = "round" },
			func(c *Config) bool { return c.Theme.KeyStyle == "square" }, "should reset to square"},
		{"rounded key_style", func(c *Config) { c.Theme.KeyStyle = "rounded" },
			func(c *Config) bool { return c.Theme.KeyStyle == "rounded" }, "should keep rounded"},
		{"opacity zero", func(c *Config) { c.Window.Opacity = 0 },
			func(c *Config) bool { return c.Window.Opacity == 0.2 }, "should raise to 0.2"},
		{"opacity negative", func(c *Config) { c.Window.Opacity = -1 },
			func(c *Config) bool { return c.Window.Opacity == 0.2 }, "should raise to 0.2"},
		{"opacity at floor", func(c *Config) { c.Window.Opacity = 0.2 },
			func(c *Config) bool { return c.Window.Opacity == 0.2 }, "should keep 0.2"},
		{"opacity valid", func(c *Config) { c.Window.Opacity = 0.5 },
			func(c *Config) bool { return c.Window.Opacity == 0.5 }, "should keep 0.5"},
		{"opacity too high", func(c *Config) { c.Window.Opacity = 1.5 },
			func(c *Config) bool { return c.Window.Opacity == 1 }, "should lower to 1"},
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

func TestBuildActionMap_PositionToggleDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gamepad.Buttons.PositionToggle = ""
	am := BuildActionMap(cfg.Gamepad)

	// ActionPositionToggle must not appear in any button slot
	for code, action := range am.BtnPress {
		if action == ActionPositionToggle {
			t.Errorf("ActionPositionToggle mapped to button 0x%x with empty position_toggle", code)
		}
	}
	for code, action := range am.BtnRelease {
		if action == ActionPositionToggle {
			t.Errorf("ActionPositionToggle mapped to release 0x%x with empty position_toggle", code)
		}
	}
}

func TestPosToggleLabel(t *testing.T) {
	if got := posToggleLabel("start"); got != "Start" {
		t.Errorf("posToggleLabel(start) = %q, want Start", got)
	}
	if got := posToggleLabel(""); got != "(disabled)" {
		t.Errorf("posToggleLabel(\"\") = %q, want (disabled)", got)
	}
}

func TestMouseLbl(t *testing.T) {
	if got := mouseLbl(true, "Right stick"); got != "Right stick" {
		t.Errorf("mouseLbl(true, ...) = %q, want label unchanged", got)
	}
	if got := mouseLbl(false, "Right stick"); got != "(disabled)" {
		t.Errorf("mouseLbl(false, ...) = %q, want (disabled)", got)
	}
}

// captureHelp prints help for cfg and returns the output.
func captureHelp(t *testing.T, cfg Config) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	printHelp(cfg)
	_ = w.Close()
	os.Stdout = origStdout
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	return buf.String()
}

func TestPrintHelpComboSection(t *testing.T) {
	// Disabled combo shows setup instructions
	cfg := DefaultConfig()
	cfg.Gamepad.ToggleCombo = ""
	out := captureHelp(t, cfg)
	if !strings.Contains(out, "Disabled.") {
		t.Error("help with no toggle_combo should show Disabled")
	}
	if strings.Contains(out, "Active:") {
		t.Error("help with no toggle_combo should not show Active")
	}

	// Configured combo shows active line
	cfg.Gamepad.ToggleCombo = "select+start"
	cfg.Gamepad.ComboPeriodMs = 200
	out = captureHelp(t, cfg)
	if !strings.Contains(out, "Active:") {
		t.Error("help with toggle_combo set should show Active")
	}
	if !strings.Contains(out, "select+start") {
		t.Error("help should show configured combo string")
	}
	if strings.Contains(out, "Disabled.") {
		t.Error("help with toggle_combo set should not show Disabled")
	}
}

func TestPrintHelpInvalidCombo(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gamepad.ToggleCombo = "select+banana"
	out := captureHelp(t, cfg)
	if !strings.Contains(out, "INVALID") {
		t.Error("help with invalid combo should show INVALID")
	}
	if strings.Contains(out, "Active:") {
		t.Error("help with invalid combo should not show Active")
	}
}

func TestPrintHelpMouseDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mouse.Enabled = false
	out := captureHelp(t, cfg)

	// All 4 mouse rows should show (disabled); position_toggle disabled adds one more
	// so require at least 4
	count := strings.Count(out, "(disabled)")
	if count < 4 {
		t.Errorf("mouse disabled: got %d (disabled) occurrences in help, want >= 4", count)
	}
}

func TestBuildKeyGlyphs(t *testing.T) {
	cfg := DefaultConfig()
	glyphs := BuildKeyGlyphs(cfg.Gamepad)

	if glyphs[KEY_BACKSPACE].Glyph == "" {
		t.Error("backspace should have a glyph")
	}
	if glyphs[KEY_SPACE].Glyph == "" {
		t.Error("space should have a glyph")
	}
	if glyphs[KEY_ENTER].Glyph == "" {
		t.Error("enter should have a glyph")
	}
	if glyphs[0].Glyph != "\u2699" {
		t.Errorf("theme key glyph = %q, want gear icon", glyphs[0].Glyph)
	}

	// Without Promptfont the keys show the mapped button's name
	want := map[int]string{KEY_BACKSPACE: "X", KEY_SPACE: "Y", KEY_ENTER: "RT", KEY_LEFTSHIFT: "LT", KEY_CAPSLOCK: "L3"}
	for code, name := range want {
		if got := glyphs[code].Text; got != name {
			t.Errorf("key %d text = %q, want %q", code, got, name)
		}
	}

	// Remapping changes the name with the glyph
	cfg.Gamepad.Buttons.Backspace = "lb"
	glyphs = BuildKeyGlyphs(cfg.Gamepad)
	if g := glyphs[KEY_BACKSPACE]; g.Text != "LB" || g.Glyph != buttonTable(false)["lb"].Glyph {
		t.Errorf("remapped backspace = %+v, want the lb glyph and LB", g)
	}
}

func TestBuildPopupHint(t *testing.T) {
	table := buttonTable(false)
	cfg := DefaultConfig()
	hint := BuildPopupHint(cfg.Gamepad)
	if len(hint) != 3 || hint[0].Glyph != dpadLeftRight {
		t.Fatalf("default hint = %+v, want choose, type and cancel", hint)
	}
	if hint[1].Glyph != table["a"].Glyph || hint[1].Text != "A" {
		t.Errorf("type = %+v, want the a button", hint[1])
	}
	if hint[2].Glyph != table["b"].Glyph || hint[2].Text != "B" {
		t.Errorf("cancel = %+v, want the b button", hint[2])
	}

	// Remapped buttons are followed
	cfg.Gamepad.Buttons.Press, cfg.Gamepad.Buttons.Close = "x", "y"
	hint = BuildPopupHint(cfg.Gamepad)
	if hint[1].Glyph != table["x"].Glyph || hint[2].Glyph != table["y"].Glyph {
		t.Errorf("remapped hint = %+v, want x and y", hint)
	}

	// An unmapped close button drops the cancel item
	cfg.Gamepad.Buttons.Close = ""
	if hint = BuildPopupHint(cfg.Gamepad); len(hint) != 2 {
		t.Errorf("hint without close = %+v, want 2 items", hint)
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

func TestGetTheme(t *testing.T) {
	dark := GetTheme("dark")
	if dark.Name != "dark" {
		t.Errorf("GetTheme(dark) = %q, want dark", dark.Name)
	}
	nord := GetTheme("nord")
	if nord.Name != "nord" {
		t.Errorf("GetTheme(nord) = %q, want nord", nord.Name)
	}
	fallback := GetTheme("nonexistent")
	if fallback.Name != "dark" {
		t.Errorf("GetTheme(nonexistent) = %q, want dark fallback", fallback.Name)
	}
}

func TestThemeCount(t *testing.T) {
	if len(Themes) != 62 {
		t.Errorf("theme count = %d, want 62", len(Themes))
	}
	// Spot-check mid-range and phone-style themes
	for _, name := range []string{"chalk", "fjord", "sand", "plum", "moss", "light", "night"} {
		if _, ok := Themes[name]; !ok {
			t.Errorf("missing theme %q", name)
		}
	}
}

func TestThemeOrder(t *testing.T) {
	if !sort.StringsAreSorted(themeOrder) {
		t.Error("themeOrder is not sorted")
	}
	if len(themeOrder) != len(Themes) {
		t.Errorf("themeOrder has %d names, want %d", len(themeOrder), len(Themes))
	}
	for _, name := range []string{"light", "night"} {
		if !slices.Contains(themeOrder, name) {
			t.Errorf("themeOrder missing %q", name)
		}
	}
}

func TestThemeUniqueBgColors(t *testing.T) {
	type bgKey struct{ r, g, b uint8 }
	seen := make(map[bgKey]string)
	for name, theme := range Themes {
		key := bgKey{theme.Bg.R, theme.Bg.G, theme.Bg.B}
		if other, ok := seen[key]; ok {
			// Black backgrounds are shared by retro/terminal/etc - skip (0,0,0)
			if key.r == 0 && key.g == 0 && key.b == 0 {
				continue
			}
			t.Errorf("themes %q and %q share bg color (%d,%d,%d)", name, other, key.r, key.g, key.b)
		}
		seen[key] = name
	}
}

func TestValidateRepeatConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Keys.RepeatDelayMs = 50 // too low
	ValidateConfig(&cfg)
	if cfg.Keys.RepeatDelayMs != 400 {
		t.Errorf("repeat_delay_ms = %d, want 400 after validation", cfg.Keys.RepeatDelayMs)
	}

	cfg.Keys.RepeatRateMs = 5 // too low
	ValidateConfig(&cfg)
	if cfg.Keys.RepeatRateMs != 80 {
		t.Errorf("repeat_rate_ms = %d, want 80 after validation", cfg.Keys.RepeatRateMs)
	}
}

func TestValidateScale(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Keys.Scale = 0
	ValidateConfig(&cfg)
	if cfg.Keys.Scale != 50 {
		t.Errorf("scale = %d, want 50 after validation", cfg.Keys.Scale)
	}

	cfg.Keys.Scale = 200
	ValidateConfig(&cfg)
	if cfg.Keys.Scale != 50 {
		t.Errorf("scale = %d, want 50 after validation", cfg.Keys.Scale)
	}
}

func TestSaveConfigRoundTrip(t *testing.T) {
	// Use a temp dir to avoid touching real config
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	resetConfigState(t)

	// Save a theme
	SaveTheme("nord")
	path := filepath.Join(tmpDir, "gamepad-osk", "config")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	// Verify file is INI format (no TOML quotes)
	data, err := os.ReadFile(path) //nolint:gosec // G304: test path
	if err != nil {
		t.Fatalf("cannot read config: %v", err)
	}
	if strings.Contains(string(data), `"nord"`) {
		t.Error("config contains TOML-style quoted string")
	}

	// Save position
	SavePosition(true)

	// Load and verify
	cfg := LoadConfig("")
	if cfg.Theme.Name != "nord" {
		t.Errorf("saved theme = %q, want nord", cfg.Theme.Name)
	}
	if cfg.Window.Position != "top" {
		t.Errorf("saved position = %q, want top", cfg.Window.Position)
	}

	// Save position back to bottom
	SavePosition(false)
	cfg = LoadConfig("")
	if cfg.Window.Position != "bottom" {
		t.Errorf("saved position = %q, want bottom", cfg.Window.Position)
	}
}

func TestParseINI(t *testing.T) {
	input := `# test config
[theme]
name = cobalt

[window]
position = top
margin = 30
opacity = 0.85

[keys]
unit_size = 10
padding = 6
font_size = 14
scale = 60
repeat_delay_ms = 300
repeat_rate_ms = 50

[gamepad]
device = /dev/input/event5
grab = false
deadzone = 0.3
long_press_ms = 400
swap_xy = true
mouse_stick = left

[gamepad.buttons]
press = b
close = a
backspace = y
space = x
shift = rt
enter = lt
left_click = lb
right_click = rb
position_toggle = select

[mouse]
enabled = false
sensitivity = 12
`
	cfg := DefaultConfig()
	if err := parseINI(strings.NewReader(input), &cfg); err != nil {
		t.Fatalf("parseINI error: %v", err)
	}

	if cfg.Theme.Name != "cobalt" {
		t.Errorf("theme.name = %q, want cobalt", cfg.Theme.Name)
	}
	if cfg.Window.Position != "top" {
		t.Errorf("window.position = %q, want top", cfg.Window.Position)
	}
	if cfg.Window.Margin != 30 {
		t.Errorf("window.margin = %d, want 30", cfg.Window.Margin)
	}
	if cfg.Window.Opacity != 0.85 {
		t.Errorf("window.opacity = %f, want 0.85", cfg.Window.Opacity)
	}
	if cfg.Keys.Scale != 60 {
		t.Errorf("keys.scale = %d, want 60", cfg.Keys.Scale)
	}
	if cfg.Keys.RepeatDelayMs != 300 {
		t.Errorf("keys.repeat_delay_ms = %d, want 300", cfg.Keys.RepeatDelayMs)
	}
	if cfg.Gamepad.Device != "/dev/input/event5" {
		t.Errorf("gamepad.device = %q, want /dev/input/event5", cfg.Gamepad.Device)
	}
	if cfg.Gamepad.Grab != false {
		t.Error("gamepad.grab should be false")
	}
	if cfg.Gamepad.Deadzone != 0.3 {
		t.Errorf("gamepad.deadzone = %f, want 0.3", cfg.Gamepad.Deadzone)
	}
	if cfg.Gamepad.SwapXY != "true" {
		t.Errorf("gamepad.swap_xy = %q, want true", cfg.Gamepad.SwapXY)
	}
	if cfg.Gamepad.MouseStick != "left" {
		t.Errorf("gamepad.mouse_stick = %q, want left", cfg.Gamepad.MouseStick)
	}
	if cfg.Gamepad.Buttons.Press != "b" {
		t.Errorf("gamepad.buttons.press = %q, want b", cfg.Gamepad.Buttons.Press)
	}
	if cfg.Gamepad.Buttons.PositionToggle != "select" {
		t.Errorf("gamepad.buttons.position_toggle = %q, want select", cfg.Gamepad.Buttons.PositionToggle)
	}
	if cfg.Mouse.Enabled != false {
		t.Error("mouse.enabled should be false")
	}
	if cfg.Mouse.Sensitivity != 12 {
		t.Errorf("mouse.sensitivity = %d, want 12", cfg.Mouse.Sensitivity)
	}
}

func TestParseINIQuotedValues(t *testing.T) {
	// TOML-migrated files may still have quoted strings
	input := `[theme]
name = "nord"

[gamepad]
device = ""
swap_xy = "auto"
`
	cfg := DefaultConfig()
	if err := parseINI(strings.NewReader(input), &cfg); err != nil {
		t.Fatalf("parseINI error: %v", err)
	}
	if cfg.Theme.Name != "nord" {
		t.Errorf("theme.name = %q, want nord (quotes stripped)", cfg.Theme.Name)
	}
	if cfg.Gamepad.Device != "" {
		t.Errorf("gamepad.device = %q, want empty string", cfg.Gamepad.Device)
	}
	if cfg.Gamepad.SwapXY != "auto" {
		t.Errorf("gamepad.swap_xy = %q, want auto", cfg.Gamepad.SwapXY)
	}
}

func TestWriteINIRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Theme.Name = "cobalt"
	cfg.Theme.KeyStyle = "rounded"
	cfg.Theme.KeyShadow = false // not the default, so the round trip is tested
	cfg.Window.Opacity = 0.98
	cfg.Gamepad.Buttons.Press = "b"

	var buf strings.Builder
	if err := writeINI(&buf, cfg); err != nil {
		t.Fatalf("writeINI error: %v", err)
	}

	// Parse back
	cfg2 := DefaultConfig()
	if err := parseINI(strings.NewReader(buf.String()), &cfg2); err != nil {
		t.Fatalf("parseINI error on round-trip: %v", err)
	}

	if cfg2.Theme.Name != "cobalt" {
		t.Errorf("round-trip theme = %q, want cobalt", cfg2.Theme.Name)
	}
	if cfg2.Theme.KeyStyle != "rounded" {
		t.Errorf("round-trip key_style = %q, want rounded", cfg2.Theme.KeyStyle)
	}
	if cfg2.Theme.KeyShadow {
		t.Error("round-trip key_shadow = true, want false")
	}
	if cfg2.Window.Opacity != 0.98 {
		t.Errorf("round-trip opacity = %f, want 0.98", cfg2.Window.Opacity)
	}
	if cfg2.Gamepad.Buttons.Press != "b" {
		t.Errorf("round-trip press = %q, want b", cfg2.Gamepad.Buttons.Press)
	}
	if cfg2.Gamepad.Grab != true {
		t.Error("round-trip grab should be true (default)")
	}
	if cfg2.Mouse.Enabled != true {
		t.Error("round-trip mouse.enabled should be true (default)")
	}
}

func TestMigrateFromTOML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	resetConfigState(t)

	// Write a TOML config (with quoted strings, like BurntSushi/toml produces)
	tomlDir := filepath.Join(tmpDir, "gamepad-osk")
	if err := os.MkdirAll(tomlDir, 0755); err != nil { //nolint:gosec // G301: test dir
		t.Fatal(err)
	}
	tomlPath := filepath.Join(tomlDir, "config.toml")
	tomlContent := `[theme]
  name = "cobalt"

[window]
  position = "top"
  margin = 15
  opacity = 0.90

[keys]
  scale = 60

[gamepad]
  device = ""
  grab = true
  deadzone = 0.3
  swap_xy = "auto"
  mouse_stick = "right"

  [gamepad.buttons]
    press = "a"
    close = "b"

[mouse]
  enabled = true
  sensitivity = 10
`
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0644); err != nil { //nolint:gosec // G306: test file
		t.Fatal(err)
	}

	// LoadConfig should auto-migrate (suppress migration log output)
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	cfg := LoadConfig("")

	// Verify config values loaded correctly
	if cfg.Theme.Name != "cobalt" {
		t.Errorf("migrated theme = %q, want cobalt", cfg.Theme.Name)
	}
	if cfg.Window.Position != "top" {
		t.Errorf("migrated position = %q, want top", cfg.Window.Position)
	}
	if cfg.Window.Margin != 15 {
		t.Errorf("migrated margin = %d, want 15", cfg.Window.Margin)
	}
	if cfg.Gamepad.Deadzone != 0.3 {
		t.Errorf("migrated deadzone = %f, want 0.3", cfg.Gamepad.Deadzone)
	}

	// Verify new config file exists
	newPath := filepath.Join(tomlDir, "config")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("migrated config not created: %v", err)
	}

	// Verify new config has no TOML-style quotes
	data, err := os.ReadFile(newPath) //nolint:gosec // G304: test path
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"cobalt"`) {
		t.Error("migrated config still contains TOML-style quotes")
	}

	// Verify old file renamed to .v1.bak
	bakPath := tomlPath + ".v1.bak"
	if _, err := os.Stat(bakPath); err != nil {
		t.Errorf("backup file not created: %v", err)
	}
	if _, err := os.Stat(tomlPath); !os.IsNotExist(err) {
		t.Error("original config.toml should no longer exist")
	}

	// Re-running should not re-migrate (new config already exists)
	cfg2 := LoadConfig("")
	if cfg2.Theme.Name != "cobalt" {
		t.Errorf("second load theme = %q, want cobalt", cfg2.Theme.Name)
	}
}

func TestStickClickAutoMap(t *testing.T) {
	// Default: mouse_stick=right → R3=click, L3=caps
	cfg := DefaultConfig()
	am := BuildActionMap(cfg.Gamepad)

	if am.BtnPress[BTN_THUMBR] != ActionLeftClick {
		t.Errorf("R3 = %v, want ActionLeftClick (mouse_stick=right)", am.BtnPress[BTN_THUMBR])
	}
	if am.BtnRelease[BTN_THUMBR] != ActionLeftClickRelease {
		t.Errorf("R3 release = %v, want ActionLeftClickRelease", am.BtnRelease[BTN_THUMBR])
	}
	if am.BtnPress[BTN_THUMBL] != ActionCapsToggle {
		t.Errorf("L3 = %v, want ActionCapsToggle (mouse_stick=right)", am.BtnPress[BTN_THUMBL])
	}

	// Swap: mouse_stick=left → L3=click, R3=caps
	cfg.Gamepad.MouseStick = "left"
	am = BuildActionMap(cfg.Gamepad)

	if am.BtnPress[BTN_THUMBL] != ActionLeftClick {
		t.Errorf("L3 = %v, want ActionLeftClick (mouse_stick=left)", am.BtnPress[BTN_THUMBL])
	}
	if am.BtnPress[BTN_THUMBR] != ActionCapsToggle {
		t.Errorf("R3 = %v, want ActionCapsToggle (mouse_stick=left)", am.BtnPress[BTN_THUMBR])
	}
}

func TestParseComboString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantN   int
		wantErr bool
	}{
		{"two buttons", "select+start", 2, false},
		{"three buttons", "select+start+rb", 3, false},
		{"four buttons", "select+start+lb+rb", 4, false},
		{"with spaces", " select + start ", 2, false},
		{"case insensitive", "Select+START", 2, false},
		{"empty string", "", 0, false},
		{"single button", "a", 0, true},
		{"five buttons", "a+b+x+y+lb", 0, true},
		{"unknown button", "a+banana", 0, true},
		{"duplicate", "a+a", 0, true},
		{"guide button", "guide+a", 2, false},
		{"dpad buttons", "dpad_up+dpad_down", 2, false},
		{"dpad shorthand", "up+down", 2, false},
		{"dpad mixed", "up+dpad_down", 2, false},
		{"shorthand+button", "select+up", 2, false},
		{"cross-alias duplicate", "up+dpad_up", 0, true},
		{"triggers", "lt+rt", 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buttons, err := parseComboString(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseComboString(%q) = no error, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("parseComboString(%q) error: %v", tt.input, err)
				return
			}
			if len(buttons) != tt.wantN {
				t.Errorf("parseComboString(%q) = %d buttons, want %d", tt.input, len(buttons), tt.wantN)
			}
		})
	}
}

func TestParseComboStringDpad(t *testing.T) {
	buttons, err := parseComboString("dpad_up+a")
	if err != nil {
		t.Fatalf("parseComboString error: %v", err)
	}
	if len(buttons) != 2 {
		t.Fatalf("got %d buttons, want 2", len(buttons))
	}

	// Find the dpad_up button
	var dpad ComboButton
	for _, b := range buttons {
		if b.Name == "dpad_up" {
			dpad = b
			break
		}
	}
	if dpad.Name == "" {
		t.Fatal("dpad_up not found in parsed buttons")
	}
	if len(dpad.BtnCodes) == 0 {
		t.Error("dpad_up should have BtnCodes (BTN_DPAD_UP)")
	}
	if dpad.AxisCode != ABS_HAT0Y {
		t.Errorf("dpad_up AxisCode = 0x%x, want ABS_HAT0Y (0x%x)", dpad.AxisCode, ABS_HAT0Y)
	}
	if dpad.AxisVal != -1 {
		t.Errorf("dpad_up AxisVal = %d, want -1", dpad.AxisVal)
	}
}

func TestParseComboStringTriggers(t *testing.T) {
	buttons, err := parseComboString("lt+rt")
	if err != nil {
		t.Fatalf("parseComboString error: %v", err)
	}

	var lt, rt ComboButton
	for _, b := range buttons {
		switch b.Name {
		case "lt":
			lt = b
		case "rt":
			rt = b
		}
	}
	// LT: both BTN_TL2 and ABS_Z
	if len(lt.BtnCodes) == 0 || lt.BtnCodes[0] != BTN_TL2 {
		t.Errorf("lt BtnCodes = %v, want [BTN_TL2]", lt.BtnCodes)
	}
	if lt.AxisCode != ABS_Z {
		t.Errorf("lt AxisCode = 0x%x, want ABS_Z", lt.AxisCode)
	}
	// RT: both BTN_TR2 and ABS_RZ
	if len(rt.BtnCodes) == 0 || rt.BtnCodes[0] != BTN_TR2 {
		t.Errorf("rt BtnCodes = %v, want [BTN_TR2]", rt.BtnCodes)
	}
	if rt.AxisCode != ABS_RZ {
		t.Errorf("rt AxisCode = 0x%x, want ABS_RZ", rt.AxisCode)
	}
}

func TestComboConfigRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gamepad.ToggleCombo = "select+start"
	cfg.Gamepad.ComboPeriodMs = 300

	var buf strings.Builder
	if err := writeINI(&buf, cfg); err != nil {
		t.Fatalf("writeINI error: %v", err)
	}

	cfg2 := DefaultConfig()
	if err := parseINI(strings.NewReader(buf.String()), &cfg2); err != nil {
		t.Fatalf("parseINI error: %v", err)
	}

	if cfg2.Gamepad.ToggleCombo != "select+start" {
		t.Errorf("round-trip toggle_combo = %q, want select+start", cfg2.Gamepad.ToggleCombo)
	}
	if cfg2.Gamepad.ComboPeriodMs != 300 {
		t.Errorf("round-trip combo_period_ms = %d, want 300", cfg2.Gamepad.ComboPeriodMs)
	}
}

func TestValidateComboConfig(t *testing.T) {
	// Valid combo passes validation
	cfg := DefaultConfig()
	cfg.Gamepad.ToggleCombo = "select+start"
	ValidateConfig(&cfg)
	if cfg.Gamepad.ToggleCombo != "select+start" {
		t.Errorf("valid combo cleared: %q", cfg.Gamepad.ToggleCombo)
	}

	// Invalid combo gets cleared
	cfg.Gamepad.ToggleCombo = "banana+phone"
	ValidateConfig(&cfg)
	if cfg.Gamepad.ToggleCombo != "" {
		t.Errorf("invalid combo not cleared: %q", cfg.Gamepad.ToggleCombo)
	}

	// combo_period_ms out of range
	cfg.Gamepad.ComboPeriodMs = 10
	ValidateConfig(&cfg)
	if cfg.Gamepad.ComboPeriodMs != 200 {
		t.Errorf("combo_period_ms = %d, want 200 after validation", cfg.Gamepad.ComboPeriodMs)
	}
}

func TestCheckConfig(t *testing.T) {
	// Valid default config has no issues
	cfg := DefaultConfig()
	if issues := checkConfig(cfg); len(issues) != 0 {
		t.Errorf("default config has issues: %v", issues)
	}

	// Each bad value produces an issue
	cases := []struct {
		name  string
		apply func(*Config)
		want  string
	}{
		{"bad theme", func(c *Config) { c.Theme.Name = "nonexistent" }, "unknown theme"},
		{"bad key_style", func(c *Config) { c.Theme.KeyStyle = "round" }, "key_style"},
		{"bad scale low", func(c *Config) { c.Keys.Scale = 10 }, "scale"},
		{"bad scale high", func(c *Config) { c.Keys.Scale = 200 }, "scale"},
		{"bad deadzone", func(c *Config) { c.Gamepad.Deadzone = 2.0 }, "deadzone"},
		{"bad sensitivity", func(c *Config) { c.Mouse.Sensitivity = 0 }, "sensitivity"},
		{"bad repeat_delay", func(c *Config) { c.Keys.RepeatDelayMs = 50 }, "repeat_delay_ms"},
		{"bad repeat_rate", func(c *Config) { c.Keys.RepeatRateMs = 5 }, "repeat_rate_ms"},
		{"bad long_press", func(c *Config) { c.Gamepad.LongPressMs = 10 }, "long_press_ms"},
		{"bad combo_period", func(c *Config) { c.Gamepad.ComboPeriodMs = 5 }, "combo_period_ms"},
		{"bad toggle_combo", func(c *Config) { c.Gamepad.ToggleCombo = "a+banana" }, "toggle_combo"},
		{"opacity below floor", func(c *Config) { c.Window.Opacity = 0.1 }, "opacity"},
		{"opacity too high", func(c *Config) { c.Window.Opacity = 1.5 }, "opacity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := DefaultConfig()
			tc.apply(&c)
			issues := checkConfig(c)
			if len(issues) == 0 {
				t.Fatalf("expected issue containing %q, got none", tc.want)
			}
			found := false
			for _, issue := range issues {
				if strings.Contains(issue, tc.want) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected issue containing %q, got %v", tc.want, issues)
			}
		})
	}
}

func TestCheckConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config")

	// Valid sections - no issues
	if err := os.WriteFile(path, []byte("[theme]\nname = dark\n[keys]\nscale = 50\n"), 0644); err != nil { //nolint:gosec // G306: test file
		t.Fatal(err)
	}
	if issues := checkConfigFile(path); len(issues) != 0 {
		t.Errorf("valid config has section issues: %v", issues)
	}

	// Unknown section caught
	if err := os.WriteFile(path, []byte("[theme]\nname = dark\n[kkeys]\nscale = 50\n"), 0644); err != nil { //nolint:gosec // G306: test file
		t.Fatal(err)
	}
	issues := checkConfigFile(path)
	if len(issues) == 0 {
		t.Fatal("expected issue for [kkeys], got none")
	}
	if !strings.Contains(issues[0], "kkeys") {
		t.Errorf("expected issue mentioning 'kkeys', got %q", issues[0])
	}
}

func TestTriggerDualMap(t *testing.T) {
	cfg := DefaultConfig()
	am := BuildActionMap(cfg.Gamepad)

	// RT should be mapped as both axis and button
	if am.AxisActions[ABS_RZ] != ActionEnter {
		t.Errorf("ABS_RZ axis = %v, want ActionEnter", am.AxisActions[ABS_RZ])
	}
	if am.BtnPress[BTN_TR2] != ActionEnter {
		t.Errorf("BTN_TR2 button = %v, want ActionEnter (Switch Pro compat)", am.BtnPress[BTN_TR2])
	}

	// Release events for both
	if am.AxisRelease[ABS_RZ] != ActionEnterRelease {
		t.Errorf("ABS_RZ release = %v, want ActionEnterRelease", am.AxisRelease[ABS_RZ])
	}
	if am.BtnRelease[BTN_TR2] != ActionEnterRelease {
		t.Errorf("BTN_TR2 release = %v, want ActionEnterRelease", am.BtnRelease[BTN_TR2])
	}

	// LT should be mapped as axis (shift) and button
	if am.AxisActions[ABS_Z] != ActionShiftOn {
		t.Errorf("ABS_Z axis = %v, want ActionShiftOn", am.AxisActions[ABS_Z])
	}
	if am.BtnPress[BTN_TL2] != ActionShiftOn {
		t.Errorf("BTN_TL2 button = %v, want ActionShiftOn (Switch Pro compat)", am.BtnPress[BTN_TL2])
	}
}

func TestBorderlessThemes(t *testing.T) {
	// Only the phone-style themes are designed without key borders; other
	// themes rely on the border where keys are close to the background.
	for name, theme := range Themes {
		want := name == "light" || name == "night"
		if theme.Borderless != want {
			t.Errorf("theme %q: Borderless = %v, want %v", name, theme.Borderless, want)
		}
	}
}

// isolateConfig points the user config and the read-only search paths at
// temp dirs, so a config in /etc or next to the test binary is never read.
// It returns the user config path and the dir holding the search paths.
func isolateConfig(t *testing.T) (userPath, searchDir string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	resetConfigState(t)
	searchDir = t.TempDir()
	orig := configSearchPaths
	configSearchPaths = func() []string {
		return []string{filepath.Join(searchDir, "system"), filepath.Join(searchDir, "cwd")}
	}
	t.Cleanup(func() { configSearchPaths = orig })
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return UserConfigPath(), searchDir
}

// resetConfigState clears what LoadConfig and saves remember, before and
// after the test, so a --config test cannot redirect a later save.
func resetConfigState(t *testing.T) {
	t.Helper()
	reset := func() {
		configSource.Path, configSource.FromFlag = "", false
		saveWarned = false
	}
	reset()
	t.Cleanup(reset)
}

func writeTestConfig(t *testing.T, path, theme string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil { //nolint:gosec // G301: test dir
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[theme]\nname = "+theme+"\n"), 0644); err != nil { //nolint:gosec // G306: test file
		t.Fatal(err)
	}
}

func TestLoadConfigSearchOrder(t *testing.T) {
	t.Run("nothing found", func(t *testing.T) {
		isolateConfig(t)
		cfg := LoadConfig("")
		if cfg.Theme.Name != DefaultConfig().Theme.Name {
			t.Errorf("theme = %q, want default", cfg.Theme.Name)
		}
		if configSource.Path != "" || configSource.FromFlag {
			t.Errorf("configSource = %+v, want empty", configSource)
		}
	})
	t.Run("search path used when no user config", func(t *testing.T) {
		_, dir := isolateConfig(t)
		cwd := filepath.Join(dir, "cwd")
		writeTestConfig(t, cwd, "nord")
		if cfg := LoadConfig(""); cfg.Theme.Name != "nord" {
			t.Errorf("theme = %q, want nord", cfg.Theme.Name)
		}
		if configSource.Path != cwd || configSource.FromFlag {
			t.Errorf("configSource = %+v, want %s", configSource, cwd)
		}
	})
	t.Run("first search path wins", func(t *testing.T) {
		_, dir := isolateConfig(t)
		writeTestConfig(t, filepath.Join(dir, "system"), "nord")
		writeTestConfig(t, filepath.Join(dir, "cwd"), "cobalt")
		if cfg := LoadConfig(""); cfg.Theme.Name != "nord" {
			t.Errorf("theme = %q, want nord", cfg.Theme.Name)
		}
	})
	t.Run("user config before search paths", func(t *testing.T) {
		user, dir := isolateConfig(t)
		writeTestConfig(t, user, "dracula")
		writeTestConfig(t, filepath.Join(dir, "system"), "nord")
		if cfg := LoadConfig(""); cfg.Theme.Name != "dracula" {
			t.Errorf("theme = %q, want dracula", cfg.Theme.Name)
		}
		if configSource.Path != user {
			t.Errorf("configSource = %+v, want %s", configSource, user)
		}
	})
	t.Run("--config before user config", func(t *testing.T) {
		user, dir := isolateConfig(t)
		writeTestConfig(t, user, "dracula")
		flag := filepath.Join(dir, "flag")
		writeTestConfig(t, flag, "gotham")
		if cfg := LoadConfig(flag); cfg.Theme.Name != "gotham" {
			t.Errorf("theme = %q, want gotham", cfg.Theme.Name)
		}
		if configSource.Path != flag || !configSource.FromFlag {
			t.Errorf("configSource = %+v, want %s from flag", configSource, flag)
		}
	})
}

// Every value the parser ignores is reported with its line, in the log
// at start and in --setup. Nothing is ignored silently.
func TestConfigNotes(t *testing.T) {
	tests := []struct {
		name, in string
		want     []string
		check    func(Config) bool // the setting kept its default
	}{
		{"unknown key", "[theme]\nnam = nord\n",
			[]string{"line 2: unknown key theme.nam ignored (check for typos)"}, nil},
		{"removed key", "[keys]\nfont_size = 14\n",
			[]string{"line 2: keys.font_size is no longer used and is ignored (the line can be deleted)"}, nil},
		{"unknown section once", "[kkeys]\nscale = 60\npadding = 3\n",
			[]string{"line 2: unknown section [kkeys], keys under it are ignored (check section headers)"},
			func(c Config) bool { return c.Keys.Scale == 50 && c.Keys.Padding == 4 }},
		{"key before any section", "name = nord\n",
			[]string{"line 1: unknown section [], keys under it are ignored (check section headers)"}, nil},
		{"not key = value", "[theme]\nname: nord\n",
			[]string{`line 2: "name: nord" is not a key = value line, ignored`}, nil},
		{"empty key", "[theme]\n= nord\n",
			[]string{`line 2: "= nord" is not a key = value line, ignored`}, nil},
		{"bad whole number", "[keys]\nscale = 60ms\n",
			[]string{`line 2: keys.scale = "60ms" is not a whole number, using 50`},
			func(c Config) bool { return c.Keys.Scale == 50 }},
		{"bad float", "[window]\nopacity = 0,8\n",
			[]string{`line 2: window.opacity = "0,8" is not a number (use a dot: 0.8), using 0.95`},
			func(c Config) bool { return c.Window.Opacity == 0.95 }},
		{"NaN float", "[window]\nopacity = nan\n",
			[]string{`line 2: window.opacity = "nan" is not a number (use a dot: 0.8), using 0.95`},
			func(c Config) bool { return c.Window.Opacity == 0.95 }},
		{"infinite float", "[gamepad]\ndeadzone = -Inf\n",
			[]string{`line 2: gamepad.deadzone = "-Inf" is not a number (use a dot: 0.8), using 0.25`},
			func(c Config) bool { return c.Gamepad.Deadzone == 0.25 }},
		{"bad bool", "[gamepad]\ngrab = yes\n",
			[]string{`line 2: gamepad.grab = "yes" is not true or false, using true`},
			func(c Config) bool { return c.Gamepad.Grab }},
		{"bad value keeps the earlier line", "[keys]\nscale = 60\nscale = x\n",
			[]string{"line 3: keys.scale is set twice (also set on line 2, this line wins)",
				`line 3: keys.scale = "x" is not a whole number, using 60`},
			func(c Config) bool { return c.Keys.Scale == 60 }},
		{"set twice", "[theme]\nname = cobalt\n\nname = nord\n",
			[]string{"line 4: theme.name is set twice (also set on line 2, this line wins)"},
			func(c Config) bool { return c.Theme.Name == "nord" }},
		{"single quotes stripped", "[theme]\nname = 'nord'\n", nil,
			func(c Config) bool { return c.Theme.Name == "nord" }},
		{"clean file", "[theme]\nname = nord # mine\n[ keys ] # x\nScale = 60\n", nil,
			func(c Config) bool { return c.Keys.Scale == 60 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			notes, err := parseINIKeys(strings.NewReader(tt.in), &cfg)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, n := range notes {
				got = append(got, n.String())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("notes =\n%q\nwant\n%q", got, tt.want)
			}
			if tt.check != nil && !tt.check(cfg) {
				t.Errorf("setting changed: %+v", cfg)
			}

			// The same notes reach the log and --setup.
			var buf strings.Builder
			log.SetOutput(&buf)
			defer log.SetOutput(os.Stderr)
			cfg = DefaultConfig()
			if err := parseINI(strings.NewReader(tt.in), &cfg); err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.want {
				if !strings.Contains(buf.String(), "Warning: config "+w) {
					t.Errorf("log is missing %q:\n%s", w, buf.String())
				}
			}
			if tt.want == nil && buf.Len() != 0 {
				t.Errorf("clean file logged:\n%s", buf.String())
			}
			path := filepath.Join(t.TempDir(), "config")
			if err := os.WriteFile(path, []byte(tt.in), 0600); err != nil {
				t.Fatal(err)
			}
			if issues := checkConfigFile(path); !slices.Equal(issues, tt.want) {
				t.Errorf("checkConfigFile = %q, want %q", issues, tt.want)
			}
		})
	}
}

// The shipped example and a freshly written default file both load to
// exactly DefaultConfig() and raise no issues.
func TestExampleMatchesDefaults(t *testing.T) {
	for _, src := range []string{"example", "written"} {
		var data []byte
		if src == "example" {
			var err error
			if data, err = os.ReadFile("config.example"); err != nil {
				t.Fatal(err)
			}
		} else {
			var buf strings.Builder
			if err := writeINI(&buf, DefaultConfig()); err != nil {
				t.Fatal(err)
			}
			data = []byte(buf.String())
		}
		var cfg Config
		unknown, err := parseINIKeys(strings.NewReader(string(data)), &cfg)
		if err != nil {
			t.Fatal(err)
		}
		if len(unknown) != 0 {
			t.Errorf("%s: unknown keys %q", src, unknown)
		}
		if !reflect.DeepEqual(cfg, DefaultConfig()) {
			t.Errorf("%s does not match DefaultConfig():\n got  %+v\n want %+v", src, cfg, DefaultConfig())
		}
		if issues := checkConfig(cfg); len(issues) != 0 {
			t.Errorf("%s: checkConfig issues %q", src, issues)
		}
		path := filepath.Join(t.TempDir(), "config")
		if err := os.WriteFile(path, data, 0644); err != nil { //nolint:gosec // G306: test file
			t.Fatal(err)
		}
		if issues := checkConfigFile(path); len(issues) != 0 {
			t.Errorf("%s: checkConfigFile issues %q", src, issues)
		}
	}
}

// Issue #3: --config is the only file read and written.
func TestConfigFlag(t *testing.T) {
	t.Run("does not create the user config", func(t *testing.T) {
		user, dir := isolateConfig(t)
		flag := filepath.Join(dir, "flag")
		writeTestConfig(t, flag, "gotham")
		LoadConfig(flag)
		if _, err := os.Stat(user); !os.IsNotExist(err) {
			t.Errorf("user config created at %s (err %v)", user, err)
		}
		if _, err := os.Stat(filepath.Dir(user)); !os.IsNotExist(err) {
			t.Errorf("user config dir created (err %v)", err)
		}
	})
	t.Run("does not fall back when the file goes away", func(t *testing.T) {
		user, dir := isolateConfig(t)
		writeTestConfig(t, user, "dracula")
		writeTestConfig(t, filepath.Join(dir, "system"), "nord")
		cfg := LoadConfig(filepath.Join(dir, "missing"))
		if cfg.Theme.Name != DefaultConfig().Theme.Name {
			t.Errorf("theme = %q, want the default (no other file read)", cfg.Theme.Name)
		}
	})
	t.Run("saves go to the flag file", func(t *testing.T) {
		user, dir := isolateConfig(t)
		flag := filepath.Join(dir, "flag")
		writeTestConfig(t, flag, "gotham")
		LoadConfig(flag)
		SaveTheme("nord")
		SavePosition(true)
		SaveSensitivity(22)
		if _, err := os.Stat(user); !os.IsNotExist(err) {
			t.Errorf("save wrote the user config (err %v)", err)
		}
		cfg := DefaultConfig()
		data, err := os.ReadFile(flag) //nolint:gosec // G304: test path
		if err != nil {
			t.Fatal(err)
		}
		if err := parseINI(strings.NewReader(string(data)), &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.Theme.Name != "nord" || cfg.Window.Position != "top" || cfg.Mouse.Sensitivity != 22 {
			t.Errorf("flag file has theme %q, position %q, sensitivity %d; want nord, top, 22",
				cfg.Theme.Name, cfg.Window.Position, cfg.Mouse.Sensitivity)
		}
	})
	t.Run("read-only flag file warns once and is not replaced", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("root ignores file permissions")
		}
		user, dir := isolateConfig(t)
		flag := filepath.Join(dir, "flag")
		writeTestConfig(t, flag, "gotham")
		if err := os.Chmod(flag, 0400); err != nil {
			t.Fatal(err)
		}
		var buf strings.Builder
		log.SetOutput(&buf)
		LoadConfig(flag)
		SaveTheme("nord")
		SaveTheme("cobalt")
		SavePosition(true)
		if n := strings.Count(buf.String(), "cannot save"); n != 1 {
			t.Errorf("%d save warnings, want 1:\n%s", n, buf.String())
		}
		data, _ := os.ReadFile(flag) //nolint:gosec // G304: test path
		if !strings.Contains(string(data), "gotham") {
			t.Error("read-only flag file was changed")
		}
		if _, err := os.Stat(user); !os.IsNotExist(err) {
			t.Error("save fell back to the user config")
		}
	})
}

func TestCheckConfigFlag(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "c")
	writeTestConfig(t, file, "nord")

	if got, err := checkConfigFlag(file); err != nil || got != file {
		t.Errorf("existing file: %q, %v", got, err)
	}
	t.Chdir(dir)
	if got, err := checkConfigFlag("c"); err != nil || got != file {
		t.Errorf("relative path: got %q, %v; want %q", got, err, file)
	}
	for name, path := range map[string]string{
		"missing":   filepath.Join(dir, "nope"),
		"directory": dir,
	} {
		if _, err := checkConfigFlag(path); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if os.Getuid() != 0 {
		unreadable := filepath.Join(dir, "unreadable")
		writeTestConfig(t, unreadable, "nord")
		if err := os.Chmod(unreadable, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := checkConfigFlag(unreadable); err == nil {
			t.Error("unreadable: no error")
		}
	}
}

func TestSetINIValue(t *testing.T) {
	tests := []struct {
		name, in, section, key, value, want string
	}{
		{"replace keeps comment column",
			"[theme]\nname = matrix                # see --help\n", "theme", "name", "nord",
			"[theme]\nname = nord                  # see --help\n"},
		{"long value pushes comment",
			"[theme]\nname = a # c\n", "theme", "name", "tokyo_storm",
			"[theme]\nname = tokyo_storm # c\n"},
		{"comments, blank lines and unknown keys stay",
			"# mine\n\n[theme]\n# pick one\nname = matrix\nfoo = bar\n\n[keys]\nfont_size = 14\n", "theme", "name", "nord",
			"# mine\n\n[theme]\n# pick one\nname = nord\nfoo = bar\n\n[keys]\nfont_size = 14\n"},
		{"other section untouched",
			"[window]\nposition = top\n[theme]\nname = matrix\n", "window", "position", "bottom",
			"[window]\nposition = bottom\n[theme]\nname = matrix\n"},
		{"key missing goes after the section's last key",
			"[window]\nmargin = 5\n\n[keys]\nscale = 60\n", "window", "position", "top",
			"[window]\nmargin = 5\nposition = top\n\n[keys]\nscale = 60\n"},
		{"empty section",
			"[window]\n[keys]\nscale = 60\n", "window", "position", "top",
			"[window]\nposition = top\n[keys]\nscale = 60\n"},
		{"section missing goes at the end",
			"[keys]\nscale = 60\n", "mouse", "sensitivity", "12",
			"[keys]\nscale = 60\n\n[mouse]\nsensitivity = 12\n"},
		{"empty file", "", "theme", "name", "nord", "[theme]\nname = nord\n"},
		{"no final newline",
			"[theme]\nname = matrix", "theme", "name", "nord", "[theme]\nname = nord\n"},
		{"CRLF kept",
			"[theme]\r\nname = matrix\r\n[keys]\r\nscale = 60\r\n", "theme", "name", "nord",
			"[theme]\r\nname = nord\r\n[keys]\r\nscale = 60\r\n"},
		{"CRLF insert",
			"[keys]\r\nscale = 60\r\n", "mouse", "sensitivity", "12",
			"[keys]\r\nscale = 60\r\n\r\n[mouse]\r\nsensitivity = 12\r\n"},
		{"quotes kept", "[theme]\nname = \"matrix\" # q\n", "theme", "name", "nord",
			"[theme]\nname = \"nord\"   # q\n"},
		{"indent and tight equals",
			"[theme]\n  name=matrix\n", "theme", "name", "nord", "[theme]\n  name= nord\n"},
		{"last duplicate key wins, first left alone",
			"[theme]\nname = a\nname = b\n", "theme", "name", "nord", "[theme]\nname = a\nname = nord\n"},
		{"duplicate section: last key line",
			"[theme]\nname = a\n[keys]\nscale = 60\n[theme]\nkey_style = square\n", "theme", "name", "nord",
			"[theme]\nname = nord\n[keys]\nscale = 60\n[theme]\nkey_style = square\n"},
		{"header with comment and spaces",
			"[ Theme ] # look\nname = matrix\n", "theme", "name", "nord", "[ Theme ] # look\nname = nord\n"},
		{"byte order mark",
			"\ufeff[theme]\nname = matrix\n", "theme", "name", "nord", "\ufeff[theme]\nname = nord\n"},
		{"upper-case key",
			"[theme]\nName = matrix\n", "theme", "name", "nord", "[theme]\nName = nord\n"},
		{"commented-out key is not the key",
			"[theme]\n# name = old\nname = matrix\n", "theme", "name", "nord",
			"[theme]\n# name = old\nname = nord\n"},
		{"bad value elsewhere stays (R9)",
			"[keys]\nscale = 60ms\n[theme]\nname = matrix\n", "theme", "name", "nord",
			"[keys]\nscale = 60ms\n[theme]\nname = nord\n"},
		{"dotted section",
			"[gamepad]\ngrab = true\n[gamepad.buttons]\npress = a\n", "gamepad.buttons", "press", "b",
			"[gamepad]\ngrab = true\n[gamepad.buttons]\npress = b\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(setINIValue([]byte(tt.in), tt.section, tt.key, tt.value))
			if got != tt.want {
				t.Errorf("got\n%q\nwant\n%q", got, tt.want)
			}
			if err := checkINIEdit([]byte(tt.in), []byte(got), tt.section, tt.key, tt.value); err != nil {
				t.Errorf("guard rejects the edit: %v", err)
			}
		})
	}
}

func TestCheckINIEditRejects(t *testing.T) {
	old := []byte("[theme]\nname = matrix\n[keys]\nscale = 60\nfoo = 1\n")
	bad := map[string]string{
		"other value changed": "[theme]\nname = nord\n[keys]\nscale = 50\nfoo = 1\n",
		"unknown key lost":    "[theme]\nname = nord\n[keys]\nscale = 60\n",
		"new value missing":   "[theme]\nname = matrix\n[keys]\nscale = 60\nfoo = 1\n",
		"section header lost": "name = nord\n[keys]\nscale = 60\nfoo = 1\n",
		"emptied":             "",
	}
	for name, edited := range bad {
		if err := checkINIEdit(old, []byte(edited), "theme", "name", "nord"); err == nil {
			t.Errorf("%s: guard accepted it", name)
		}
	}
}

// Saves keep the user's file; the guard and unreadable files never lead
// to a rewrite from defaults.
func TestSaveKeepsUserFile(t *testing.T) {
	t.Run("whole file kept except the line", func(t *testing.T) {
		user, _ := isolateConfig(t)
		in := "# my settings\n[window]\nbottom_margin = 20   # old name\n\n[keys]\nfont_size = 14\nscale = 60ms\n[theme]\nname = matrix # theme\n"
		writeRaw(t, user, in, 0600)
		SaveTheme("nord")
		SavePosition(true)
		SaveSensitivity(14)
		data, _ := os.ReadFile(user) //nolint:gosec // G304: test path
		want := "# my settings\n[window]\nbottom_margin = 20   # old name\nposition = top\n\n[keys]\nfont_size = 14\nscale = 60ms\n[theme]\nname = nord   # theme\n\n[mouse]\nsensitivity = 14\n"
		if string(data) != want {
			t.Errorf("got\n%s\nwant\n%s", data, want)
		}
		if info, _ := os.Stat(user); info.Mode().Perm() != 0600 {
			t.Errorf("mode = %v, want 0600 kept", info.Mode().Perm())
		}
		// H2: the old bottom_margin still gives a margin of 20 after saves.
		if cfg := LoadConfig(""); cfg.Window.Margin != 20 {
			t.Errorf("margin after saves = %d, want 20 from bottom_margin", cfg.Window.Margin)
		}
	})
	t.Run("new file starts from the defaults", func(t *testing.T) {
		user, _ := isolateConfig(t)
		SaveTheme("nord")
		cfg := DefaultConfig()
		data, _ := os.ReadFile(user) //nolint:gosec // G304: test path
		if err := parseINI(strings.NewReader(string(data)), &cfg); err != nil {
			t.Fatal(err)
		}
		want := DefaultConfig()
		want.Theme.Name = "nord"
		if !reflect.DeepEqual(cfg, want) {
			t.Errorf("new file loads to %+v, want defaults with nord", cfg)
		}
		if !strings.Contains(string(data), "# gamepad-osk configuration") {
			t.Error("new file has no comments")
		}
	})
	t.Run("unreadable file is not replaced", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("root ignores file permissions")
		}
		user, _ := isolateConfig(t)
		writeRaw(t, user, "[theme]\nname = gotham\n", 0200)
		SaveTheme("nord")
		if err := os.Chmod(user, 0600); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(user) //nolint:gosec // G304: test path
		if string(data) != "[theme]\nname = gotham\n" {
			t.Errorf("unreadable file was rewritten:\n%s", data)
		}
	})
}

// FuzzSetINIValue: whatever the file holds, the edit must pass the guard.
// A failure here is a bug in setINIValue; the guard still keeps the file.
func FuzzSetINIValue(f *testing.F) {
	seeds := []string{
		"", "[theme]\nname = matrix\n", "[theme] # c\r\nname=\"x\"\r\n", "\ufeff[ theme ]\nName = a\nname = b\n",
		"x = 1\n[theme]\n[theme]\nname = a # b # c\n", "[window]\nposition = top\n[unknown]\nname = z\n",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		log.SetOutput(io.Discard)
		defer log.SetOutput(os.Stderr)
		for _, kv := range [][3]string{{"theme", "name", "nord"}, {"window", "position", "top"}, {"mouse", "sensitivity", "12"}} {
			out := setINIValue([]byte(in), kv[0], kv[1], kv[2])
			if err := checkINIEdit([]byte(in), out, kv[0], kv[1], kv[2]); err != nil {
				t.Errorf("%s.%s on %q:\n%q\n%v", kv[0], kv[1], in, out, err)
			}
		}
	})
}

func writeRaw(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil { //nolint:gosec // G301: test dir
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

// --help and --setup read the config but never create or migrate one.
func TestReadConfigNeverWrites(t *testing.T) {
	user, _ := isolateConfig(t)
	legacy := filepath.Join(filepath.Dir(user), "config.toml")
	writeRaw(t, legacy, "[theme]\nname = \"nord\"\n", 0600)
	ReadConfig("")
	if _, err := os.Stat(user); !os.IsNotExist(err) {
		t.Errorf("ReadConfig created %s (err %v)", user, err)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Errorf("ReadConfig migrated the TOML file: %v", err)
	}
}

func TestConfigInUse(t *testing.T) {
	user, dir := isolateConfig(t)
	ReadConfig("")
	if got := configInUse(); !strings.Contains(got, "none found") || !strings.Contains(got, user) {
		t.Errorf("no file: %q", got)
	}
	system := filepath.Join(dir, "system")
	writeTestConfig(t, system, "nord")
	ReadConfig("")
	if got := configInUse(); got != "In use: "+system {
		t.Errorf("system file: %q", got)
	}
	flag := filepath.Join(dir, "flag")
	writeTestConfig(t, flag, "nord")
	ReadConfig(flag)
	if got := configInUse(); got != "In use: "+flag+" (--config)" {
		t.Errorf("--config: %q", got)
	}
}

// Issue #3: --setup checks the --config file, else the file in use.
func TestSetupDiagnoseConfig(t *testing.T) {
	run := func(configPath string) string {
		t.Helper()
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		old := os.Stdout
		os.Stdout = w
		runDiagnose(configPath)
		os.Stdout = old
		_ = w.Close()
		out, _ := io.ReadAll(r)
		return string(out)
	}
	user, dir := isolateConfig(t)

	flag := filepath.Join(dir, "flag")
	writeRaw(t, flag, "[keys]\nscale = 60ms\n", 0600)
	writeTestConfig(t, user, "nord")
	out := run(flag)
	if !strings.Contains(out, flag) || !strings.Contains(out, `keys.scale = "60ms"`) {
		t.Errorf("--setup -c did not check the flag file:\n%s", out)
	}

	trashUser := filepath.Join(dir, "gone")
	if err := os.Rename(user, trashUser); err != nil {
		t.Fatal(err)
	}
	system := filepath.Join(dir, "system")
	writeRaw(t, system, "[theme]\nnam = x\n", 0600)
	out = run("")
	if !strings.Contains(out, system) || !strings.Contains(out, "unknown key theme.nam") {
		t.Errorf("--setup did not check the file in use:\n%s", out)
	}
	if _, err := os.Stat(user); !os.IsNotExist(err) {
		t.Error("--setup created the user config")
	}
}

// R14: a file named config in the working directory is never read.
func TestWorkingDirNotSearched(t *testing.T) {
	for _, p := range configSearchPaths() {
		if !filepath.IsAbs(p) {
			t.Errorf("relative search path %q", p)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	resetConfigState(t)
	dir := t.TempDir()
	writeTestConfig(t, filepath.Join(dir, "config"), "nord")
	t.Chdir(dir)
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	if cfg := ReadConfig(""); cfg.Theme.Name == "nord" {
		t.Error("read config from the working directory")
	}
}

// fakeSudo makes the app believe it runs as root through sudo for a user
// whose home is home. It returns the paths handed to lchown.
func fakeSudo(t *testing.T, home string) *[]string {
	t.Helper()
	var chowned []string
	oldEuid, oldChown, oldHome := geteuid, lchown, homeOfID
	geteuid = func() int { return 0 }
	lchown = func(path string, uid, gid int) error {
		if uid != 1234 || gid != 5678 {
			t.Errorf("lchown(%s, %d, %d), want 1234:5678", path, uid, gid)
		}
		chowned = append(chowned, path)
		return nil
	}
	homeOfID = func(int) (string, error) { return home, nil }
	t.Cleanup(func() { geteuid, lchown, homeOfID = oldEuid, oldChown, oldHome })
	t.Setenv("SUDO_UID", "1234")
	t.Setenv("SUDO_GID", "5678")
	return &chowned
}

// R11: under sudo, what the app creates in the user's home goes to the user.
func TestSudoCreatesOwnedByUser(t *testing.T) {
	t.Run("first run: new folders and file", func(t *testing.T) {
		home := t.TempDir()
		xdg := filepath.Join(home, ".config")
		t.Setenv("XDG_CONFIG_HOME", xdg)
		resetConfigState(t)
		orig := configSearchPaths
		configSearchPaths = func() []string { return nil }
		t.Cleanup(func() { configSearchPaths = orig })
		log.SetOutput(io.Discard)
		defer log.SetOutput(os.Stderr)
		chowned := fakeSudo(t, home)
		LoadConfig("")
		want := []string{filepath.Join(xdg, "gamepad-osk"), xdg, filepath.Join(xdg, "gamepad-osk", "config")}
		if !slices.Equal(*chowned, want) {
			t.Errorf("chowned %q, want %q", *chowned, want)
		}
	})
	t.Run("existing folders are not touched", func(t *testing.T) {
		home := t.TempDir()
		xdg := filepath.Join(home, ".config")
		writeRaw(t, filepath.Join(xdg, "other", "x"), "", 0600)
		t.Setenv("XDG_CONFIG_HOME", xdg)
		resetConfigState(t)
		chowned := fakeSudo(t, home)
		SaveTheme("nord")
		want := []string{filepath.Join(xdg, "gamepad-osk"), filepath.Join(xdg, "gamepad-osk", "config")}
		if !slices.Equal(*chowned, want) {
			t.Errorf("chowned %q, want %q", *chowned, want)
		}
		// Existing file keeps its owner: the save writes a temp file and
		// gives it to the old file's owner (here the test user, since the
		// fake lchown above changed nothing) before the rename
		var kept []string
		lchown = func(path string, uid, gid int) error {
			if uid != os.Getuid() || gid != os.Getgid() || filepath.Dir(path) != filepath.Join(xdg, "gamepad-osk") {
				t.Errorf("save to an existing file: lchown(%s, %d, %d), want the old owner in the same folder", path, uid, gid)
			}
			kept = append(kept, path)
			return nil
		}
		SaveTheme("cobalt")
		if len(kept) != 1 {
			t.Errorf("save to an existing file: lchown calls %q, want one", kept)
		}
	})
	t.Run("outside the sudo user's home", func(t *testing.T) {
		user, _ := isolateConfig(t)
		chowned := fakeSudo(t, "/home/someone")
		SaveTheme("nord")
		if len(*chowned) != 0 {
			t.Errorf("chowned %q outside the user's home (%s)", *chowned, user)
		}
	})
	t.Run("not root", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
		resetConfigState(t)
		chowned := fakeSudo(t, home)
		geteuid = os.Geteuid
		SaveTheme("nord")
		if os.Geteuid() != 0 && len(*chowned) != 0 {
			t.Errorf("chowned %q without root", *chowned)
		}
	})
	t.Run("root without sudo", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
		resetConfigState(t)
		chowned := fakeSudo(t, home)
		t.Setenv("SUDO_UID", "")
		SaveTheme("nord")
		if len(*chowned) != 0 {
			t.Errorf("chowned %q with no SUDO_UID", *chowned)
		}
	})
}

// L: one rule list. For each key, a bad value gives exactly one --setup
// issue, one start warning that names the value used, and that value
// afterwards. A good value gives none and stays. H1: the fallback named
// in the warning is the one set.
func TestConfigRules(t *testing.T) {
	def := DefaultConfig()
	tests := []struct {
		key  string
		bad  func(*Config)
		good func(*Config)
		got  func(Config) string // the setting after ValidateConfig
		want string              // fallback after a bad value
	}{
		{"theme", func(c *Config) { c.Theme.Name = "nope" }, func(c *Config) { c.Theme.Name = "nord" },
			func(c Config) string { return c.Theme.Name }, def.Theme.Name},
		{"key_style", func(c *Config) { c.Theme.KeyStyle = "round" }, func(c *Config) { c.Theme.KeyStyle = "rounded" },
			func(c Config) string { return c.Theme.KeyStyle }, def.Theme.KeyStyle},
		{"position", func(c *Config) { c.Window.Position = "middle" }, func(c *Config) { c.Window.Position = "top" },
			func(c Config) string { return c.Window.Position }, def.Window.Position},
		{"margin", func(c *Config) { c.Window.Margin = -5 }, func(c *Config) { c.Window.Margin = 30 },
			func(c Config) string { return strconv.Itoa(c.Window.Margin) }, "0"},
		{"opacity", func(c *Config) { c.Window.Opacity = 0.05 }, func(c *Config) { c.Window.Opacity = 0.8 },
			func(c Config) string { return fmt.Sprint(c.Window.Opacity) }, "0.2"},
		{"unit_size", func(c *Config) { c.Keys.UnitSize = -1 }, func(c *Config) { c.Keys.UnitSize = 50 },
			func(c Config) string { return strconv.Itoa(c.Keys.UnitSize) }, "0"},
		{"padding", func(c *Config) { c.Keys.Padding = -2 }, func(c *Config) { c.Keys.Padding = 6 },
			func(c Config) string { return strconv.Itoa(c.Keys.Padding) }, "4"},
		{"scale", func(c *Config) { c.Keys.Scale = 200 }, func(c *Config) { c.Keys.Scale = 70 },
			func(c Config) string { return strconv.Itoa(c.Keys.Scale) }, "50"},
		{"repeat_delay_ms", func(c *Config) { c.Keys.RepeatDelayMs = 50 }, func(c *Config) { c.Keys.RepeatDelayMs = 300 },
			func(c Config) string { return strconv.Itoa(c.Keys.RepeatDelayMs) }, "400"},
		{"repeat_rate_ms", func(c *Config) { c.Keys.RepeatRateMs = 5 }, func(c *Config) { c.Keys.RepeatRateMs = 60 },
			func(c Config) string { return strconv.Itoa(c.Keys.RepeatRateMs) }, "80"},
		{"deadzone", func(c *Config) { c.Gamepad.Deadzone = 2 }, func(c *Config) { c.Gamepad.Deadzone = 0.4 },
			func(c Config) string { return fmt.Sprint(c.Gamepad.Deadzone) }, "0.25"},
		{"long_press_ms", func(c *Config) { c.Gamepad.LongPressMs = 10 }, func(c *Config) { c.Gamepad.LongPressMs = 800 },
			func(c Config) string { return strconv.Itoa(c.Gamepad.LongPressMs) }, "500"},
		{"swap_xy", func(c *Config) { c.Gamepad.SwapXY = "True" }, func(c *Config) { c.Gamepad.SwapXY = "false" },
			func(c Config) string { return c.Gamepad.SwapXY }, "auto"},
		{"controller", func(c *Config) { c.Gamepad.Controller = "sega" }, func(c *Config) { c.Gamepad.Controller = "xbox" },
			func(c Config) string { return c.Gamepad.Controller }, "auto"},
		{"mouse_stick", func(c *Config) { c.Gamepad.MouseStick = "both" }, func(c *Config) { c.Gamepad.MouseStick = "left" },
			func(c Config) string { return c.Gamepad.MouseStick }, "right"},
		{"toggle_combo", func(c *Config) { c.Gamepad.ToggleCombo = "a+banana" }, func(c *Config) { c.Gamepad.ToggleCombo = "select+start" },
			func(c Config) string { return c.Gamepad.ToggleCombo }, ""},
		{"combo_period_ms", func(c *Config) { c.Gamepad.ComboPeriodMs = 5 }, func(c *Config) { c.Gamepad.ComboPeriodMs = 300 },
			func(c Config) string { return strconv.Itoa(c.Gamepad.ComboPeriodMs) }, "200"},
		{"press", func(c *Config) { c.Gamepad.Buttons.Press = "z" }, func(c *Config) { c.Gamepad.Buttons.Press = "select" },
			func(c Config) string { return c.Gamepad.Buttons.Press }, "a"},
		{"close", func(c *Config) { c.Gamepad.Buttons.Close = "" }, func(c *Config) { c.Gamepad.Buttons.Close = "select" },
			func(c Config) string { return c.Gamepad.Buttons.Close }, "b"},
		{"backspace", func(c *Config) { c.Gamepad.Buttons.Backspace = "X" }, nil,
			func(c Config) string { return c.Gamepad.Buttons.Backspace }, "x"},
		{"space", func(c *Config) { c.Gamepad.Buttons.Space = "guide" }, nil,
			func(c Config) string { return c.Gamepad.Buttons.Space }, "y"},
		{"shift", func(c *Config) { c.Gamepad.Buttons.Shift = "l2" }, nil,
			func(c Config) string { return c.Gamepad.Buttons.Shift }, "lt"},
		{"enter", func(c *Config) { c.Gamepad.Buttons.Enter = "r2" }, nil,
			func(c Config) string { return c.Gamepad.Buttons.Enter }, "rt"},
		{"left_click", func(c *Config) { c.Gamepad.Buttons.LeftClick = "r1" }, nil,
			func(c Config) string { return c.Gamepad.Buttons.LeftClick }, "rb"},
		{"right_click", func(c *Config) { c.Gamepad.Buttons.RightClick = "l1" }, nil,
			func(c Config) string { return c.Gamepad.Buttons.RightClick }, "lb"},
		{"position_toggle", func(c *Config) { c.Gamepad.Buttons.PositionToggle = "home" },
			func(c *Config) { c.Gamepad.Buttons.PositionToggle = "" }, // empty disables it
			func(c Config) string { return c.Gamepad.Buttons.PositionToggle }, "start"},
		{"sensitivity", func(c *Config) { c.Mouse.Sensitivity = 0 }, func(c *Config) { c.Mouse.Sensitivity = 30 },
			func(c Config) string { return strconv.Itoa(c.Mouse.Sensitivity) }, "10"},
	}
	run := func(c *Config) (issues []string, logged string) {
		issues = checkConfig(*c)
		var buf strings.Builder
		log.SetOutput(&buf)
		ValidateConfig(c)
		log.SetOutput(os.Stderr)
		return issues, buf.String()
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			c := DefaultConfig()
			tt.bad(&c)
			issues, logged := run(&c)
			if len(issues) != 1 || !strings.Contains(issues[0], tt.key) {
				t.Errorf("bad value: issues %q, want one naming %s", issues, tt.key)
			}
			if strings.Count(logged, "Warning:") != 1 {
				t.Errorf("bad value: %d warnings, want 1:\n%s", strings.Count(logged, "Warning:"), logged)
			}
			if got := tt.got(c); got != tt.want {
				t.Errorf("after ValidateConfig %s = %q, want %q", tt.key, got, tt.want)
			}
			if tt.want != "" && !strings.Contains(logged, "using "+tt.want) {
				t.Errorf("warning does not name the value set (%s):\n%s", tt.want, logged)
			}
			if again := checkConfig(c); len(again) != 0 {
				t.Errorf("still broken after the fix: %q", again)
			}

			if tt.good == nil {
				return
			}
			c = DefaultConfig()
			tt.good(&c)
			want := tt.got(c)
			if issues, logged := run(&c); len(issues) != 0 || logged != "" || tt.got(c) != want {
				t.Errorf("good value: issues %q, log %q, value %q (want %q)", issues, logged, tt.got(c), want)
			}
		})
	}
}

func TestButtonClashes(t *testing.T) {
	tests := []struct {
		name string
		set  func(*Config)
		want []string
	}{
		{"defaults", func(*Config) {}, nil},
		{"two actions on one button", func(c *Config) { c.Gamepad.Buttons.Close = "a" },
			[]string{"press and close are both on a"}},
		{"action on the caps stick click", func(c *Config) { c.Gamepad.Buttons.Space = "l3" },
			[]string{"Caps Lock (stick click) and space are both on l3"}},
		{"mouse_stick left swaps the stick clicks", func(c *Config) {
			c.Gamepad.MouseStick = "left"
			c.Gamepad.Buttons.Space = "l3"
		}, []string{"left click (stick click) and space are both on l3"}},
		{"position_toggle disabled is no clash", func(c *Config) { c.Gamepad.Buttons.PositionToggle = "" }, nil},
		{"swapped pair is fine", func(c *Config) {
			c.Gamepad.Buttons.Backspace, c.Gamepad.Buttons.Space = "y", "x"
		}, nil},
	}
	for _, tt := range tests {
		c := DefaultConfig()
		tt.set(&c)
		if got := buttonClashes(c); !slices.Equal(got, tt.want) {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
	// A clash puts back every button, since which action the user meant is
	// unknown. The warning names the clash and the buttons now in use.
	def := DefaultConfig().Gamepad.Buttons
	clash := []struct {
		name string
		set  func(*Config)
		want string
	}{
		{"two actions on one button", func(c *Config) {
			c.Gamepad.Buttons.Close, c.Gamepad.Buttons.Space = "a", "lb"
		}, "press and close are both on a"},
		{"click on the enter trigger", func(c *Config) {
			c.Gamepad.Buttons.LeftClick, c.Gamepad.Buttons.Backspace = "rt", "y"
		}, "enter and left_click are both on rt"},
		{"action on a stick click", func(c *Config) { c.Gamepad.Buttons.Close = "r3" },
			"left click (stick click) and close are both on r3"},
	}
	for _, tt := range clash {
		c := DefaultConfig()
		tt.set(&c)
		var buf strings.Builder
		log.SetOutput(&buf)
		ValidateConfig(&c)
		log.SetOutput(os.Stderr)
		if c.Gamepad.Buttons != def {
			t.Errorf("%s: buttons = %+v, want the defaults", tt.name, c.Gamepad.Buttons)
		}
		got := buf.String()
		if !strings.Contains(got, tt.want) || !strings.Contains(got, "using the default buttons") ||
			!strings.Contains(got, "left_click = rb") {
			t.Errorf("%s: log %q", tt.name, got)
		}
		if strings.Count(got, "Warning:") != 1 {
			t.Errorf("%s: want one warning, log %q", tt.name, got)
		}
	}
	// The defaults never clash with either stick, so the revert always ends
	// in a working layout.
	for _, stick := range []string{"right", "left"} {
		c := DefaultConfig()
		c.Gamepad.MouseStick = stick
		if got := buttonClashes(c); got != nil {
			t.Errorf("defaults with mouse_stick=%s clash: %q", stick, got)
		}
	}
	// Clicks on the triggers with shift and enter moved off them is valid.
	c := triggerClickConfig()
	if issues := checkConfig(c); issues != nil {
		t.Errorf("clicks on triggers: %q", issues)
	}
}

// Keys the coverage table found untested by name.
func TestPanelAvoidAndBottomMargin(t *testing.T) {
	cfg := DefaultConfig()
	if err := parseINI(strings.NewReader("[window]\npanel_avoid = false\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Window.PanelAvoid {
		t.Error("panel_avoid = false not read")
	}
	var buf strings.Builder
	if err := writeINI(&buf, cfg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "panel_avoid = false") {
		t.Error("panel_avoid = false not written")
	}

	user, _ := isolateConfig(t)
	writeRaw(t, user, "[window]\nbottom_margin = 12\n", 0600)
	if got := LoadConfig("").Window.Margin; got != 12 {
		t.Errorf("bottom_margin = 12 gives margin %d, want 12", got)
	}
	writeRaw(t, user, "[window]\nmargin = 5\nbottom_margin = 12\n", 0600)
	if got := LoadConfig("").Window.Margin; got != 5 {
		t.Errorf("margin and bottom_margin: margin %d, want 5 (margin wins)", got)
	}
}

// writeConfigFile replaces the file through a temp file and a rename.
func TestWriteConfigFile(t *testing.T) {
	read := func(p string) string {
		b, err := os.ReadFile(p) //nolint:gosec // G304: test path
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	noTemp := func(dir string) {
		t.Helper()
		if m, _ := filepath.Glob(filepath.Join(dir, ".*.tmp")); len(m) > 0 {
			t.Errorf("temp files left: %v", m)
		}
	}

	t.Run("keeps mode", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "config")
		writeRaw(t, p, "old", 0640)
		before, _ := os.Stat(p)
		if err := writeConfigFile(p, []byte("new")); err != nil {
			t.Fatal(err)
		}
		info, _ := os.Stat(p)
		if read(p) != "new" || info.Mode().Perm() != 0640 {
			t.Errorf("content %q mode %v, want new and 0640", read(p), info.Mode().Perm())
		}
		// A new inode: the file was replaced by a rename, never cut to
		// zero and rewritten in place (a crash then left an empty config)
		if os.SameFile(before, info) {
			t.Error("the file was rewritten in place, not replaced")
		}
		noTemp(dir)
	})

	t.Run("symlink stays a link", func(t *testing.T) {
		dir := t.TempDir()
		realDir := filepath.Join(dir, "dotfiles")
		target := filepath.Join(realDir, "config")
		link := filepath.Join(dir, "config")
		writeRaw(t, target, "old", 0600)
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if err := writeConfigFile(link, []byte("new")); err != nil {
			t.Fatal(err)
		}
		if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
			t.Error("the link was replaced by a file")
		}
		if read(target) != "new" {
			t.Errorf("target %q, want new", read(target))
		}
		noTemp(realDir)
	})

	t.Run("read-only file is not replaced", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can write a read-only file")
		}
		dir := t.TempDir()
		p := filepath.Join(dir, "config")
		writeRaw(t, p, "old", 0444)
		if err := writeConfigFile(p, []byte("new")); err == nil {
			t.Error("no error for a read-only file")
		}
		if read(p) != "old" {
			t.Errorf("read-only file changed to %q", read(p))
		}
		noTemp(dir)
	})

	t.Run("read-only folder writes in place", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can write a read-only folder")
		}
		dir := t.TempDir()
		p := filepath.Join(dir, "config")
		writeRaw(t, p, "old", 0644)
		if err := os.Chmod(dir, 0555); err != nil { //nolint:gosec // G302: read-only test dir
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0755) }) //nolint:gosec // G302: restore test dir
		if err := writeConfigFile(p, []byte("new")); err != nil {
			t.Fatal(err)
		}
		if read(p) != "new" {
			t.Errorf("content %q, want new", read(p))
		}
	})

	t.Run("new file", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "config")
		if err := writeConfigFile(p, []byte("new")); err != nil {
			t.Fatal(err)
		}
		if read(p) != "new" {
			t.Errorf("content %q, want new", read(p))
		}
	})

	t.Run("under sudo the owner is kept", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "config")
		writeRaw(t, p, "old", 0644)
		oldEuid, oldChown := geteuid, lchown
		t.Cleanup(func() { geteuid, lchown = oldEuid, oldChown })
		var calls []string
		geteuid = func() int { return 0 }
		lchown = func(_ string, uid, gid int) error {
			calls = append(calls, fmt.Sprintf("%d:%d", uid, gid))
			return nil
		}
		if err := writeConfigFile(p, []byte("new")); err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
		if len(calls) != 1 || calls[0] != want {
			t.Errorf("lchown calls %v, want [%s] (the old file's owner)", calls, want)
		}
	})
}
