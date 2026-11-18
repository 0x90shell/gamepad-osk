package main

import (
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
)

var version = "2.1.1"

// Verbose controls debug logging throughout the app
var Verbose bool

// options holds the parsed command line.
type options struct {
	toggle, version, help bool
	setup, install        bool
	daemon, verbose       bool
	device, theme, config string
}

// parseArgs parses the command line. It acts on nothing; main does that
// after every argument is read.
func parseArgs(args []string) (options, error) {
	var o options
	value := func(i *int, flag, what string) (string, error) {
		if *i+1 >= len(args) {
			return "", fmt.Errorf("%s requires %s", flag, what)
		}
		*i++
		return args[*i], nil
	}
	i := 0
	for i < len(args) {
		var err error
		switch args[i] {
		case "--toggle":
			o.toggle = true
		case "--version", "-V":
			o.version = true
		case "--help", "-h":
			o.help = true
		case "--setup":
			o.setup = true
		case "--install":
			o.install = true
		case "--daemon":
			o.daemon = true
		case "--verbose", "-v":
			o.verbose = true
		case "--device", "-d":
			o.device, err = value(&i, "--device", "a path")
		case "--theme", "-t":
			o.theme, err = value(&i, "--theme", "a name")
		case "--config", "-c":
			o.config, err = value(&i, "--config", "a path")
		default:
			if strings.HasPrefix(args[i], "/") {
				o.device = args[i]
			}
		}
		if err != nil {
			return o, err
		}
		i++
	}
	return o, nil
}

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	Verbose = opts.verbose
	if opts.version {
		fmt.Printf("gamepad-osk v%s\n", version)
		os.Exit(0)
	}
	if opts.help {
		printHelp(LoadConfig(""))
		os.Exit(0)
	}
	if opts.toggle {
		if IPCSend("toggle") {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "No running instance, starting new...")
	}
	devicePath, themeName, configPath := opts.device, opts.theme, opts.config
	daemon, setupMode, installMode := opts.daemon, opts.setup, opts.install

	// --install requires --setup
	if installMode && !setupMode {
		fmt.Fprintln(os.Stderr, "Error: --install requires --setup")
		fmt.Fprintln(os.Stderr, "Usage: gamepad-osk --setup --install")
		os.Exit(1)
	}

	// --setup mode: diagnose or install, then exit
	if setupMode {
		os.Exit(runSetup(installMode))
	}

	config := LoadConfig(configPath)

	// Flags override config
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

	// Check for existing instance before starting
	Debugf("Checking for existing instance...")
	if IPCSend("ping") {
		fmt.Fprintln(os.Stderr, "Error: another instance is already running")
		fmt.Fprintln(os.Stderr, "Use --toggle to show/hide, or stop the other instance first")
		os.Exit(1)
	}
	// Socket exists but we can't connect - may be owned by another user (sudo)
	if socketOwnedByOther(sockPath) {
		fmt.Fprintln(os.Stderr, "Error: another instance may be running (socket owned by different user)")
		fmt.Fprintln(os.Stderr, "Stop it: sudo pkill -x gamepad-osk")
		fmt.Fprintf(os.Stderr, "If already stopped: sudo rm %s\n", sockPath)
		os.Exit(1)
	}
	Debugf("No existing instance found")

	app := NewApp(config)
	if daemon {
		app.daemon = true
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
		"start": "Start", "select": "Select", "guide": "Guide",
	}
	if l, ok := labels[name]; ok {
		return l
	}
	return strings.ToUpper(name)
}

func printHelp(cfg Config) {
	b := cfg.Gamepad.Buttons
	mouse := cfg.Gamepad.MouseStick
	if mouse == "" {
		mouse = "right"
	}
	nav := "left"
	if mouse == "left" {
		nav = "right"
	}
	mouseC := strings.ToUpper(mouse[:1]) + mouse[1:]
	navC := strings.ToUpper(nav[:1]) + nav[1:]

	var comboHelp string
	if cfg.Gamepad.ToggleCombo != "" {
		if _, err := parseComboString(cfg.Gamepad.ToggleCombo); err != nil {
			comboHelp = fmt.Sprintf("  INVALID: %s (%v)\n  Change in config: toggle_combo, combo_period_ms",
				cfg.Gamepad.ToggleCombo, err)
		} else {
			comboHelp = fmt.Sprintf("  Active: %-20s (%dms window)\n  Change in config: toggle_combo, combo_period_ms\n"+
				"  Buttons: a, b, x, y, lb, rb, lt, rt, l3, r3, start, select, guide, up, down, left, right",
				cfg.Gamepad.ToggleCombo, cfg.Gamepad.ComboPeriodMs)
		}
	} else {
		comboHelp = "  Disabled. Set in config:\n" +
			"    toggle_combo = guide+a         # 2-4 buttons, + separated\n" +
			"    combo_period_ms = 200          # timing window (ms)\n" +
			"  Buttons: a, b, x, y, lb, rb, lt, rt, l3, r3, start, select, guide, up, down, left, right"
	}

	fmt.Printf(`gamepad-osk v%s - Gamepad-controlled on-screen keyboard for Linux

USAGE
  gamepad-osk [options] [/dev/input/device]

OPTIONS
  --device, -d PATH    Input device (overrides config, overrides auto-detect)
  --theme, -t NAME     Color theme (overrides config)
  --config, -c PATH    Config file path (overrides search order)
  --toggle             Toggle visibility of running instance (for evsieve/hotkey)
  --daemon             Start hidden, wait for toggle combo or --toggle to show
                       Close button (B) hides instead of exiting in daemon mode
  --setup              Check system configuration (udev, permissions, config)
  --setup --install    Deploy udev rules, config, and systemd service
  --verbose, -v        Verbose logging (gamepad events, key injection, config)
  --version, -V        Print version and exit
  --help, -h           Show this help

DEVICE PRIORITY
  1. --device flag or bare /dev/input/... argument
  2. device = ... in config
  3. Auto-detect from /proc/bus/input/devices

CONTROLS (from config)
  %-24s Navigate keyboard
  %-24s Move mouse cursor
  %-24s Press highlighted key (hold to repeat)
  %-24s Close keyboard
  %-24s Backspace (hold to repeat)
  %-24s Space (hold to repeat)
  %-24s Shift
  %-24s Enter (hold to repeat)
  %-24s Left mouse click (hold to drag)
  %-24s Right mouse click
  %-24s Left mouse click (stick click)
  %-24s Caps Lock
  %-24s Toggle keyboard top/bottom
  %-24s Accent popup (on vowels: é, ñ, ü)
  Shift + up/down arrow     Adjust mouse sensitivity (saved to config)

TOGGLE COMBO (config: toggle_combo)
%s

THEMES
  %s
  Cycle live with the Theme key on the keyboard (Shift = reverse)

CONFIG (first found)
  1. --config flag
  2. ~/.config/gamepad-osk/config
  3. /etc/gamepad-osk/config
  4. config next to binary
  5. config in working directory

NOTES
  Controller auto-reconnects if disconnected (timeout, power-off, unplug).
  Wayland: panel_avoid controls panel spacing (true = respect panels, false = screen edge).

REQUIREMENTS
  Runtime: sdl3, sdl3_ttf, wayland, libx11, ttf-promptfont (AUR)
  User must be in 'input' group for gamepad and key injection
`,
		version,
		navC+" stick / D-pad",
		mouseLbl(cfg.Mouse.Enabled, mouseC+" stick"),
		buttonLabel(b.Press), buttonLabel(b.Close),
		buttonLabel(b.Backspace), buttonLabel(b.Space),
		buttonLabel(b.Shift), buttonLabel(b.Enter),
		mouseLbl(cfg.Mouse.Enabled, buttonLabel(b.LeftClick)),
		mouseLbl(cfg.Mouse.Enabled, buttonLabel(b.RightClick)),
		mouseLbl(cfg.Mouse.Enabled, mouseC+" stick click"),
		navC+" stick click",
		posToggleLabel(b.PositionToggle),
		"Shift + hold A",
		comboHelp,
		availableThemes())
}

func mouseLbl(enabled bool, label string) string {
	if !enabled {
		return "(disabled)"
	}
	return label
}

func posToggleLabel(name string) string {
	if name == "" {
		return "(disabled)"
	}
	return buttonLabel(name)
}

func availableThemes() string {
	var names []string
	for name := range Themes {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
