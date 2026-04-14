package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
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
			func(c *Config) bool { return c.Theme.Name == "matrix" }, "should reset to matrix"},
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
	if len(Themes) != 60 {
		t.Errorf("theme count = %d, want 60", len(Themes))
	}
	// Spot-check new mid-range themes
	for _, name := range []string{"chalk", "fjord", "sand", "plum", "moss"} {
		if _, ok := Themes[name]; !ok {
			t.Errorf("missing theme %q", name)
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
		{"bad scale low", func(c *Config) { c.Keys.Scale = 10 }, "scale"},
		{"bad scale high", func(c *Config) { c.Keys.Scale = 200 }, "scale"},
		{"bad deadzone", func(c *Config) { c.Gamepad.Deadzone = 2.0 }, "deadzone"},
		{"bad sensitivity", func(c *Config) { c.Mouse.Sensitivity = 0 }, "sensitivity"},
		{"bad repeat_delay", func(c *Config) { c.Keys.RepeatDelayMs = 50 }, "repeat_delay_ms"},
		{"bad repeat_rate", func(c *Config) { c.Keys.RepeatRateMs = 5 }, "repeat_rate_ms"},
		{"bad long_press", func(c *Config) { c.Gamepad.LongPressMs = 10 }, "long_press_ms"},
		{"bad combo_period", func(c *Config) { c.Gamepad.ComboPeriodMs = 5 }, "combo_period_ms"},
		{"bad toggle_combo", func(c *Config) { c.Gamepad.ToggleCombo = "a+banana" }, "toggle_combo"},
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
