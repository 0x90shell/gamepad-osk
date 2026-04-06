package main

import (
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Debugf logs only when --verbose is set
func Debugf(format string, args ...interface{}) {
	if Verbose {
		log.Printf("[DEBUG] "+format, args...)
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
	BottomMargin int     `toml:"bottom_margin"`
	Opacity      float64 `toml:"opacity"`
}

type KeysConfig struct {
	UnitSize int `toml:"unit_size"`
	Padding  int `toml:"padding"`
	FontSize int `toml:"font_size"`
	Scale    int `toml:"scale"` // percentage of screen width (30-100, default 70)
}

type ButtonsConfig struct {
	Press      string `toml:"press"`
	Close      string `toml:"close"`
	Backspace  string `toml:"backspace"`
	Space      string `toml:"space"`
	Caps       string `toml:"caps"`
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
	NavStick    string        `toml:"nav_stick"`    // "left" or "right"
	MouseStick  string        `toml:"mouse_stick"`  // "left" or "right"
	Buttons     ButtonsConfig `toml:"buttons"`
}

type MouseConfig struct {
	Enabled     bool `toml:"enabled"`
	Sensitivity int  `toml:"sensitivity"`
}

func DefaultConfig() Config {
	return Config{
		Theme:  ThemeConfig{Name: "dark"},
		Window: WindowConfig{Position: "bottom", BottomMargin: 20, Opacity: 0.95},
		Keys:   KeysConfig{UnitSize: 0, Padding: 4, FontSize: 0, Scale: 70},
		Gamepad: GamepadConfig{
			Grab:        true,
			Deadzone:    0.25,
			LongPressMs: 500,
			SwapXY:      "auto",
			NavStick:    "left",
			MouseStick:  "right",
			Buttons: ButtonsConfig{
				Press:      "a",
				Close:      "b",
				Backspace:  "x",
				Space:      "y",
				Caps:       "l3",
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

// UserConfigPath returns the user's config file path (XDG standard)
func UserConfigPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(xdg, "gamepad-osk", "config.toml")
}

func LoadConfig() Config {
	cfg := DefaultConfig()

	// Priority: user config > system config > next to binary > cwd
	paths := []string{
		UserConfigPath(),
		"/etc/gamepad-osk/config.toml",
	}
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), "config.toml"))
	}
	paths = append(paths, "config.toml")

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			if _, err := toml.DecodeFile(p, &cfg); err != nil {
				log.Printf("Warning: error parsing %s: %v", p, err)
			} else {
				log.Printf("Loaded config from %s", p)
			}
			break
		}
	}

	// Auto-copy: if no user config exists, copy from system/binary dir
	userPath := UserConfigPath()
	if _, err := os.Stat(userPath); os.IsNotExist(err) {
		for _, src := range paths[1:] { // skip user path itself
			if _, err := os.Stat(src); err == nil {
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("Warning: cannot create config dir: %v", err)
		return
	}

	cfg := DefaultConfig()
	if _, err := os.Stat(path); err == nil {
		toml.DecodeFile(path, &cfg)
	}
	mutate(&cfg)

	f, err := os.Create(path)
	if err != nil {
		log.Printf("Warning: cannot write config: %v", err)
		return
	}
	defer f.Close()

	enc := toml.NewEncoder(f)
	enc.Encode(cfg)
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
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
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
}
