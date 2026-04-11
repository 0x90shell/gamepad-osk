package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/user"
	"path/filepath"
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
	Name string
}

type WindowConfig struct {
	Position     string  // "bottom" or "top"
	Margin       int
	BottomMargin int     // deprecated, migrated to Margin
	Opacity      float64
}

type KeysConfig struct {
	UnitSize      int
	Padding       int
	FontSize      int
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
		Theme:  ThemeConfig{Name: "matrix"},
		Window: WindowConfig{Position: "bottom", Margin: 0, Opacity: 0.95},
		Keys:   KeysConfig{UnitSize: 0, Padding: 4, FontSize: 0, Scale: 50, RepeatDelayMs: 400, RepeatRateMs: 80},
		Gamepad: GamepadConfig{
			Grab:          true,
			Deadzone:      0.25,
			LongPressMs:   500,
			SwapXY:        "auto",
			MouseStick:    "right",
			ComboPeriodMs: 200,
			Buttons: ButtonsConfig{
				Press:      "a",
				Close:      "b",
				Backspace:  "x",
				Space:      "y",
				Shift:      "lt",
				Enter:      "rt",
				LeftClick:      "rb",
				RightClick:     "lb",
				PositionToggle: "start",
			},
		},
		Mouse: MouseConfig{Enabled: true, Sensitivity: 8},
	}
}

// --- INI config parser/writer ---

// parseINI reads an INI-style config file into a Config.
// Supports [section] and [section.subsection] headers, key = value pairs,
// # comments, and inline # comments.
func parseINI(r io.Reader, cfg *Config) error {
	scanner := bufio.NewScanner(r)
	section := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(line[1 : len(line)-1])
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
		key := strings.TrimSpace(parts[0])
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
			default:
				Debugf("unknown config key: theme.%s", key)
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
			default:
				Debugf("unknown config key: window.%s", key)
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
			case "font_size":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Keys.FontSize = n
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
				Debugf("unknown config key: keys.%s", key)
			}
		case "gamepad":
			switch key {
			case "device":
				cfg.Gamepad.Device = val
			case "grab":
				if b, err := strconv.ParseBool(val); err == nil {
					cfg.Gamepad.Grab = b
				}
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
			case "mouse_stick":
				cfg.Gamepad.MouseStick = val
			case "toggle_combo":
				cfg.Gamepad.ToggleCombo = val
			case "combo_period_ms":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Gamepad.ComboPeriodMs = n
				}
			default:
				Debugf("unknown config key: gamepad.%s", key)
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
				Debugf("unknown config key: gamepad.buttons.%s", key)
			}
		case "mouse":
			switch key {
			case "enabled":
				if b, err := strconv.ParseBool(val); err == nil {
					cfg.Mouse.Enabled = b
				}
			case "sensitivity":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Mouse.Sensitivity = n
				}
			default:
				Debugf("unknown config key: mouse.%s", key)
			}
		default:
			log.Printf("Warning: unknown config section [%s], key %q ignored (check section headers)", section, key) //nolint:gosec // G706: section/key from user's own config file
		}
	}

	return scanner.Err()
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
	b.WriteString(line(kv("name", cfg.Theme.Name), "see --help for full list (60 themes)"))
	b.WriteString("\n")

	b.WriteString("[window]\n")
	b.WriteString(line(kv("position", cfg.Window.Position), "bottom or top"))
	b.WriteString(line(kvf("margin", cfg.Window.Margin), "pixels from screen edge"))
	b.WriteString(line(kv("opacity", strconv.FormatFloat(cfg.Window.Opacity, 'f', -1, 64)), "0.0-1.0 (1.0 = fully opaque)"))
	b.WriteString("\n")

	b.WriteString("[keys]\n")
	b.WriteString(line(kvf("unit_size", cfg.Keys.UnitSize), "0 = auto-scale, or fixed pixel size"))
	b.WriteString(line(kvf("padding", cfg.Keys.Padding), "pixels between keys"))
	b.WriteString(line(kvf("font_size", cfg.Keys.FontSize), "0 = auto-scale relative to unit_size"))
	b.WriteString(line(kvf("scale", cfg.Keys.Scale), "percentage of screen width (30-100)"))
	b.WriteString(line(kvf("repeat_delay_ms", cfg.Keys.RepeatDelayMs), "ms before key repeat starts (100-2000)"))
	b.WriteString(line(kvf("repeat_rate_ms", cfg.Keys.RepeatRateMs), "ms between repeats (20-500)"))
	b.WriteString("\n")

	b.WriteString("[gamepad]\n")
	b.WriteString(line(kv("device", cfg.Gamepad.Device), "empty = auto-detect, or /dev/input/eventX"))
	b.WriteString(line(kvf("grab", cfg.Gamepad.Grab), "grab device when visible (prevents input bleed to game)"))
b.WriteString(line(kv("deadzone", strconv.FormatFloat(cfg.Gamepad.Deadzone, 'f', -1, 64)), "stick deadzone (0.0-1.0)"))
	b.WriteString(line(kvf("long_press_ms", cfg.Gamepad.LongPressMs), "ms to hold for accent popup (100-5000)"))
	b.WriteString(line(kv("swap_xy", cfg.Gamepad.SwapXY), "auto = detect Xbox 360, true = force swap, false = no swap"))
	b.WriteString(line(kv("mouse_stick", cfg.Gamepad.MouseStick), "left or right - nav uses the other stick"))
	b.WriteString(line(kv("toggle_combo", cfg.Gamepad.ToggleCombo), "built-in show/hide combo (empty = disabled, use --toggle/evsieve)"))
	b.WriteString("                             # format: button+button (2-4 buttons, + separated)\n")
	b.WriteString("                             # buttons: a, b, x, y, lb, rb, lt, rt, l3, r3, start, select, guide,\n")
	b.WriteString("                             #          dpad_up, dpad_down, dpad_left, dpad_right\n")
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
	b.WriteString(line(kv("position_toggle", cfg.Gamepad.Buttons.PositionToggle), "toggle keyboard top/bottom"))
	b.WriteString("\n")

	b.WriteString("[mouse]\n")
	b.WriteString(line(kvf("enabled", cfg.Mouse.Enabled), "enable mouse cursor via right stick"))
	b.WriteString(line(kvf("sensitivity", cfg.Mouse.Sensitivity), "1-50, higher = faster cursor"))

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

// UserConfigPath returns the user's config file path (XDG standard).
// When run via sudo, resolves the real user's home via SUDO_USER.
func UserConfigPath() string {
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
	return filepath.Join(xdg, "gamepad-osk", "config")
}

// legacyConfigPath returns the old TOML config path for migration.
func legacyConfigPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		if sudoHomeResolved == "" {
			sudoHomeResolved = os.Getenv("HOME")
			if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && sudoHomeResolved == "/root" {
				if u, err := user.Lookup(sudoUser); err == nil {
					sudoHomeResolved = u.HomeDir
				}
			}
		}
		xdg = filepath.Join(sudoHomeResolved, ".config")
	}
	return filepath.Join(xdg, "gamepad-osk", "config.toml")
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

	// Priority: --config flag > user config > system config > next to binary > cwd
	var paths []string
	if overridePath != "" {
		paths = append(paths, overridePath)
	}
	paths = append(paths,
		userPath,
		"/etc/gamepad-osk/config",
	)
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), "config"))
	}
	paths = append(paths, "config")

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

	// Auto-create user config if it doesn't exist.
	// Prefer copying from a system/binary config file; fall back to writing defaults.
	if _, err := os.Stat(userPath); os.IsNotExist(err) {
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

// saveConfig loads the user config, applies mutator, and writes it back.
func saveConfig(mutate func(*Config)) {
	path := UserConfigPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil { //nolint:gosec // G301: config dir, not secrets
		log.Printf("Warning: cannot create config dir: %v", err)
		return
	}

	cfg := DefaultConfig()
	if _, err := os.Stat(path); err == nil {
		f, err := os.Open(path) //nolint:gosec // G304: user config path
		if err != nil {
			log.Printf("Warning: error reading config %s: %v", path, err)
		} else {
			if parseErr := parseINI(f, &cfg); parseErr != nil {
				log.Printf("Warning: error parsing config %s: %v", path, parseErr)
			}
			_ = f.Close()
		}
	}
	mutate(&cfg)

	f, err := os.Create(path) //nolint:gosec // G304: user config path
	if err != nil {
		log.Printf("Warning: cannot write config: %v", err)
		return
	}
	defer func() { _ = f.Close() }()

	if err := writeINI(f, cfg); err != nil {
		log.Printf("Warning: cannot write config: %v", err)
	}
}

// SaveTheme writes the current theme name to the user config file.
func SaveTheme(themeName string) {
	saveConfig(func(cfg *Config) {
		cfg.Theme.Name = themeName
	})
}

// SavePosition writes the current position (top/bottom) to the user config file.
func SavePosition(top bool) {
	pos := "bottom"
	if top {
		pos = "top"
	}
	saveConfig(func(cfg *Config) {
		cfg.Window.Position = pos
	})
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
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "[") {
			continue
		}
		// Strip inline comment before checking suffix
		if idx := strings.Index(line, "#"); idx > 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if !strings.HasSuffix(line, "]") {
			continue
		}
		section := strings.ToLower(line[1 : len(line)-1])
		if !knownSections[section] {
			issues = append(issues, fmt.Sprintf("unknown section [%s] - keys under it are ignored (check for typos)", section))
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
	if cfg.Keys.RepeatDelayMs < 100 || cfg.Keys.RepeatDelayMs > 2000 {
		issues = append(issues, fmt.Sprintf("repeat_delay_ms %d out of range [100,2000]", cfg.Keys.RepeatDelayMs))
	}
	if cfg.Keys.RepeatRateMs < 20 || cfg.Keys.RepeatRateMs > 500 {
		issues = append(issues, fmt.Sprintf("repeat_rate_ms %d out of range [20,500]", cfg.Keys.RepeatRateMs))
	}
	if cfg.Gamepad.ComboPeriodMs < 50 || cfg.Gamepad.ComboPeriodMs > 2000 {
		issues = append(issues, fmt.Sprintf("combo_period_ms %d out of range [50,2000]", cfg.Gamepad.ComboPeriodMs))
	}
	if cfg.Gamepad.ToggleCombo != "" {
		if _, err := parseComboString(cfg.Gamepad.ToggleCombo); err != nil {
			issues = append(issues, fmt.Sprintf("invalid toggle_combo %q: %v", cfg.Gamepad.ToggleCombo, err))
		}
	}
	return issues
}

func ValidateConfig(cfg *Config) {
	if cfg.Gamepad.Deadzone < 0 || cfg.Gamepad.Deadzone > 1 {
		log.Printf("Warning: deadzone %.2f out of range [0,1], using 0.25", cfg.Gamepad.Deadzone)
		cfg.Gamepad.Deadzone = 0.25
	}
	if cfg.Mouse.Sensitivity < 1 || cfg.Mouse.Sensitivity > 50 {
		log.Printf("Warning: sensitivity %d out of range [1,50], using 8", cfg.Mouse.Sensitivity)
		cfg.Mouse.Sensitivity = 8
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
}
