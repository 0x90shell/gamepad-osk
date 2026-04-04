package main

import (
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
)

// Verbose controls debug logging throughout the app
var Verbose bool

func main() {
	args := os.Args[1:]

	var devicePath, themeName string
	daemon := false

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--toggle":
			if IPCSend("toggle") {
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, "No running instance, starting new...")
		case "--help", "-h":
			printHelp(LoadConfig())
			os.Exit(0)
		case "--daemon":
			daemon = true
		case "--verbose", "-v":
			Verbose = true
		case "--device", "-d":
			if i+1 < len(args) {
				i++
				devicePath = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "Error: --device requires a path")
				os.Exit(1)
			}
		case "--theme", "-t":
			if i+1 < len(args) {
				i++
				themeName = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "Error: --theme requires a name")
				os.Exit(1)
			}
		default:
			if strings.HasPrefix(args[i], "/") {
				devicePath = args[i]
			}
		}
		i++
	}

	config := LoadConfig()

	// Flags override TOML
	if devicePath != "" {
		config.Gamepad.Device = devicePath
	}
	if themeName != "" {
		if _, ok := Themes[themeName]; !ok {
			fmt.Fprintf(os.Stderr, "Error: unknown theme %q\n", themeName)
			fmt.Fprintf(os.Stderr, "Available: %s\n", availableThemes())
			os.Exit(1)
		}
		config.Theme.Name = themeName
	}

	app := NewApp(config)
	if daemon {
		app.visible = false
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		app.running = false
	}()

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// buttonLabel returns the uppercase button name for a config value
func buttonLabel(name string) string {
	labels := map[string]string{
		"a": "A", "b": "B", "x": "X", "y": "Y",
		"lb": "LB", "rb": "RB", "lt": "LT (hold)", "rt": "RT",
		"l3": "L3 (stick click)", "r3": "R3 (stick click)",
		"start": "Start", "select": "Select",
	}
	if l, ok := labels[name]; ok {
		return l
	}
	return strings.ToUpper(name)
}

func printHelp(cfg Config) {
	b := cfg.Gamepad.Buttons
	nav := cfg.Gamepad.NavStick
	mouse := cfg.Gamepad.MouseStick
	if nav == "" {
		nav = "left"
	}
	if mouse == "" {
		mouse = "right"
	}

	fmt.Printf(`gamepad-osk — Gamepad-controlled on-screen keyboard for Linux

USAGE
  gamepad-osk [options] [/dev/input/device]

OPTIONS
  --device, -d PATH    Input device (overrides config.toml, overrides auto-detect)
  --theme, -t NAME     Color theme (overrides config.toml)
  --toggle             Toggle visibility of running instance (for evsieve/hotkey)
  --daemon             Start hidden, wait for --toggle to show
  --verbose, -v        Verbose logging (gamepad events, key injection, config)
  --help, -h           Show this help

DEVICE PRIORITY
  1. --device flag or bare /dev/input/... argument
  2. device = "..." in config.toml
  3. Auto-detect from /proc/bus/input/devices

CONTROLS (from config)
  %s stick / D-pad     Navigate keyboard
  %s stick             Move mouse cursor
  %-20s Press highlighted key
  %-20s Close keyboard
  %-20s Backspace
  %-20s Space
  %-20s Shift
  %-20s Enter
  %-20s Left mouse click (hold to drag)
  %-20s Right mouse click
  %-20s Caps Lock
  %-20s Toggle keyboard top/bottom
  Long-press A           Accent popup (on vowels)

THEMES
  %s
  Cycle live with the Cfg key on the keyboard

CONFIG
  ~/.config/gamepad-osk/config.toml    (or config.toml next to binary)

REQUIREMENTS
  Runtime: sdl2, sdl2_ttf, ttf-promptfont (AUR)
  User must be in 'input' group for key injection
`,
		strings.ToUpper(nav[:1])+nav[1:], strings.ToUpper(mouse[:1])+mouse[1:],
		buttonLabel(b.Press), buttonLabel(b.Close),
		buttonLabel(b.Backspace), buttonLabel(b.Space),
		buttonLabel(b.Shift), buttonLabel(b.Enter),
		buttonLabel(b.LeftClick), buttonLabel(b.RightClick),
		buttonLabel(b.Caps), buttonLabel(b.PositionToggle),
		availableThemes())
}

func availableThemes() string {
	var names []string
	for name := range Themes {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
