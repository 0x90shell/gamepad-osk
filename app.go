package main //nolint:revive // main package needs no doc comment

import (
	"log"
	"os"
	"strconv"
	"runtime"
	"sort"
	"sync"
	"time"
)

var themeOrder []string
var isWayland bool

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
	daemon        bool
	togglePending bool
	lock          sync.Mutex
	themeIdx int
	posTop   bool // true = keyboard at top of screen
	window   *Window
	mon      MonitorRect // raw monitor bounds
	posArea  MonitorRect // positioning area (workarea on X11, same as mon on Wayland)
	winH     int32
	winX     int32 // stored for re-apply after show
	winY     int32
	margin   int32

	// Key repeat state
	repeatAction   ActionType // action to repeat (ActionNone = inactive)
	repeatStart    time.Time  // when key was first pressed
	repeatLast     time.Time  // when last repeat fired
	repeatInitial  bool       // true = still in initial delay phase

	reconnectLast time.Time // cooldown for gamepad reconnection attempts

	// Deferred grab: when the OSK becomes visible we wait for any held
	// trigger buttons to release before calling EVIOCGRAB, so other readers
	// of the device (evsieve hooks, gamepad mappers) see the release events
	// and don't end up with stuck per-button state. Bounded by a 500 ms
	// fallback so a user who never releases the combo still gets a grab.
	grabPending      bool
	grabPendingUntil time.Time
}

func NewApp(config Config) *App {
	return &App{
		config:  config,
		visible: true,
	}
}

func (app *App) Run() error {
	// SDL/OpenGL requires all calls from the same OS thread.
	// go-sdl2 did this internally in sdl.Init(); we must do it explicitly.
	runtime.LockOSThread()

	cfg := app.config
	ValidateConfig(&cfg)
	layout := LayoutQWERTY
	theme := GetTheme(cfg.Theme.Name)

	// Build dynamic glyph map from button config
	KeyGlyphs = BuildKeyGlyphs(cfg.Gamepad)

	// Prefer native Wayland over XWayland when running in a Wayland session
	if os.Getenv("WAYLAND_DISPLAY") != "" && os.Getenv("SDL_VIDEODRIVER") == "" {
		if err := os.Setenv("SDL_VIDEODRIVER", "wayland"); err != nil {
			log.Printf("Warning: cannot set SDL_VIDEODRIVER: %v", err)
		}
	}

	if err := SDL3Init(SDL_INIT_VIDEO); err != nil {
		return err
	}
	defer SDL3Quit()

	initX11Detection()

	// Get primary monitor
	mon := GetPrimaryMonitor()
	unit := CalcUnitSize(layout, mon.W, cfg)
	pad := int32(cfg.Keys.Padding) //nolint:gosec // G115: padding fits in int32
	statusH := max32(20, int32(float64(unit)*0.4))
	width, height := CalcWindowSize(layout, unit, pad, statusH)
	Debugf("Monitor: %dx%d+%d+%d, scale=%d%%, unit=%d, window=%dx%d",
		mon.W, mon.H, mon.X, mon.Y, cfg.Keys.Scale, unit, width, height)

	// Use workarea on X11 to avoid panels/taskbars; fall back to monitor bounds.
	// Workarea spans all monitors - intersect with primary to get per-monitor area.
	posArea := mon
	waOk := false
	if wx, wy, ww, wh, ok := GetWorkarea(); ok && ww > 0 && wh > 0 {
		waOk = true
		wa := MonitorRect{X: wx, Y: wy, W: ww, H: wh}
		posArea = intersectRect(mon, wa)
		Debugf("Workarea: %dx%d+%d+%d, monitor: %dx%d+%d+%d, effective: %dx%d+%d+%d",
			ww, wh, wx, wy, mon.W, mon.H, mon.X, mon.Y,
			posArea.W, posArea.H, posArea.X, posArea.Y)
	}
	// Fallback: workarea available but didn't subtract panel for this monitor
	// (multi-monitor XFCE reports combined workarea without per-monitor panel gaps).
	// Scan _NET_WM_STRUT_PARTIAL on dock/panel windows directly.
	if waOk && posArea == mon {
		if top, bottom, left, right := GetStrutInsets(mon); top > 0 || bottom > 0 || left > 0 || right > 0 {
			posArea = MonitorRect{
				X: mon.X + left,
				Y: mon.Y + top,
				W: mon.W - left - right,
				H: mon.H - top - bottom,
			}
			Debugf("Strut fallback: top=%d bottom=%d left=%d right=%d, effective: %dx%d+%d+%d",
				top, bottom, left, right, posArea.W, posArea.H, posArea.X, posArea.Y)
		}
	}

	// Position: center horizontally, top or bottom based on config
	x := posArea.X + (posArea.W-width)/2
	margin := int32(cfg.Window.Margin) //nolint:gosec // G115: margin fits in int32

	SaveFocusedWindow()

	// Set Wayland app_id so compositor rules can target this window
	SDL3SetHint("SDL_APP_ID", "gamepad-osk")
	if err := os.Setenv("SDL_APP_ID", "gamepad-osk"); err != nil {
		log.Printf("Warning: cannot set SDL_APP_ID: %v", err)
	}

	isWayland = os.Getenv("WAYLAND_DISPLAY") != ""
	var window *Window
	var err error
	if isWayland {
		// Wayland: create roleless surface, attach layer-shell asynchronously
		setTargetMonitor(mon.X, mon.Y)
		window, err = createWaylandWindow("gamepad-osk", width, height)
		if err != nil {
			return err
		}
	} else {
		// X11: override-redirect handles z-ordering; SDL_WINDOW_ALWAYS_ON_TOP omitted.
		// SDL3's X11 backend sends _NET_WM_STATE_ABOVE via XSendEvent to root when
		// showing a window with that flag. xfwm4 processes it (even for override-redirect
		// windows) and triggers fullscreen stack re-evaluation, causing xfce4-panel to
		// re-appear. Override-redirect + XMapRaised (called by SDL ShowWindow) provides
		// z-ordering without notifying the WM.
		window, err = SDL3CreateWindow("gamepad-osk",
			width, height,
			SDL_WINDOW_HIDDEN|SDL_WINDOW_BORDERLESS)
		if err != nil {
			return err
		}
		// Position set after app fields are initialized (computeY needs them)
	}
	defer func() {
		cleanupLayerShell()
		SDL3DestroyWindow(window)
	}()

	// Store for position toggling (must be set before computeY)
	app.window = window
	app.mon = mon
	app.posArea = posArea
	app.winH = height
	app.winX = x
	app.margin = margin
	app.posTop = cfg.Window.Position == "top"
	app.winY = app.computeY()

	if !isWayland {
		SDL3SetWindowPosition(window, x, app.winY)
	}

	// Attach layer-shell asynchronously -- configure arrives via SDL event pump.
	// Skip in daemon mode (starts hidden, first toggle-show will attach).
	if isWayland && app.visible {
		attachLayerShellAsync(window, width, height, app.posTop, margin, cfg.Window.PanelAvoid)
	}

	renderer, err := SDL3CreateRenderer(window)
	if err != nil {
		return err
	}
	defer SDL3DestroyRenderer(renderer)
	SDL3SetRenderVSync(renderer, 1)
	SDL3SetRenderDrawBlendMode(renderer, 1) // SDL_BLENDMODE_BLEND
	Debugf("Renderer: %s", SDL3GetRendererName(renderer))

	// Set opacity after renderer creation (SDL3 requires renderer to exist first)
	if cfg.Window.Opacity < 1.0 {
		SDL3SetWindowOpacity(window, float32(cfg.Window.Opacity))
	}

	// Set X11 hints AFTER renderer init (layer-shell handles this on Wayland)
	if !hasLayerShell() {
		SetNoFocusHints(window)
		// No RestoreFocus here: window is hidden, SDL init does not steal X11 focus when a WM is present
	}

	rend, err := NewRenderer(renderer, theme, unit, pad)
	if err != nil {
		return err
	}
	defer rend.Destroy()

	inj, err := NewInjector()
	if err != nil {
		log.Print(colorRed("Warning: key injection disabled - " + err.Error()))
		logPermissionFix()
		log.Printf("The on-screen keyboard will display but cannot send keystrokes.")
	}
	defer func() {
		if inj != nil {
			inj.Close()
		}
	}()

	gamepad := NewGamepadReader(cfg)
	if !gamepad.Open("") {
		log.Printf("Warning: no gamepad found")
		log.Printf("Usage: gamepad-osk --device /dev/input/gamepad0")
	}
	defer gamepad.Close()
	if cfg.Gamepad.Grab && app.visible {
		app.grabPending = true
		app.grabPendingUntil = time.Now().Add(500 * time.Millisecond)
	}

	// Rebuild glyphs after gamepad open (swap_xy may have been auto-detected)
	KeyGlyphs = BuildKeyGlyphs(gamepad.config.Gamepad)

	// Init IPC
	ipc := NewIPCServer(func(cmd string) {
		switch cmd {
		case "toggle":
			app.lock.Lock()
			app.togglePending = true
			app.lock.Unlock()
		case "ping":
			// Instance check - connection success is the answer
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
	kb.OnSensitivityUp = func() {
		cfg.Mouse.Sensitivity = min(50, cfg.Mouse.Sensitivity+2)
		gamepad.SetSensitivity(cfg.Mouse.Sensitivity)
		rend.FlashGlyph("\u27F9", strconv.Itoa(cfg.Mouse.Sensitivity))
		SaveSensitivity(cfg.Mouse.Sensitivity)
	}
	kb.OnSensitivityDown = func() {
		cfg.Mouse.Sensitivity = max(1, cfg.Mouse.Sensitivity-2)
		gamepad.SetSensitivity(cfg.Mouse.Sensitivity)
		rend.FlashGlyph("\u27FE", strconv.Itoa(cfg.Mouse.Sensitivity))
		SaveSensitivity(cfg.Mouse.Sensitivity)
	}

	app.running = true

	opacity := float32(cfg.Window.Opacity)

	// Show window after everything is initialized (avoids ghost frame)
	if app.visible {
		if !isWayland {
			rend.Draw(kb) // render first frame before showing (Wayland: gated by IsLayerShellReady)
			SDL3ShowWindow(window)
			if opacity < 1.0 {
				SDL3SetWindowOpacity(window, opacity) // re-apply after show
			}
			SDL3SetWindowPosition(window, x, app.winY)
			SetNoFocusHints(window)
		}
		// Wayland: first frame renders after configure arrives via SDL event pump
	}

	for app.running {
		// Handle toggle
		app.lock.Lock()
		if app.togglePending {
			app.togglePending = false
			app.visible = !app.visible
			if app.visible {
				if cfg.Gamepad.Grab {
					app.grabPending = true
					app.grabPendingUntil = time.Now().Add(500 * time.Millisecond)
				}
				if !hasLayerShell() {
					SaveFocusedWindow() // capture game window now, used by RestoreFocus on hide
				}
				if isWayland {
					attachLayerShellAsync(window, width, height, app.posTop, margin, cfg.Window.PanelAvoid)
				} else {
					SDL3ShowWindow(window)
				}
				rend.MarkDirty()
				if opacity < 1.0 {
					SDL3SetWindowOpacity(window, opacity) // re-apply after show
				}
				if !isWayland {
					app.winY = app.computeY()
					SDL3SetWindowPosition(window, app.winX, app.winY)
					SetNoFocusHints(window)
					// No RestoreFocus: XMapRaised does not steal focus; calling XSetInputFocus here
					// changes _NET_ACTIVE_WINDOW to prev_focused (terminal), triggering xfce4-panel.
				}
			} else {
				app.stopRepeat()
				kb.CapsActive = false
				kb.ShiftActive = false
				kb.ShiftHeld = false
				kb.CtrlActive = false
				kb.AltActive = false
				kb.MetaActive = false
				kb.ReleaseAltTab(inj)
				if inj != nil {
					inj.ReleaseKey(KEY_LEFTSHIFT)
					inj.ReleaseKey(KEY_LEFTCTRL)
					inj.ReleaseKey(KEY_LEFTALT)
					inj.ReleaseKey(KEY_LEFTMETA)
					inj.ClickMouse(0x110, false) // BTN_LEFT release
					inj.ClickMouse(0x111, false) // BTN_RIGHT release
				}
				// Cancel any pending grab so it can't fire after hide.
				app.grabPending = false
				gamepad.Ungrab()
				if isWayland {
					destroyLayerSurface()
				} else {
					SDL3HideWindow(window)
				}
				if !isWayland {
					if IsSavedWindowFullscreen() {
						WarpPointerIfOutside()
					}
					RestoreFocus()
				}
			}
		}
		app.lock.Unlock()

		// Process SDL3 window events
		for evType, ok := SDL3PollEvent(); ok; evType, ok = SDL3PollEvent() {
			if evType == SDL_EVENT_QUIT {
				app.running = false
			}
		}

		// Process gamepad events (evdev - works regardless of window focus)
		prevFd := gamepad.Fd()
		for _, action := range gamepad.ReadEvents() {
			app.handleAction(action, kb, inj, rend)
		}
		if prevFd >= 0 && gamepad.Fd() < 0 {
			rend.Flash("Controller lost")
		}

		// Deferred grab: defer EVIOCGRAB until trigger combo is released so
		// other readers (evsieve hooks, gamepad mappers) see the release events
		// and don't end up with stuck per-button state. Mirrors evsieve's
		// own grab=auto idiom. Falls back to grabbing after grabPendingUntil
		// so a user holding the combo indefinitely still gets a grab.
		if app.grabPending {
			if !gamepad.AnyButtonHeld() || time.Now().After(app.grabPendingUntil) {
				app.grabPending = false
				gamepad.Grab()
			}
		}

		// Gamepad reconnection (2s cooldown between attempts)
		if gamepad.Fd() < 0 {
			now := time.Now()
			if now.Sub(app.reconnectLast) >= 2*time.Second {
				app.reconnectLast = now
				if gamepad.Reconnect() {
					rend.Flash("Controller connected")
					if app.visible && cfg.Gamepad.Grab {
						gamepad.Grab()
					}
				}
			}
		}

		// Long-press check
		if kb.CheckLongPress(cfg.Gamepad.LongPressMs) {
			rend.MarkDirty()
		}

		// Alt+Tab timeout (auto-release after 3s idle)
		if kb.CheckAltTabTimeout(inj) {
			rend.MarkDirty()
		}

		// Key repeat check
		if app.repeatAction != ActionNone {
			now := time.Now()
			elapsed := now.Sub(app.repeatStart).Milliseconds()
			if app.repeatInitial {
				if elapsed >= int64(cfg.Keys.RepeatDelayMs) {
					app.handleAction(Action{Type: app.repeatAction}, kb, inj, rend)
					app.repeatLast = now
					app.repeatInitial = false
				}
			} else if now.Sub(app.repeatLast).Milliseconds() >= int64(cfg.Keys.RepeatRateMs) {
				app.handleAction(Action{Type: app.repeatAction}, kb, inj, rend)
				app.repeatLast = now
			}
		}

		// Check flash/key flash expiry to trigger final redraw
		if app.visible {
			if rend.flashText != "" && time.Now().After(rend.flashEnd) {
				rend.MarkDirty()
				rend.flashText = ""
				rend.flashGlyphText = ""
			}
			if kb.FlashCode != 0 && time.Now().After(kb.FlashUntil) {
				rend.MarkDirty()
				kb.FlashCode = 0
			}
		}

		// Render
		if app.visible {
			rend.Draw(kb)
		}

		// Frame pacing: vsync in rend.Draw handles active rendering.
		// Sleep when hidden, when visible but idle (nothing to draw), or
		// when stick is active (sub-vsync polling for smooth cursor).
		if !app.visible || rend.dirtyFrames <= 0 || gamepad.NeedsPolling() {
			pollMs := 16 // hidden idle: ~60Hz for IPC + gamepad checks
			if gamepad.NeedsPolling() {
				pollMs = 4 // mouse/nav active: ~250Hz for smooth cursor
			}
			if app.repeatAction != ActionNone {
				var nextMs int64
				if app.repeatInitial {
					nextMs = int64(cfg.Keys.RepeatDelayMs) - time.Since(app.repeatStart).Milliseconds()
				} else {
					nextMs = int64(cfg.Keys.RepeatRateMs) - time.Since(app.repeatLast).Milliseconds()
				}
				if nextMs > 0 && int(nextMs) < pollMs {
					pollMs = int(nextMs)
				}
			}
			if pollMs > 0 {
				time.Sleep(time.Duration(pollMs) * time.Millisecond)
			}
		}
	}
	return nil
}

func (app *App) handleAction(a Action, kb *KeyboardState, inj *Injector, rend *Renderer) {
	// Toggle combo works even when hidden (it's the show/hide mechanism)
	if a.Type == ActionToggle {
		app.lock.Lock()
		app.togglePending = true
		app.lock.Unlock()
		return
	}

	// Block all input when keyboard is hidden
	if !app.visible {
		return
	}

	rend.MarkDirty()

	switch a.Type { //nolint:exhaustive // ActionNone is filtered by caller

	case ActionNavigate:
		kb.ReleaseAltTab(inj)
		kb.Navigate(a.DX, a.DY)
		app.stopRepeat()
	case ActionPress:
		// A button released
		if app.repeatAction != ActionNone {
			// Was repeating - just stop, don't fire again
			app.stopRepeat()
		} else {
			// Accent popup select, vowel short-press, or modifier - fire on release
			kb.PressCurrent(inj)
		}
		kb.CancelLongPress()
	case ActionPressStart:
		key := kb.CurrentKey()
		if len(key.Accents) > 0 && kb.ShiftActive {
			// Shift(LT)+hold on vowel - accent popup
			kb.StartLongPress()
		} else if !key.IsModifier && key.Label != "Cfg" && key.Label != "Paste" &&
			len(key.Combo) == 0 && key.ShiftCode == 0 && key.Label != "Esc" {
			// Repeatable key - fire immediately and start repeat
			kb.PressCurrent(inj)
			app.startRepeat(ActionPressRepeat)
		}
		// Non-repeatable keys (shortcuts, Esc, Cfg, Paste, modifiers) fire on release via ActionPress
	case ActionPressRepeat:
		kb.PressCurrent(inj)
	case ActionBackspace:
		if inj != nil {
			inj.PressKey(KEY_BACKSPACE, nil)
		}
		kb.FlashKey(KEY_BACKSPACE)
		app.startRepeat(ActionBackspace)
	case ActionBackspaceRelease:
		app.stopRepeat()
	case ActionSpace:
		if inj != nil {
			inj.PressKey(KEY_SPACE, nil)
		}
		kb.FlashKey(KEY_SPACE)
		app.startRepeat(ActionSpace)
	case ActionSpaceRelease:
		app.stopRepeat()
	case ActionEnter:
		if inj != nil {
			inj.PressKey(KEY_ENTER, nil)
		}
		kb.FlashKey(KEY_ENTER)
		app.startRepeat(ActionEnter)
	case ActionEnterRelease:
		app.stopRepeat()
	case ActionShiftOn:
		kb.ShiftActive = true
		kb.ShiftHeld = true
	case ActionShiftOff:
		kb.ShiftActive = false
		kb.ShiftHeld = false
	case ActionCapsToggle:
		kb.CapsActive = !kb.CapsActive
	case ActionMouseMove:
		if inj != nil {
			inj.MoveMouse(a.DX, a.DY)
		}
	case ActionLeftClick:
		if inj != nil {
			inj.ClickMouse(0x110, true) // BTN_LEFT press
		}
	case ActionLeftClickRelease:
		if inj != nil {
			inj.ClickMouse(0x110, false) // BTN_LEFT release
		}
	case ActionRightClick:
		if inj != nil {
			inj.ClickMouse(0x111, true) // BTN_RIGHT press
		}
	case ActionRightClickRelease:
		if inj != nil {
			inj.ClickMouse(0x111, false) // BTN_RIGHT release
		}
	case ActionPositionToggle:
		app.posTop = !app.posTop
		if app.posTop {
			rend.Flash("↑ TOP")
		} else {
			rend.Flash("↓ BOTTOM")
		}
		if hasLayerShell() {
			repositionLayerSurface(app.posTop, app.margin, app.window)
		} else {
			app.winY = app.computeY()
			SDL3SetWindowPosition(app.window, app.winX, app.winY)
		}
		SavePosition(app.posTop)
	case ActionClose:
		if app.daemon {
			// Daemon mode: close button hides instead of exiting
			app.lock.Lock()
			app.togglePending = true
			app.lock.Unlock()
			return
		}
		app.stopRepeat()
		app.running = false
		return
	}
}

// computeY returns the window Y position based on current fullscreen state.
// When a fullscreen window is active, uses raw monitor bounds (screen edge).
// Otherwise uses workarea bounds (respects panels/taskbars).
func (app *App) computeY() int32 {
	area := app.posArea
	if IsFullscreenActive() {
		area = app.mon
	}
	if app.posTop {
		return area.Y + app.margin
	}
	return area.Y + area.H - app.winH - app.margin
}

func (app *App) startRepeat(action ActionType) {
	// Don't restart repeat if already repeating this action (called from repeat loop)
	if app.repeatAction == action {
		return
	}
	app.repeatAction = action
	app.repeatStart = time.Now()
	app.repeatInitial = true
}

func (app *App) stopRepeat() {
	app.repeatAction = ActionNone
}
