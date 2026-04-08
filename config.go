package main

import (
	"io"
	"log"
	"os"
	"os/user"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Debugf logs only when --verbose is set
func Debugf(format string, args ...any) {
	if Verbose {
		log.Printf("[DEBUG] "+format, args...) //nolint:gosec // G706: format string is from our code, not user input
	}
}

type Config struct {
	Theme   ThemeConfig   `toml:"theme"`
	Window  WindowConfig  `toml:"window"`
	Keys    KeysConfig    `toml:"keys"`
	Gamepad GamepadConfig `toml:"gamepad"`
	Mouse   MouseConfig   `toml:"mouse"`
}

type ThemeConfig struct {
	Name string `toml:"name"`
}

type WindowConfig struct {
	Position     string  `toml:"position"`     // "bottom" or "top"
	Margin       int     `toml:"margin"`
	BottomMargin int     `toml:"bottom_margin"` // deprecated, migrated to Margin
	Opacity      float64 `toml:"opacity"`
}

type KeysConfig struct {
	UnitSize      int `toml:"unit_size"`
	Padding       int `toml:"padding"`
	FontSize      int `toml:"font_size"`
	Scale         int `toml:"scale"`          // percentage of screen width (30-100, default 70)
	RepeatDelayMs int `toml:"repeat_delay_ms"` // ms before key repeat starts (default 400)
	RepeatRateMs  int `toml:"repeat_rate_ms"`  // ms between repeats (default 80)
}

type ButtonsConfig struct {
	Press      string `toml:"press"`
	Close      string `toml:"close"`
	Backspace  string `toml:"backspace"`
	Space      string `toml:"space"`
	Shift      string `toml:"shift"`
	Enter      string `toml:"enter"`
	LeftClick      string `toml:"left_click"`
	RightClick     string `toml:"right_click"`
	PositionToggle string `toml:"position_toggle"`
}

type GamepadConfig struct {
	Device      string        `toml:"device"`
	Grab        bool          `toml:"grab"`
	GrabDevice  string        `toml:"grab_device"`
	Deadzone    float64       `toml:"deadzone"`
	LongPressMs int           `toml:"long_press_ms"`
	SwapXY      string        `toml:"swap_xy"`     // "auto", "true", "false"
	MouseStick  string        `toml:"mouse_stick"`  // "left" or "right" (nav uses the other stick)
	Buttons     ButtonsConfig `toml:"buttons"`
}

type MouseConfig struct {
	Enabled     bool `toml:"enabled"`
	Sensitivity int  `toml:"sensitivity"`
}

func DefaultConfig() Config {
	return Config{
		Theme:  ThemeConfig{Name: "dark"},
		Window: WindowConfig{Position: "bottom", Margin: 20, Opacity: 0.95},
		Keys:   KeysConfig{UnitSize: 0, Padding: 4, FontSize: 0, Scale: 70, RepeatDelayMs: 400, RepeatRateMs: 80},
		Gamepad: GamepadConfig{
			Grab:        true,
			Deadzone:    0.25,
			LongPressMs: 500,
			SwapXY:      "auto",
			MouseStick:  "right",
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
	return filepath.Join(xdg, "gamepad-osk", "config.toml")
}

func LoadConfig(overridePath string) Config {
	cfg := DefaultConfig()

	// Priority: --config flag > user config > system config > next to binary > cwd
	var paths []string
	if overridePath != "" {
		paths = append(paths, overridePath)
	}
	paths = append(paths,
		UserConfigPath(),
		"/etc/gamepad-osk/config.toml",
	)
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), "config.toml"))
	}
	paths = append(paths, "config.toml")

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil { //nolint:gosec // G703: config paths are trusted
			if _, err := toml.DecodeFile(p, &cfg); err != nil {
				log.Printf("Warning: error parsing %s: %v", p, err) //nolint:gosec // G706: log format from our code
			} else {
				log.Printf("Loaded config from %s", p) //nolint:gosec // G706: log format from our code
			}
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

	// Auto-copy: if no user config exists, copy from system/binary dir
	userPath := UserConfigPath()
	if _, err := os.Stat(userPath); os.IsNotExist(err) {
		for _, src := range paths[1:] { // skip user path itself
			if _, err := os.Stat(src); err == nil { //nolint:gosec // G703: config paths are trusted
				if copyFile(src, userPath) == nil {
					log.Printf("Created user config at %s", userPath)
				}
				break
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
		if _, decErr := toml.DecodeFile(path, &cfg); decErr != nil { //nolint:gosec // G304: user config path
			log.Printf("Warning: error reading config %s: %v", path, decErr)
		}
	}
	mutate(&cfg)

	f, err := os.Create(path) //nolint:gosec // G304: user config path
	if err != nil {
		log.Printf("Warning: cannot write config: %v", err)
		return
	}
	defer func() { _ = f.Close() }()

	enc := toml.NewEncoder(f)
	if err := enc.Encode(cfg); err != nil {
		log.Printf("Warning: cannot encode config: %v", err)
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
		cfg.Keys.Scale = 70
	}
	if _, ok := Themes[cfg.Theme.Name]; !ok {
		log.Printf("Warning: unknown theme %q, using dark", cfg.Theme.Name)
		cfg.Theme.Name = "dark"
	}
	if cfg.Keys.RepeatDelayMs < 100 || cfg.Keys.RepeatDelayMs > 2000 {
		log.Printf("Warning: repeat_delay_ms %d out of range [100,2000], using 400", cfg.Keys.RepeatDelayMs)
		cfg.Keys.RepeatDelayMs = 400
	}
	if cfg.Keys.RepeatRateMs < 20 || cfg.Keys.RepeatRateMs > 500 {
		log.Printf("Warning: repeat_rate_ms %d out of range [20,500], using 80", cfg.Keys.RepeatRateMs)
		cfg.Keys.RepeatRateMs = 80
	}
}
