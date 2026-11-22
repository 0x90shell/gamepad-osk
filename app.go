package main //nolint:revive // main package needs no doc comment

import (
	"errors"
	"log"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var themeOrder []string
var isWayland bool

// waylandLayerShell: the Wayland compositor offers wlr-layer-shell (probed
// before SDL starts). layerShellMode: the window is a layer surface.
var waylandLayerShell, layerShellMode bool

func init() {
	for name := range Themes {
		themeOrder = append(themeOrder, name)
	}
	sort.Strings(themeOrder)
}

type App struct {
	config        Config
	running       atomic.Bool // set from the signal handler goroutine
	visible       bool
	daemon        bool
	togglePending bool
	lock          sync.Mutex
	themeIdx      int
	posTop        bool // true = keyboard at top of screen
	window        *Window
	mon           MonitorRect // raw monitor bounds
	posArea       MonitorRect // positioning area (workarea on X11, same as mon on Wayland)
	winH          int32
	winX          int32 // stored for re-apply after show
	winY          int32
	margin        int32

	// Key repeat state
	repeatAction  ActionType // action to repeat (ActionNone = inactive)
	repeatStart   time.Time  // when key was first pressed
	repeatLast    time.Time  // when last repeat fired
	repeatInitial bool       // true = still in initial delay phase

	reconnectLast time.Time // cooldown for gamepad reconnection attempts

	// Deferred grab: when the OSK becomes visible we wait for any held
	// trigger buttons to release before calling EVIOCGRAB, so other readers
	// of the device (evsieve hooks, gamepad mappers) see the release events
	// and don't end up with stuck per-button state. Bounded by a 500 ms
	// fallback so a user who never releases the combo still gets a grab.
	grabPending      bool
	grabPendingUntil time.Time

	layerClosedAt time.Time // last time the compositor closed the layer surface

	// What the loop works on, set by Run (fakes in tests)
	kb   *KeyboardState
	inj  *Injector
	rend *Renderer
	pad  padDevice
	win  windowOps
	now  func() time.Time // nil: time.Now
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

	ValidateConfig(&app.config)
	cfg := app.config
	layout := LayoutQWERTY
	theme := GetTheme(cfg.Theme.Name)

	// Build dynamic glyph map from button config
	KeyGlyphs = BuildKeyGlyphs(cfg.Gamepad)
	PopupHint = BuildPopupHint(cfg.Gamepad)

	// Prefer native Wayland over XWayland when running in a Wayland session,
	// unless the compositor has no layer-shell (GNOME): a Wayland window
	// there cannot place itself, stay on top or refuse focus, but an X11
	// override-redirect window under XWayland can.
	if os.Getenv("WAYLAND_DISPLAY") != "" && os.Getenv("SDL_VIDEODRIVER") == "" {
		layerShell := probeLayerShell()
		driver := chooseVideoDriver(layerShell, os.Getenv("DISPLAY") != "")
		switch {
		case layerShell == 0 && driver == "x11":
			log.Printf("Compositor has no wlr-layer-shell (GNOME?), using XWayland for the overlay")
		case layerShell == 0:
			log.Printf("Warning: compositor has no wlr-layer-shell and XWayland is not available, using a normal window")
		}
		if err := os.Setenv("SDL_VIDEODRIVER", driver); err != nil {
			log.Printf("Warning: cannot set SDL_VIDEODRIVER: %v", err)
		}
		waylandLayerShell = layerShell == 1
	}

	// Allow the screensaver and sleep timers to fire. SDL_Init's video
	// subsystem disables the screensaver by default; on X11 that schedules
	// XResetScreenSaver every ~30s, which resets the idle counter and blocks
	// screen blank, DPMS, and suspend. The OSK has no use for an always-on
	// display, so opt out before init. On Wayland this avoids requesting
	// idle-inhibit.
	SDL3SetHint("SDL_VIDEO_ALLOW_SCREENSAVER", "1")

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

	isWayland = SDL3GetCurrentVideoDriver() == "wayland"
	layerShellMode = isWayland && waylandLayerShell
	var window *Window
	var err error
	switch {
	case layerShellMode:
		// Wayland: create roleless surface, attach layer-shell asynchronously
		setTargetMonitor(mon.X, mon.Y)
		window, err = createWaylandWindow("gamepad-osk", width, height)
		if err != nil {
			return err
		}
	case isWayland:
		// Wayland without layer-shell and without XWayland: a normal window,
		// the best a client can do there. The compositor places it.
		SDL3SetHint("SDL_WINDOW_ACTIVATE_WHEN_SHOWN", "0")
		window, err = SDL3CreateWindowWithProps("gamepad-osk", 0, 0, width, height,
			true, true, true, false)
		if err != nil {
			return err
		}
	default:
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
	if layerShellMode && app.visible {
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
	rend.keyShadow = cfg.Theme.KeyShadow
	rend.roundKeys = cfg.Theme.KeyStyle == "rounded"
	// Set opacity after renderer creation (SDL3 requires renderer to exist first)
	applyOpacity(window, rend, cfg.Window.Opacity)

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
	PopupHint = BuildPopupHint(gamepad.config.Gamepad)

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
		if errors.Is(err, errInstanceRunning) {
			// Lost a start race with another instance: two keyboards would
			// fight over the pad, and this one's exit would end --toggle
			return errors.New("another instance is already running")
		}
		log.Printf("Warning: IPC server failed: %v", err)
	}
	defer ipc.Stop()

	kb := NewKeyboardState(layout)
	app.kb, app.inj, app.rend, app.pad = kb, inj, rend, gamepad
	app.win = &sdlWindow{app: app, window: window, rend: rend, width: width, height: height}

	// Find current theme index for cycling
	for i, name := range themeOrder {
		if name == cfg.Theme.Name {
			app.themeIdx = i
			break
		}
	}
	kb.OnThemeCycle = func() { app.cycleTheme(1) }
	kb.OnThemeCycleReverse = func() { app.cycleTheme(-1) }
	kb.OnSensitivityUp = func() { app.changeSensitivity(2) }
	kb.OnSensitivityDown = func() { app.changeSensitivity(-2) }
	kb.OnNotice = func(msg string) { app.rend.Flash(msg) }

	app.running.Store(true)

	// Show window after everything is initialized (avoids ghost frame)
	if app.visible {
		if !layerShellMode {
			rend.Draw(kb) // render first frame before showing (layer-shell: gated by IsLayerShellReady)
			SDL3ShowWindow(window)
			applyOpacity(window, rend, cfg.Window.Opacity) // re-apply after show
			if !isWayland {
				SDL3SetWindowPosition(window, x, app.winY)
				SetNoFocusHints(window)
			}
		}
		// Layer-shell: first frame renders after configure arrives via SDL event pump
	}

	app.loop()
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
		switch {
		case kb.SkipPress:
			// Accent popup was cancelled while held - type nothing
			kb.SkipPress = false
		case app.repeatAction != ActionNone:
			// Was repeating - just stop, don't fire again
			app.stopRepeat()
		default:
			// Accent popup select, vowel short-press, or modifier - fire on release
			kb.PressCurrent(inj)
		}
		kb.CancelLongPress()
	case ActionPressStart:
		key := kb.CurrentKey()
		if len(key.Accents) > 0 && kb.ShiftActive {
			// Shift(LT)+hold on vowel - accent popup
			kb.StartLongPress()
		} else if !key.IsModifier && key.Label != themeKeyLabel && key.Label != "Paste" &&
			len(key.Combo) == 0 && key.ShiftCode == 0 && key.Label != "Esc" {
			// Repeatable key - fire immediately and start repeat
			kb.PressCurrent(inj)
			app.startRepeat(ActionPressRepeat)
		}
		// Non-repeatable keys (shortcuts, Esc, Theme, Paste, modifiers) fire on release via ActionPress
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
		if kb.AccentPopup != nil {
			// Close button backs out of the accent popup first
			kb.CancelAccentPopup()
			return
		}
		if app.daemon {
			// Daemon mode: close button hides instead of exiting
			app.lock.Lock()
			app.togglePending = true
			app.lock.Unlock()
			return
		}
		app.stopRepeat()
		app.running.Store(false)
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
	app.repeatStart = app.clock()
	app.repeatInitial = true
}

// releasePadHolds lets go of everything a held pad button or trigger was
// keeping down, for a pad that disconnects mid-hold: its releases never
// arrive. Key repeat, a held Shift, mouse buttons, a pending long press and
// the accent popup end; toggled keys (Caps, sticky modifiers, Alt-Tab) stay.
func (app *App) releasePadHolds(kb *KeyboardState, inj *Injector) {
	app.stopRepeat()
	if kb.ShiftHeld {
		kb.ShiftHeld = false
		kb.ShiftActive = false
	}
	kb.CloseAccentPopup()
	kb.SkipPress = false
	if inj != nil {
		inj.ClickMouse(0x110, false) // BTN_LEFT release
		inj.ClickMouse(0x111, false) // BTN_RIGHT release
	}
}

func (app *App) stopRepeat() {
	app.repeatAction = ActionNone
}

// opacityPath is how the window is made see-through.
type opacityPath int

const (
	opacityOpaque  opacityPath = iota // opacity 1: nothing to do
	opacityWindow                     // SDL window opacity: X11, Wayland without layer-shell
	opacitySurface                    // compositor alpha modifier on the layer surface
	opacityLayer                      // layer surface without alpha modifier: renderer draws see-through
)

// chooseOpacityPath picks the opacity method. Layer-shell attaches on each
// show (daemon mode starts hidden), so this is decided after every attach,
// not once at startup. Window opacity has no effect on a layer surface.
func chooseOpacityPath(opacity float64, layerShell, alphaModifier bool) opacityPath {
	switch {
	case opacity >= 1:
		return opacityOpaque
	case !layerShell:
		return opacityWindow
	case alphaModifier:
		return opacitySurface
	default:
		return opacityLayer
	}
}

// applyOpacity sets the configured opacity by the method that works on the
// current window. Call it after each show.
func applyOpacity(window *Window, rend *Renderer, opacity float64) {
	path := chooseOpacityPath(opacity, hasLayerShell(), hasAlphaModifier())
	rend.layerOpacity = 1
	switch path {
	case opacityWindow:
		SDL3SetWindowOpacity(window, float32(opacity))
	case opacitySurface:
		setLayerOpacity(opacity)
		Debugf("Opacity %.2f via compositor alpha modifier", opacity)
	case opacityLayer:
		rend.layerOpacity = opacity
		rend.MarkDirty()
		Debugf("Opacity %.2f drawn by the renderer: compositor has no alpha modifier", opacity)
	case opacityOpaque:
	}
}

// chooseVideoDriver picks SDL's video driver in a Wayland session from the
// layer-shell probe (1 yes, 0 no, -1 failed) and whether XWayland is there.
func chooseVideoDriver(layerShell int, hasXWayland bool) string {
	if layerShell == 0 && hasXWayland {
		return "x11"
	}
	return "wayland"
}
