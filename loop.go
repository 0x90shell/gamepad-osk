package main

import (
	"log"
	"strconv"
	"time"
)

// padDevice is the part of GamepadReader the main loop uses, so tests can
// drive the loop with a fake pad.
type padDevice interface {
	ReadEvents() []Action
	Fd() int
	AnyButtonHeld() bool
	Grab()
	Ungrab()
	Reconnect() bool
	NeedsPolling() bool
	SetSensitivity(value int)
}

// windowOps is what the main loop asks of the window: SDL, layer-shell and
// X11 calls, all on the main thread. Tests use a fake.
type windowOps interface {
	show()
	hide()
	quitRequested() bool // drains the SDL event queue
	layerClosed() bool   // the compositor closed the layer surface since the last call
	dropLayer()          // destroy a closed layer surface and forget its output
	reattach()           // attach the layer surface again after a close
	draw(kb *KeyboardState)
}

// sdlWindow is the real window: an SDL window, a layer surface on Wayland
// compositors with layer-shell, override-redirect on X11.
type sdlWindow struct {
	app           *App
	window        *Window
	rend          *Renderer
	width, height int32
}

func (w *sdlWindow) show() {
	app, cfg := w.app, w.app.config
	if !hasLayerShell() {
		SaveFocusedWindow() // capture game window now, used by RestoreFocus on hide
	}
	if layerShellMode {
		attachLayerShellAsync(w.window, w.width, w.height, app.posTop, app.margin, cfg.Window.PanelAvoid)
	} else {
		SDL3ShowWindow(w.window)
	}
	w.rend.MarkDirty()
	applyOpacity(w.window, w.rend, cfg.Window.Opacity) // re-apply after show
	if !isWayland {
		app.winY = app.computeY()
		SDL3SetWindowPosition(w.window, app.winX, app.winY)
		SetNoFocusHints(w.window)
		// No RestoreFocus: XMapRaised does not steal focus; calling XSetInputFocus here
		// changes _NET_ACTIVE_WINDOW to prev_focused (terminal), triggering xfce4-panel.
	}
}

func (w *sdlWindow) hide() {
	if layerShellMode {
		destroyLayerSurface()
	} else {
		SDL3HideWindow(w.window)
	}
	if !isWayland {
		if IsSavedWindowFullscreen() {
			WarpPointerIfOutside()
		}
		RestoreFocus()
	}
}

func (w *sdlWindow) quitRequested() bool {
	quit := false
	for evType, ok := SDL3PollEvent(); ok; evType, ok = SDL3PollEvent() {
		if evType == SDL_EVENT_QUIT {
			quit = true
		}
	}
	return quit
}

func (w *sdlWindow) layerClosed() bool {
	return layerShellMode && takeLayerSurfaceClosed()
}

func (w *sdlWindow) dropLayer() {
	destroyLayerSurface()
	forgetTargetOutput()
}

func (w *sdlWindow) reattach() {
	app := w.app
	attachLayerShellAsync(w.window, w.width, w.height, app.posTop, app.margin, app.config.Window.PanelAvoid)
	w.rend.MarkDirty()
	applyOpacity(w.window, w.rend, app.config.Window.Opacity)
}

func (w *sdlWindow) draw(kb *KeyboardState) {
	w.rend.Draw(kb)
}

// clock is time.Now unless a test set app.now.
func (app *App) clock() time.Time {
	if app.now != nil {
		return app.now()
	}
	return time.Now()
}

// loop runs until the app stops: one pass per frame or poll interval.
func (app *App) loop() {
	for app.running.Load() {
		app.applyToggle()
		app.pollWindow()
		app.pollPad()
		app.checkGrab()
		app.checkReconnect()
		app.checkTimers()
		app.expireFlash()
		if app.visible {
			app.win.draw(app.kb)
		}
		if ms := app.sleepMs(); ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
	}
}

// applyToggle shows or hides the keyboard when a toggle is pending (IPC,
// the toggle combo, Close in daemon mode).
func (app *App) applyToggle() {
	app.lock.Lock()
	defer app.lock.Unlock()
	if !app.togglePending {
		return
	}
	app.togglePending = false
	app.visible = !app.visible
	if app.visible {
		if app.config.Gamepad.Grab {
			app.grabPending = true
			app.grabPendingUntil = app.clock().Add(500 * time.Millisecond)
		}
		app.win.show()
		return
	}
	kb, inj := app.kb, app.inj
	app.stopRepeat()
	kb.CapsActive = false
	kb.ShiftActive = false
	kb.ShiftHeld = false
	kb.CtrlActive = false
	kb.AltActive = false
	kb.MetaActive = false
	kb.CloseAccentPopup() // also ends a pending long press
	kb.SkipPress = false
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
	app.pad.Ungrab()
	app.win.hide()
}

// pollWindow handles SDL events and a layer surface the compositor closed.
func (app *App) pollWindow() {
	if app.win.quitRequested() {
		app.running.Store(false)
	}

	// The compositor closed the keyboard (its output went away): show it
	// again where the compositor chooses, or hide it if that keeps failing
	if app.win.layerClosed() && app.visible {
		app.win.dropLayer()
		now := app.clock()
		if reattachAfterClose(now, app.layerClosedAt) {
			log.Printf("Compositor closed the keyboard (output removed?), showing it again")
			app.win.reattach()
		} else {
			log.Printf("Compositor closed the keyboard again, hiding it: press the toggle combo to show it")
			app.lock.Lock()
			app.togglePending = true
			app.lock.Unlock()
		}
		app.layerClosedAt = now
	}
}

// pollPad reads the pad (evdev: works regardless of window focus) and lets
// go of held keys when the pad goes away.
func (app *App) pollPad() {
	prevFd := app.pad.Fd()
	for _, action := range app.pad.ReadEvents() {
		app.handleAction(action, app.kb, app.inj, app.rend)
	}
	if prevFd >= 0 && app.pad.Fd() < 0 {
		app.releasePadHolds(app.kb, app.inj)
		app.rend.Flash("Controller lost")
	}
}

// checkGrab: defer EVIOCGRAB until trigger combo is released so other
// readers (evsieve hooks, gamepad mappers) see the release events and don't
// end up with stuck per-button state. Mirrors evsieve's own grab=auto idiom.
// Falls back to grabbing after grabPendingUntil so a user holding the combo
// indefinitely still gets a grab.
func (app *App) checkGrab() {
	if app.grabPending {
		if !app.pad.AnyButtonHeld() || app.clock().After(app.grabPendingUntil) {
			app.grabPending = false
			app.pad.Grab()
		}
	}
}

// checkReconnect tries to reopen a lost pad, at most every 2 s.
func (app *App) checkReconnect() {
	if app.pad.Fd() >= 0 {
		return
	}
	now := app.clock()
	if now.Sub(app.reconnectLast) < 2*time.Second {
		return
	}
	app.reconnectLast = now
	if app.pad.Reconnect() {
		app.rend.Flash("Controller connected")
		if app.visible && app.config.Gamepad.Grab {
			app.pad.Grab()
		}
	}
}

// checkTimers: long press, the Alt+Tab auto-release (3 s idle) and key repeat.
func (app *App) checkTimers() {
	kb, cfg := app.kb, app.config
	if app.visible && kb.CheckLongPress(cfg.Gamepad.LongPressMs) {
		app.rend.MarkDirty()
	}
	if kb.CheckAltTabTimeout(app.inj) {
		app.rend.MarkDirty()
	}
	if app.repeatAction == ActionNone {
		return
	}
	now := app.clock()
	elapsed := now.Sub(app.repeatStart).Milliseconds()
	if app.repeatInitial {
		if elapsed >= int64(cfg.Keys.RepeatDelayMs) {
			app.handleAction(Action{Type: app.repeatAction}, kb, app.inj, app.rend)
			app.repeatLast = now
			app.repeatInitial = false
		}
	} else if now.Sub(app.repeatLast).Milliseconds() >= int64(cfg.Keys.RepeatRateMs) {
		app.handleAction(Action{Type: app.repeatAction}, kb, app.inj, app.rend)
		app.repeatLast = now
	}
}

// expireFlash triggers the final redraw when a flash message or key flash ends.
func (app *App) expireFlash() {
	if !app.visible {
		return
	}
	rend, kb, now := app.rend, app.kb, app.clock()
	if rend.flashText != "" && now.After(rend.flashEnd) {
		rend.MarkDirty()
		rend.flashText = ""
		rend.flashGlyphText = ""
	}
	if kb.FlashCode != 0 && now.After(kb.FlashUntil) {
		rend.MarkDirty()
		kb.FlashCode = 0
	}
}

// sleepMs is how long the loop sleeps after a pass. Vsync in Draw paces
// active rendering. Sleep when hidden, when visible but idle (nothing to
// draw), or when the stick is active (sub-vsync polling for smooth cursor).
func (app *App) sleepMs() int {
	polling := app.pad.NeedsPolling()
	if app.visible && app.rend.dirtyFrames > 0 && !polling {
		return 0
	}
	pollMs := 16 // hidden idle: ~60Hz for IPC + gamepad checks
	if polling {
		pollMs = 4 // mouse/nav active: ~250Hz for smooth cursor
	}
	if app.repeatAction != ActionNone {
		var nextMs int64
		cfg, now := app.config, app.clock()
		if app.repeatInitial {
			nextMs = int64(cfg.Keys.RepeatDelayMs) - now.Sub(app.repeatStart).Milliseconds()
		} else {
			nextMs = int64(cfg.Keys.RepeatRateMs) - now.Sub(app.repeatLast).Milliseconds()
		}
		if nextMs > 0 && int(nextMs) < pollMs {
			pollMs = int(nextMs)
		}
	}
	return pollMs
}

// cycleTheme moves to the next (step 1) or previous (step -1) theme and saves it.
func (app *App) cycleTheme(step int) {
	app.themeIdx = (app.themeIdx + step + len(themeOrder)) % len(themeOrder)
	name := themeOrder[app.themeIdx]
	app.rend.SetTheme(Themes[name])
	SaveTheme(name)
}

// changeSensitivity steps the mouse sensitivity (1 to 50), shows and saves it.
func (app *App) changeSensitivity(step int) {
	s := min(50, max(1, app.config.Mouse.Sensitivity+step))
	app.config.Mouse.Sensitivity = s
	app.pad.SetSensitivity(s)
	glyph := "⟹"
	if step < 0 {
		glyph = "⟾"
	}
	app.rend.FlashGlyph(glyph, strconv.Itoa(s))
	SaveSensitivity(s)
}
