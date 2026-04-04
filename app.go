package main

import (
	"log"
	"sort"
	"sync"

	"github.com/veandco/go-sdl2/sdl"
)

var themeOrder []string

func init() {
	for name := range Themes {
		themeOrder = append(themeOrder, name)
	}
	sort.Strings(themeOrder)
}

type App struct {
	config        Config
	running       bool
	visible       bool
	togglePending bool
	lock          sync.Mutex
	themeIdx int
	posTop   bool // true = keyboard at top of screen
	window   *sdl.Window
	mon      MonitorRect
	winH     int32
	margin   int32
}

func NewApp(config Config) *App {
	return &App{
		config:  config,
		visible: true,
	}
}

func (app *App) Run() error {
	cfg := app.config
	ValidateConfig(&cfg)
	layout := LayoutQWERTY
	theme := GetTheme(cfg.Theme.Name)

	// Build dynamic glyph map from button config
	KeyGlyphs = BuildKeyGlyphs(cfg.Gamepad)

	if err := sdl.Init(sdl.INIT_VIDEO | sdl.INIT_GAMECONTROLLER); err != nil {
		return err
	}
	defer sdl.Quit()

	// Get primary monitor
	mon := GetPrimaryMonitor()
	unit := CalcUnitSize(layout, mon.W, cfg)
	pad := int32(cfg.Keys.Padding)
	statusH := max32(20, int32(float64(unit)*0.4))
	width, height := CalcWindowSize(layout, unit, pad, statusH)
	Debugf("Monitor: %dx%d+%d+%d, scale=%d%%, unit=%d, window=%dx%d",
		mon.W, mon.H, mon.X, mon.Y, cfg.Keys.Scale, unit, width, height)

	// Position: center horizontally, top or bottom based on config
	x := mon.X + (mon.W-width)/2
	margin := int32(cfg.Window.BottomMargin)
	var y int32
	if cfg.Window.Position == "top" {
		y = mon.Y + margin
	} else {
		y = mon.Y + mon.H - height - margin
	}

	SaveFocusedWindow()

	window, err := sdl.CreateWindow("gamepad-osk",
		x, y, width, height,
		sdl.WINDOW_SHOWN|sdl.WINDOW_BORDERLESS|sdl.WINDOW_ALWAYS_ON_TOP)
	if err != nil {
		return err
	}
	defer window.Destroy()
	window.SetPosition(x, y)

	// Store for position toggling
	app.window = window
	app.mon = mon
	app.winH = height
	app.margin = margin
	app.posTop = cfg.Window.Position == "top"
	if cfg.Window.Opacity < 1.0 {
		window.SetWindowOpacity(float32(cfg.Window.Opacity))
	}

	renderer, err := sdl.CreateRenderer(window, -1, sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC)
	if err != nil {
		return err
	}
	defer renderer.Destroy()

	// Set X11 hints AFTER renderer init (sdl2-compat may reset properties)
	SetNoFocusHints(window)
	RestoreFocus()

	rend, err := NewRenderer(renderer, theme, unit, pad)
	if err != nil {
		return err
	}
	defer rend.Destroy()

	inj, err := NewInjector()
	if err != nil {
		log.Printf("Error: %v", err)
		return err
	}
	defer inj.Close()

	gamepad := NewGamepadReader(cfg)
	if !gamepad.Open("") {
		log.Printf("Warning: no gamepad found")
		log.Printf("Usage: gamepad-osk --device /dev/input/gamepad0")
	}
	defer gamepad.Close()
	if cfg.Gamepad.Grab {
		gamepad.Grab()
	}

	// Rebuild glyphs after gamepad open (swap_xy may have been auto-detected)
	KeyGlyphs = BuildKeyGlyphs(gamepad.config.Gamepad)

	// Init IPC
	ipc := NewIPCServer(func(cmd string) {
		if cmd == "toggle" {
			app.lock.Lock()
			app.togglePending = true
			app.lock.Unlock()
		}
	})
	if err := ipc.Start(); err != nil {
		log.Printf("Warning: IPC server failed: %v", err)
	}
	defer ipc.Stop()

	kb := NewKeyboardState(layout)

	// Find current theme index for cycling
	for i, name := range themeOrder {
		if name == cfg.Theme.Name {
			app.themeIdx = i
			break
		}
	}
	kb.OnThemeCycle = func() {
		app.themeIdx = (app.themeIdx + 1) % len(themeOrder)
		name := themeOrder[app.themeIdx]
		rend.SetTheme(Themes[name])
		SaveTheme(name)
	}
	kb.OnThemeCycleReverse = func() {
		app.themeIdx = (app.themeIdx - 1 + len(themeOrder)) % len(themeOrder)
		name := themeOrder[app.themeIdx]
		rend.SetTheme(Themes[name])
		SaveTheme(name)
	}

	app.running = true

	for app.running {
		// Handle toggle
		app.lock.Lock()
		if app.togglePending {
			app.togglePending = false
			app.visible = !app.visible
			if app.visible {
				window.Show()
				window.Raise()
			} else {
				window.Hide()
			}
		}
		app.lock.Unlock()

		// Process SDL2 window events
		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch event.(type) {
			case *sdl.QuitEvent:
				app.running = false
			}
		}

		// Process gamepad events (evdev — works regardless of window focus)
		for _, action := range gamepad.ReadEvents() {
			app.handleAction(action, kb, inj, rend)
		}

		// Long-press check
		kb.CheckLongPress(cfg.Gamepad.LongPressMs)

		// Render
		if app.visible {
			rend.Draw(kb)
		}

		sdl.Delay(16) // ~60fps
	}
	return nil
}

func (app *App) handleAction(a Action, kb *KeyboardState, inj *Injector, rend *Renderer) {
	switch a.Type {
	case ActionNavigate:
		kb.Navigate(a.DX, a.DY)
	case ActionPress:
		kb.PressCurrent(inj)
		kb.CancelLongPress()
	case ActionPressStart:
		kb.StartLongPress()
	case ActionBackspace:
		inj.PressKey(KEY_BACKSPACE, nil)
		kb.FlashKey(KEY_BACKSPACE)
	case ActionSpace:
		inj.PressKey(KEY_SPACE, nil)
		kb.FlashKey(KEY_SPACE)
	case ActionEnter:
		inj.PressKey(KEY_ENTER, nil)
		kb.FlashKey(KEY_ENTER)
	case ActionShiftOn:
		kb.ShiftActive = true
	case ActionShiftOff:
		kb.ShiftActive = false
	case ActionCapsToggle:
		kb.CapsActive = !kb.CapsActive
	case ActionClose:
		app.running = false
	case ActionMouseMove:
		inj.MoveMouse(a.DX, a.DY)
	case ActionLeftClick:
		inj.ClickMouse(0x110, true) // BTN_LEFT press
	case ActionLeftClickRelease:
		inj.ClickMouse(0x110, false) // BTN_LEFT release
	case ActionRightClick:
		inj.ClickMouse(0x111, true) // BTN_RIGHT press
	case ActionRightClickRelease:
		inj.ClickMouse(0x111, false) // BTN_RIGHT release
	case ActionPositionToggle:
		app.posTop = !app.posTop
		x, _ := app.window.GetPosition()
		var newY int32
		if app.posTop {
			newY = app.mon.Y + app.margin
			rend.Flash("↑ TOP")
		} else {
			newY = app.mon.Y + app.mon.H - app.winH - app.margin
			rend.Flash("↓ BOTTOM")
		}
		app.window.SetPosition(x, newY)
	}
}
