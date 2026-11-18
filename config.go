package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Debugf logs only when --verbose is set
func Debugf(format string, args ...any) {
	if Verbose {
		log.Printf("[DEBUG] "+format, args...) //nolint:gosec // G706: format string is from our code, not user input
	}
}

type Config struct {
	Theme   ThemeConfig
	Window  WindowConfig
	Keys    KeysConfig
	Gamepad GamepadConfig
	Mouse   MouseConfig
}

type ThemeConfig struct {
	Name      string
	KeyStyle  string // "square" or "rounded"
	KeyShadow bool
}

type WindowConfig struct {
	Position     string // "bottom" or "top"
	Margin       int
	BottomMargin int // deprecated, migrated to Margin
	Opacity      float64
	PanelAvoid   bool // Wayland: respect panel exclusive zones (false = always screen edge)
}

type KeysConfig struct {
	UnitSize      int
	Padding       int
	Scale         int // percentage of screen width (30-100, default 50)
	RepeatDelayMs int // ms before key repeat starts (default 400)
	RepeatRateMs  int // ms between repeats (default 80)
}

type ButtonsConfig struct {
	Press          string
	Close          string
	Backspace      string
	Space          string
	Shift          string
	Enter          string
	LeftClick      string
	RightClick     string
	PositionToggle string
}

type GamepadConfig struct {
	Device        string
	Grab          bool
	Deadzone      float64
	LongPressMs   int
	SwapXY        string // "auto", "true", "false"
	Controller    string // auto, xbox, playstation, nintendo, 8bitdo, generic (button names)
	MouseStick    string // "left" or "right" (nav uses the other stick)
	ToggleCombo   string // e.g. "select+start", empty = disabled
	ComboPeriodMs int    // timing window for combo detection (default 200)
	Buttons       ButtonsConfig
}

type MouseConfig struct {
	Enabled     bool
	Sensitivity int
}

func DefaultConfig() Config {
	return Config{
		Theme:  ThemeConfig{Name: "matrix", KeyStyle: "square", KeyShadow: true},
		Window: WindowConfig{Position: "bottom", Margin: 0, Opacity: 0.95, PanelAvoid: true},
		Keys:   KeysConfig{UnitSize: 0, Padding: 4, Scale: 50, RepeatDelayMs: 400, RepeatRateMs: 80},
		Gamepad: GamepadConfig{
			Grab:          true,
			Deadzone:      0.25,
			LongPressMs:   500,
			SwapXY:        "auto",
			Controller:    ProfileAuto,
			MouseStick:    "right",
			ComboPeriodMs: 200,
			Buttons: ButtonsConfig{
				Press:          "a",
				Close:          "b",
				Backspace:      "x",
				Space:          "y",
				Shift:          "lt",
				Enter:          "rt",
				LeftClick:      "rb",
				RightClick:     "lb",
				PositionToggle: "start",
			},
		},
		Mouse: MouseConfig{Enabled: true, Sensitivity: 10},
	}
}

// --- INI config parser/writer ---

// parseBool parses a bool value, logging a warning and returning the default on bad input.
func parseBool(key, val string, defaultVal bool) bool {
	b, err := strconv.ParseBool(val)
	if err != nil {
		log.Printf("Warning: %s has invalid bool value, using %v", key, defaultVal)
		return defaultVal
	}
	return b
}

// parseINI reads an INI-style config file into a Config.
// Supports [section] and [section.subsection] headers, key = value pairs,
// # comments, and inline # comments.
func parseINI(r io.Reader, cfg *Config) error {
	unknown, err := parseINIKeys(r, cfg)
	for _, k := range unknown {
		log.Printf("Warning: unknown config key %s ignored (check for typos)", k) //nolint:gosec // G706: key from user's own config file
	}
	return err
}

// iniLine trims a raw line and a UTF-8 byte order mark, which some
// editors write at the start of a file.
func iniLine(raw string) string {
	return strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
}

// iniSection reports whether a trimmed line is a section header and
// returns its lower-case name. It allows an inline comment after the
// header and spaces inside the brackets: "[ Theme ]  # look".
func iniSection(line string) (string, bool) {
	if idx := strings.Index(line, "#"); idx > 0 {
		line = strings.TrimSpace(line[:idx])
	}
	if len(line) < 2 || line[0] != '[' || line[len(line)-1] != ']' {
		return "", false
	}
	return strings.ToLower(strings.TrimSpace(line[1 : len(line)-1])), true
}

// removedKeys were valid in older versions. They are ignored without a
// warning and dropped on the next save.
var removedKeys = map[string]bool{"keys.font_size": true}

// parseINIKeys parses r into cfg and returns the unknown keys as
// section.key, without logging them.
func parseINIKeys(r io.Reader, cfg *Config) ([]string, error) {
	scanner := bufio.NewScanner(r)
	section := ""
	var unknown []string
	skip := func(key string) {
		if k := section + "." + key; !removedKeys[k] {
			unknown = append(unknown, k)
		}
	}

	for scanner.Scan() {
		line := iniLine(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if name, ok := iniSection(line); ok {
			section = name
			continue
		}

		// Strip inline comments (but not inside values - no values contain #)
		if idx := strings.Index(line, "#"); idx > 0 {
			line = strings.TrimSpace(line[:idx])
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		// Strip quotes from TOML-migrated values
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}

		switch section {
		case "theme":
			switch key {
			case "name":
				cfg.Theme.Name = val
			case "key_style":
				cfg.Theme.KeyStyle = val
			case "key_shadow":
				cfg.Theme.KeyShadow = parseBool("key_shadow", val, cfg.Theme.KeyShadow)
			default:
				skip(key)
			}
		case "window":
			switch key {
			case "position":
				cfg.Window.Position = val
			case "margin":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Window.Margin = n
				}
			case "bottom_margin":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Window.BottomMargin = n
				}
			case "opacity":
				if f, err := strconv.ParseFloat(val, 64); err == nil {
					cfg.Window.Opacity = f
				}
			case "panel_avoid":
				cfg.Window.PanelAvoid = parseBool("panel_avoid", val, true)
			default:
				skip(key)
			}
		case "keys":
			switch key {
			case "unit_size":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Keys.UnitSize = n
				}
			case "padding":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Keys.Padding = n
				}
			case "scale":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Keys.Scale = n
				}
			case "repeat_delay_ms":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Keys.RepeatDelayMs = n
				}
			case "repeat_rate_ms":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Keys.RepeatRateMs = n
				}
			default:
				skip(key)
			}
		case "gamepad":
			switch key {
			case "device":
				cfg.Gamepad.Device = val
			case "grab":
				cfg.Gamepad.Grab = parseBool("grab", val, true)
			case "deadzone":
				if f, err := strconv.ParseFloat(val, 64); err == nil {
					cfg.Gamepad.Deadzone = f
				}
			case "long_press_ms":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Gamepad.LongPressMs = n
				}
			case "swap_xy":
				cfg.Gamepad.SwapXY = val
			case "controller":
				cfg.Gamepad.Controller = strings.ToLower(val)
			case "mouse_stick":
				cfg.Gamepad.MouseStick = val
			case "toggle_combo":
				cfg.Gamepad.ToggleCombo = val
			case "combo_period_ms":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Gamepad.ComboPeriodMs = n
				}
			default:
				skip(key)
			}
		case "gamepad.buttons":
			switch key {
			case "press":
				cfg.Gamepad.Buttons.Press = val
			case "close":
				cfg.Gamepad.Buttons.Close = val
			case "backspace":
				cfg.Gamepad.Buttons.Backspace = val
			case "space":
				cfg.Gamepad.Buttons.Space = val
			case "shift":
				cfg.Gamepad.Buttons.Shift = val
			case "enter":
				cfg.Gamepad.Buttons.Enter = val
			case "left_click":
				cfg.Gamepad.Buttons.LeftClick = val
			case "right_click":
				cfg.Gamepad.Buttons.RightClick = val
			case "position_toggle":
				cfg.Gamepad.Buttons.PositionToggle = val
			default:
				skip(key)
			}
		case "mouse":
			switch key {
			case "enabled":
				cfg.Mouse.Enabled = parseBool("enabled", val, true)
			case "sensitivity":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Mouse.Sensitivity = n
				}
			default:
				skip(key)
			}
		default:
			log.Printf("Warning: unknown config section [%s], key %q ignored (check section headers)", section, key) //nolint:gosec // G706: section/key from user's own config file
		}
	}

	return unknown, scanner.Err()
}

// writeINI writes a Config in INI format with inline comments.
// Used for initial config creation - comments help users understand each setting.
func writeINI(w io.Writer, cfg Config) error {
	var b strings.Builder
	kv := func(key, val string) string { return key + " = " + val }
	kvf := func(key string, val any) string { return fmt.Sprintf("%s = %v", key, val) }
	line := func(s, comment string) string { return fmt.Sprintf("%-29s# %s\n", s, comment) }

	b.WriteString("# gamepad-osk configuration\n\n")

	b.WriteString("[theme]\n")
	b.WriteString(line(kv("name", cfg.Theme.Name), "see --help for full list (62 themes)"))
	b.WriteString(line(kv("key_style", cfg.Theme.KeyStyle), "square or rounded"))
	b.WriteString(line(kv("key_shadow", strconv.FormatBool(cfg.Theme.KeyShadow)), "drop shadow under keys"))
	b.WriteString("\n")

	b.WriteString("[window]\n")
	b.WriteString(line(kv("position", cfg.Window.Position), "bottom or top"))
	b.WriteString(line(kvf("margin", cfg.Window.Margin), "pixels from screen edge"))
	b.WriteString(line(kv("opacity", strconv.FormatFloat(cfg.Window.Opacity, 'f', -1, 64)), "0.2-1.0 (1.0 = fully opaque)"))
	b.WriteString(line(kv("panel_avoid", strconv.FormatBool(cfg.Window.PanelAvoid)), "Wayland: respect panel spacing (false = always screen edge)"))
	b.WriteString("\n")

	b.WriteString("[keys]\n")
	b.WriteString(line(kvf("unit_size", cfg.Keys.UnitSize), "0 = auto-scale, or fixed pixel size"))
	b.WriteString(line(kvf("padding", cfg.Keys.Padding), "pixels between keys"))
	b.WriteString(line(kvf("scale", cfg.Keys.Scale), "percentage of screen width (30-100)"))
	b.WriteString(line(kvf("repeat_delay_ms", cfg.Keys.RepeatDelayMs), "ms before key repeat starts (100-2000)"))
	b.WriteString(line(kvf("repeat_rate_ms", cfg.Keys.RepeatRateMs), "ms between repeats (20-500)"))
	b.WriteString("\n")

	b.WriteString("[gamepad]\n")
	b.WriteString(line(kv("device", cfg.Gamepad.Device), "empty = auto-detect, or /dev/input/eventX"))
	b.WriteString(line(kvf("grab", cfg.Gamepad.Grab), "grab device when visible (prevents input bleed to game)"))
	b.WriteString(line(kv("deadzone", strconv.FormatFloat(cfg.Gamepad.Deadzone, 'f', -1, 64)), "stick deadzone (0.0-1.0)"))
	b.WriteString(line(kvf("long_press_ms", cfg.Gamepad.LongPressMs), "ms to hold for accent popup (100-5000)"))
	b.WriteString(line(kv("swap_xy", cfg.Gamepad.SwapXY), "auto = detect xpad/xpadneo/xone drivers (default), true = force swap, false = off"))
	b.WriteString(line(kv("controller", cfg.Gamepad.Controller), "auto, xbox, playstation, nintendo, 8bitdo or generic - button names shown"))
	b.WriteString(line(kv("mouse_stick", cfg.Gamepad.MouseStick), "left or right - nav uses the other stick"))
	b.WriteString(line(kv("toggle_combo", cfg.Gamepad.ToggleCombo), "built-in show/hide combo (empty = disabled, use --toggle/evsieve)"))
	b.WriteString("                             # format: button+button (2-4 buttons, + separated)\n")
	b.WriteString("                             # buttons: a, b, x, y, lb, rb, lt, rt, l3, r3, start, select, guide,\n")
	b.WriteString("                             #          up, down, left, right (or dpad_up, dpad_down, etc.)\n")
	b.WriteString("                             # examples: guide+a, l3+r3, select+start, select+start+lb+rb\n")
	b.WriteString(line(kvf("combo_period_ms", cfg.Gamepad.ComboPeriodMs), "ms window for all combo buttons to be held (50-2000)"))
	b.WriteString("\n")

	b.WriteString("[gamepad.buttons]\n")
	b.WriteString("# button names: a, b, x, y, lb, rb, lt, rt, l3, r3, start, select\n")
	b.WriteString(line(kv("press", cfg.Gamepad.Buttons.Press), "press highlighted key"))
	b.WriteString(line(kv("close", cfg.Gamepad.Buttons.Close), "close keyboard (daemon mode: hides instead)"))
	b.WriteString(line(kv("backspace", cfg.Gamepad.Buttons.Backspace), "backspace"))
	b.WriteString(line(kv("space", cfg.Gamepad.Buttons.Space), "space"))
	b.WriteString(line(kv("shift", cfg.Gamepad.Buttons.Shift), "shift (hold)"))
	b.WriteString("# caps lock and mouse click are auto-assigned to stick clicks based on mouse_stick\n")
	b.WriteString(line(kv("enter", cfg.Gamepad.Buttons.Enter), "enter"))
	b.WriteString(line(kv("left_click", cfg.Gamepad.Buttons.LeftClick), "left mouse button (hold to drag)"))
	b.WriteString(line(kv("right_click", cfg.Gamepad.Buttons.RightClick), "right mouse button"))
	b.WriteString(line(kv("position_toggle", cfg.Gamepad.Buttons.PositionToggle), "toggle keyboard top/bottom (empty = disabled)"))
	b.WriteString("\n")

	b.WriteString("[mouse]\n")
	b.WriteString(line(kvf("enabled", cfg.Mouse.Enabled), "enable mouse cursor via right stick"))
	b.WriteString(line(kvf("sensitivity", cfg.Mouse.Sensitivity), "1-50, higher = faster cursor (Shift+up/down to adjust live)"))

	_, err := io.WriteString(w, b.String())
	return err
}

// migrateFromTOML converts a TOML config file to INI format.
// TOML key=value is a superset of INI - just strip quotes from string values.
// Returns the new config path, or error.
func migrateFromTOML(tomlPath, newPath string) error {
	in, err := os.Open(tomlPath) //nolint:gosec // G304: config path from known locations
	if err != nil {
		return fmt.Errorf("opening TOML config: %w", err)
	}
	defer func() { _ = in.Close() }()

	// Read TOML, strip quotes, write INI
	var b strings.Builder
	b.WriteString("# gamepad-osk configuration (migrated from config.toml)\n")
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Pass through comments and blanks; normalize section header indentation
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			b.WriteString(line + "\n")
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			b.WriteString(trimmed + "\n")
			continue
		}

		// key = value - strip quotes from value
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			// Strip TOML string quotes
			if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
				val = val[1 : len(val)-1]
			}
			b.WriteString(key + " = " + val + "\n")
		} else {
			b.WriteString(line + "\n")
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading TOML config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil { //nolint:gosec // G301: XDG config dir
		return fmt.Errorf("creating config dir: %w", err)
	}
	if err := os.WriteFile(newPath, []byte(b.String()), 0644); err != nil { //nolint:gosec // G306: user config
		return fmt.Errorf("writing INI config: %w", err)
	}

	bakPath := tomlPath + ".v1.bak"
	if err := os.Rename(tomlPath, bakPath); err != nil {
		log.Printf("Warning: could not rename %s to %s: %v", tomlPath, bakPath, err)
	}

	log.Printf("Migrated %s → %s (old file renamed to %s)", tomlPath, newPath, bakPath)
	return nil
}

var sudoHomeResolved string // cached sudo-aware home dir (log once)

// configDir returns the gamepad-osk config directory (XDG standard).
// When run via sudo, resolves the real user's home via SUDO_USER.
func configDir() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		if sudoHomeResolved == "" {
			sudoHomeResolved = os.Getenv("HOME")
			if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && sudoHomeResolved == "/root" {
				if u, err := user.Lookup(sudoUser); err == nil {
					sudoHomeResolved = u.HomeDir
					Debugf("sudo detected, using %s's home: %s", sudoUser, sudoHomeResolved)
				}
			}
		}
		xdg = filepath.Join(sudoHomeResolved, ".config")
	}
	return filepath.Join(xdg, "gamepad-osk")
}

// UserConfigPath returns the user's config file path.
func UserConfigPath() string {
	return filepath.Join(configDir(), "config")
}

// legacyConfigPath returns the old TOML config path for migration.
func legacyConfigPath() string {
	return filepath.Join(configDir(), "config.toml")
}

// configSearchPaths returns the read-only config locations searched after
// the user config: system config, next to the binary, working directory.
// A variable so tests can replace it.
var configSearchPaths = func() []string {
	paths := []string{"/etc/gamepad-osk/config"}
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), "config"))
	}
	return append(paths, "config")
}

// configSource records which file LoadConfig read.
var configSource struct {
	Path     string // empty: no file found, defaults used
	FromFlag bool   // Path came from --config
}

func LoadConfig(overridePath string) Config {
	cfg := DefaultConfig()

	userPath := UserConfigPath()
	legacyPath := legacyConfigPath()

	// Auto-migrate: TOML → INI if legacy exists and new does not
	if overridePath == "" {
		if _, err := os.Stat(userPath); os.IsNotExist(err) {
			if _, err := os.Stat(legacyPath); err == nil {
				if migErr := migrateFromTOML(legacyPath, userPath); migErr != nil {
					log.Printf("Warning: config migration failed: %v", migErr)
				}
			}
		}
	}

	// Priority: user config > system config > next to binary > cwd.
	// --config replaces the search: it is the only file read.
	paths := []string{overridePath}
	if overridePath == "" {
		paths = append([]string{userPath}, configSearchPaths()...)
	}

	// Saves go to the --config file even if reading it fails below.
	configSource.Path, configSource.FromFlag = overridePath, overridePath != ""
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil { //nolint:gosec // G703: config paths are trusted
			f, err := os.Open(p) //nolint:gosec // G304: config paths are trusted
			if err != nil {
				log.Printf("Warning: cannot open %s: %v", p, err) //nolint:gosec // G706: log format from our code
				continue
			}
			if parseErr := parseINI(f, &cfg); parseErr != nil {
				log.Printf("Warning: error parsing %s: %v", p, parseErr) //nolint:gosec // G706: log format from our code
			} else {
				Debugf("Loaded config from %s", p)
			}
			_ = f.Close()
			configSource.Path = p
			break
		}
	}

	// Backward compat: migrate bottom_margin → margin
	if cfg.Window.BottomMargin != 0 {
		if cfg.Window.Margin == 0 {
			cfg.Window.Margin = cfg.Window.BottomMargin
		} else {
			log.Printf("Warning: both 'margin' and deprecated 'bottom_margin' are set; using 'margin = %d'. Remove 'bottom_margin' from your config.", cfg.Window.Margin)
		}
	}

	// Auto-create user config if it doesn't exist (not with --config).
	// Prefer copying from a system/binary config file; fall back to writing defaults.
	if _, err := os.Stat(userPath); overridePath == "" && os.IsNotExist(err) {
		copied := false
		for _, src := range paths[1:] { // skip user path itself
			if _, err := os.Stat(src); err == nil { //nolint:gosec // G703: config paths are trusted
				if copyFile(src, userPath) == nil {
					log.Printf("Created user config at %s (copied from %s)", userPath, src) //nolint:gosec // G706: paths from our own config search, not user input
					copied = true
				}
				break
			}
		}
		if !copied {
			if err := os.MkdirAll(filepath.Dir(userPath), 0755); err == nil { //nolint:gosec // G301: config dir
				if f, err := os.Create(userPath); err == nil { //nolint:gosec // G304: user config path
					if writeErr := writeINI(f, cfg); writeErr == nil {
						log.Printf("Created user config at %s", userPath)
					}
					_ = f.Close()
				}
			}
		}
	}

	return cfg
}

// saveValue sets one key in the config file that saves go to (the
// --config file, else the user config). Only that line changes; comments,
// blank lines and unknown keys stay as the user wrote them. A missing file
// is created with the defaults first.
func saveValue(section, key, value string) {
	path := UserConfigPath()
	if configSource.FromFlag {
		path = configSource.Path
	} else if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil { //nolint:gosec // G301: config dir, not secrets
		saveFailed(path, err)
		return
	}

	data, err := os.ReadFile(path) //nolint:gosec // G304: user config path
	if os.IsNotExist(err) {
		var b strings.Builder
		if err := writeINI(&b, DefaultConfig()); err != nil {
			saveFailed(path, err)
			return
		}
		data, err = []byte(b.String()), nil
	}
	if err != nil {
		saveFailed(path, err)
		return
	}
	edited := setINIValue(data, section, key, value)
	if err := checkINIEdit(data, edited, section, key, value); err != nil {
		saveFailed(path, err)
		return
	}
	if err := os.WriteFile(path, edited, 0644); err != nil { //nolint:gosec // G306: config file, not secrets
		saveFailed(path, err)
	}
}

// checkINIEdit guards every save: the edited file must load to exactly
// the old settings plus the one new value, with the same unknown keys.
// Anything else means the edit misread the file, and nothing is written.
func checkINIEdit(old, edited []byte, section, key, value string) error {
	want := DefaultConfig()
	wantUnknown, err := parseINIKeys(strings.NewReader(string(old)), &want)
	if err != nil {
		return err
	}
	if _, err := parseINIKeys(strings.NewReader("["+section+"]\n"+key+" = "+value+"\n"), &want); err != nil {
		return err
	}
	got := DefaultConfig()
	gotUnknown, err := parseINIKeys(strings.NewReader(string(edited)), &got)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(got, want) || !slices.Equal(gotUnknown, wantUnknown) {
		return fmt.Errorf("could not change %s.%s without touching other settings, file left as is", section, key)
	}
	return nil
}

// setINIValue returns data with section.key set to value. It replaces
// the last matching line (the one parseINI uses), keeping its indent,
// quotes and inline comment. A missing key goes after the section's last
// key, a missing section at the end. Line endings follow the file.
func setINIValue(data []byte, section, key, value string) []byte {
	text := string(data)
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if text == "" {
		lines = nil
	}

	match, lastKey, header := -1, -1, -1
	current := ""
	for i, raw := range lines {
		line := iniLine(strings.TrimSuffix(raw, "\r"))
		if name, ok := iniSection(line); ok {
			current = name
			if name == section {
				header, lastKey = i, i
			}
			continue
		}
		if current != section || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if eq := strings.Index(line, "="); eq > 0 {
			lastKey = i
			if strings.ToLower(strings.TrimSpace(line[:eq])) == key {
				match = i
			}
		}
	}

	switch {
	case match >= 0:
		cr := strings.HasSuffix(lines[match], "\r")
		lines[match] = replaceINIValue(strings.TrimSuffix(lines[match], "\r"), value)
		if cr {
			lines[match] += "\r"
		}
	case header >= 0:
		lines = slices.Insert(lines, lastKey+1, key+" = "+value+strings.TrimSuffix(eol, "\n"))
	default:
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, strings.TrimSuffix(eol, "\n"))
		}
		cr := strings.TrimSuffix(eol, "\n")
		lines = append(lines, "["+section+"]"+cr, key+" = "+value+cr)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

// replaceINIValue swaps the value in one key = value line. The inline
// comment stays at its column when the new value fits, and quotes stay
// if the old value had them.
func replaceINIValue(line, value string) string {
	eq := strings.Index(line, "=")
	end, comment := len(line), ""
	if idx := strings.Index(line, "#"); idx > eq {
		end, comment = idx, line[idx:]
	}
	if old := strings.TrimSpace(line[eq+1 : end]); len(old) >= 2 && old[0] == '"' && old[len(old)-1] == '"' {
		value = `"` + value + `"`
	}
	out := line[:eq+1] + " " + value
	if comment == "" {
		return out
	}
	if len(out) < end {
		return out + strings.Repeat(" ", end-len(out)) + comment
	}
	return out + " " + comment
}

// saveWarned is set after the first failed save, so a read-only config
// warns once, not on every theme change.
var saveWarned bool

func saveFailed(path string, err error) {
	if !saveWarned {
		log.Printf("Warning: cannot save to %s: %v (changes last until exit)", path, err) //nolint:gosec // G706: path from our config search
		saveWarned = true
	}
}

// checkConfigFlag returns the absolute path of a --config file, or an
// error if it is missing, a directory or unreadable.
func checkConfigFlag(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("config file %s: %w", path, pathErr(err))
	}
	if info.IsDir() {
		return "", fmt.Errorf("config file %s is a directory", path)
	}
	f, err := os.Open(abs) //nolint:gosec // G304: path given by the user with --config
	if err != nil {
		return "", fmt.Errorf("config file %s: %w", path, pathErr(err))
	}
	_ = f.Close()
	return abs, nil
}

// pathErr drops the operation and path from an *os.PathError, which the
// caller already names.
func pathErr(err error) error {
	if pe, ok := errors.AsType[*os.PathError](err); ok {
		return pe.Err
	}
	return err
}

// SaveTheme writes the current theme name to the user config file.
func SaveTheme(themeName string) {
	saveValue("theme", "name", themeName)
}

// SaveSensitivity writes the current mouse sensitivity to the user config file.
func SaveSensitivity(value int) {
	saveValue("mouse", "sensitivity", strconv.Itoa(value))
}

// SavePosition writes the current position (top/bottom) to the user config file.
func SavePosition(top bool) {
	pos := "bottom"
	if top {
		pos = "top"
	}
	saveValue("window", "position", pos)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil { //nolint:gosec // G301: config dir, not secrets
		return err
	}
	in, err := os.Open(src) //nolint:gosec // G304: user file path
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst) //nolint:gosec // G304: user file path
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, in)
	return err
}

// ValidateConfig checks config ranges and logs warnings.
// checkConfig returns a list of human-readable config issues without mutating cfg.
// Used by --setup to report problems before the user tries to start.
// knownSections lists all valid INI section headers.
var knownSections = map[string]bool{
	"theme": true, "window": true, "keys": true,
	"gamepad": true, "gamepad.buttons": true, "mouse": true,
}

// checkConfigFile scans the raw config file for unknown section headers.
// These cause silent key-dropping that checkConfig can't detect from the parsed struct.
func checkConfigFile(path string) []string {
	f, err := os.Open(path) //nolint:gosec // G304: user config path from known locations
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var issues []string
	boolKeys := map[string]bool{"grab": true, "enabled": true, "panel_avoid": true, "key_shadow": true}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := iniLine(scanner.Text())
		if section, ok := iniSection(line); ok {
			if !knownSections[section] {
				issues = append(issues, fmt.Sprintf("unknown section [%s] - keys under it are ignored (check for typos)", section))
			}
			continue
		}
		// Check bool fields for invalid values
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			if boolKeys[key] {
				val := strings.TrimSpace(line[idx+1:])
				if ci := strings.Index(val, "#"); ci > 0 {
					val = strings.TrimSpace(val[:ci])
				}
				if _, err := strconv.ParseBool(val); err != nil {
					issues = append(issues, fmt.Sprintf("%s = %q is not a valid bool (use true/false)", key, val))
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return issues
	}
	if _, err := f.Seek(0, io.SeekStart); err == nil {
		var scratch Config
		unknown, _ := parseINIKeys(f, &scratch)
		for _, k := range unknown {
			issues = append(issues, fmt.Sprintf("unknown key %s - ignored (check for typos)", k))
		}
	}
	return issues
}

func checkConfig(cfg Config) []string {
	var issues []string
	if cfg.Gamepad.Deadzone < 0 || cfg.Gamepad.Deadzone > 1 {
		issues = append(issues, fmt.Sprintf("deadzone %.2f out of range [0,1]", cfg.Gamepad.Deadzone))
	}
	if cfg.Mouse.Sensitivity < 1 || cfg.Mouse.Sensitivity > 50 {
		issues = append(issues, fmt.Sprintf("sensitivity %d out of range [1,50]", cfg.Mouse.Sensitivity))
	}
	if cfg.Gamepad.LongPressMs < 100 || cfg.Gamepad.LongPressMs > 5000 {
		issues = append(issues, fmt.Sprintf("long_press_ms %d out of range [100,5000]", cfg.Gamepad.LongPressMs))
	}
	if cfg.Keys.Scale < 30 || cfg.Keys.Scale > 100 {
		issues = append(issues, fmt.Sprintf("scale %d out of range [30,100]", cfg.Keys.Scale))
	}
	if _, ok := Themes[cfg.Theme.Name]; !ok {
		issues = append(issues, fmt.Sprintf("unknown theme %q", cfg.Theme.Name))
	}
	if k := cfg.Theme.KeyStyle; k != "square" && k != "rounded" {
		issues = append(issues, fmt.Sprintf("key_style %q is not valid (use square or rounded)", k))
	}
	if cfg.Keys.RepeatDelayMs < 100 || cfg.Keys.RepeatDelayMs > 2000 {
		issues = append(issues, fmt.Sprintf("repeat_delay_ms %d out of range [100,2000]", cfg.Keys.RepeatDelayMs))
	}
	if cfg.Keys.RepeatRateMs < 20 || cfg.Keys.RepeatRateMs > 500 {
		issues = append(issues, fmt.Sprintf("repeat_rate_ms %d out of range [20,500]", cfg.Keys.RepeatRateMs))
	}
	if cfg.Gamepad.ComboPeriodMs < 50 || cfg.Gamepad.ComboPeriodMs > 2000 {
		issues = append(issues, fmt.Sprintf("combo_period_ms %d out of range [50,2000]", cfg.Gamepad.ComboPeriodMs))
	}
	if p := cfg.Window.Position; p != "bottom" && p != "top" {
		issues = append(issues, fmt.Sprintf("position %q is not valid (use bottom or top)", p))
	}
	if cfg.Window.Opacity < minOpacity || cfg.Window.Opacity > 1 {
		issues = append(issues, fmt.Sprintf("opacity %.2f out of range [%.1f,1]", cfg.Window.Opacity, minOpacity))
	}
	if m := cfg.Gamepad.MouseStick; m != "" && m != "left" && m != "right" {
		issues = append(issues, fmt.Sprintf("mouse_stick %q is not valid (use left or right)", m))
	}
	if s := cfg.Gamepad.SwapXY; s != "" && s != "auto" && s != "true" && s != "false" {
		issues = append(issues, fmt.Sprintf("swap_xy %q is not valid (use auto, true, or false)", s))
	}
	if !validController(cfg.Gamepad.Controller) {
		issues = append(issues, fmt.Sprintf("controller %q is not valid (use auto, xbox, playstation, nintendo, 8bitdo or generic)", cfg.Gamepad.Controller))
	}
	if cfg.Gamepad.ToggleCombo != "" {
		if _, err := parseComboString(cfg.Gamepad.ToggleCombo); err != nil {
			issues = append(issues, fmt.Sprintf("invalid toggle_combo %q: %v", cfg.Gamepad.ToggleCombo, err))
		}
	}
	return issues
}

// minOpacity keeps the keyboard visible: at opacity 0 it would be invisible
// but still take the controller.
const minOpacity = 0.2

func ValidateConfig(cfg *Config) {
	if cfg.Window.Opacity < minOpacity {
		log.Printf("Warning: opacity %.2f below %.1f, using %.1f", cfg.Window.Opacity, minOpacity, minOpacity)
		cfg.Window.Opacity = minOpacity
	}
	if cfg.Window.Opacity > 1 {
		log.Printf("Warning: opacity %.2f above 1, using 1", cfg.Window.Opacity)
		cfg.Window.Opacity = 1
	}
	if cfg.Gamepad.Deadzone < 0 || cfg.Gamepad.Deadzone > 1 {
		log.Printf("Warning: deadzone %.2f out of range [0,1], using 0.25", cfg.Gamepad.Deadzone)
		cfg.Gamepad.Deadzone = 0.25
	}
	if cfg.Mouse.Sensitivity < 1 || cfg.Mouse.Sensitivity > 50 {
		log.Printf("Warning: sensitivity %d out of range [1,50], using 10", cfg.Mouse.Sensitivity)
		cfg.Mouse.Sensitivity = 10
	}
	if cfg.Gamepad.LongPressMs < 100 || cfg.Gamepad.LongPressMs > 5000 {
		log.Printf("Warning: long_press_ms %d out of range [100,5000], using 500", cfg.Gamepad.LongPressMs)
		cfg.Gamepad.LongPressMs = 500
	}
	if cfg.Keys.Scale < 30 || cfg.Keys.Scale > 100 {
		log.Printf("Warning: scale %d out of range [30,100], using 70", cfg.Keys.Scale)
		cfg.Keys.Scale = 50
	}
	if _, ok := Themes[cfg.Theme.Name]; !ok {
		log.Printf("Warning: unknown theme %q, using dark", cfg.Theme.Name)
		cfg.Theme.Name = "matrix"
	}
	if k := cfg.Theme.KeyStyle; k != "square" && k != "rounded" {
		log.Printf("Warning: key_style %q is not valid, using square", k)
		cfg.Theme.KeyStyle = "square"
	}
	if cfg.Keys.RepeatDelayMs < 100 || cfg.Keys.RepeatDelayMs > 2000 {
		log.Printf("Warning: repeat_delay_ms %d out of range [100,2000], using 400", cfg.Keys.RepeatDelayMs)
		cfg.Keys.RepeatDelayMs = 400
	}
	if cfg.Keys.RepeatRateMs < 20 || cfg.Keys.RepeatRateMs > 500 {
		log.Printf("Warning: repeat_rate_ms %d out of range [20,500], using 80", cfg.Keys.RepeatRateMs)
		cfg.Keys.RepeatRateMs = 80
	}
	if cfg.Gamepad.ComboPeriodMs < 50 || cfg.Gamepad.ComboPeriodMs > 2000 {
		log.Printf("Warning: combo_period_ms %d out of range [50,2000], using 200", cfg.Gamepad.ComboPeriodMs)
		cfg.Gamepad.ComboPeriodMs = 200
	}
	if cfg.Gamepad.ToggleCombo != "" {
		if _, err := parseComboString(cfg.Gamepad.ToggleCombo); err != nil {
			log.Printf("Warning: invalid toggle_combo %q: %v - disabling", cfg.Gamepad.ToggleCombo, err)
			cfg.Gamepad.ToggleCombo = ""
		}
	}
	if !validController(cfg.Gamepad.Controller) {
		log.Printf("Warning: unknown controller %q, using auto", cfg.Gamepad.Controller)
		cfg.Gamepad.Controller = ProfileAuto
	}
}

// validController accepts auto, a profile ID, or empty (same as auto).
func validController(id string) bool {
	if id == "" || id == ProfileAuto {
		return true
	}
	_, ok := ControllerProfiles[id]
	return ok
}
