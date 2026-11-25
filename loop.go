package main

import (
	"log"
	"strconv"
	"sync"
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
	followFocus()        // a program started while shown becomes the one hide returns to
	draw(kb *KeyboardState)
}

// screen is what handling an action asks of the display.
type screen interface {
	MarkDirty()
	Flash(text string)
}

// uiSink is how the input side reaches the display and the window. queuedUI
// (the running app) hands everything to the main thread; directUI (tests)
// does it at once.
type uiSink interface {
	screen
	FlashGlyph(glyph, text string)
	SetTheme(t Theme)
	Later(job func()) // window, SDL and file work: runs on the main thread
}

// directUI runs everything at once on the calling goroutine.
type directUI struct{ app *App }

func (u directUI) MarkDirty()                    { u.app.rend.MarkDirty() }
func (u directUI) Flash(text string)             { u.app.rend.Flash(text) }
func (u directUI) FlashGlyph(glyph, text string) { u.app.rend.FlashGlyph(glyph, text) }
func (u directUI) SetTheme(t Theme)              { u.app.rend.SetTheme(t); u.app.rend.MarkDirty() }
func (u directUI) Later(job func())              { job() }

// queuedUI publishes a new frame for main to draw, and queues window, renderer
// and file work for main to run, in order. It never blocks the input side.
type queuedUI struct{ app *App }

// MarkDirty asks for a new frame; the input loop publishes it at the end of
// the pass, after the state has changed.
func (u queuedUI) MarkDirty() { u.app.frameDirty = true }
func (u queuedUI) Flash(text string) {
	u.Later(func() { u.app.rend.Flash(text) })
}

func (u queuedUI) FlashGlyph(glyph, text string) {
	u.Later(func() { u.app.rend.FlashGlyph(glyph, text) })
}

func (u queuedUI) SetTheme(t Theme) {
	u.Later(func() { u.app.rend.SetTheme(t); u.app.rend.MarkDirty() })
}

func (u queuedUI) Later(job func()) {
	u.app.jobs.push(job)
	u.app.wakeMain()
}

// jobQueue holds work for the main thread. push never blocks for long, so a
// stuck Present cannot hold up the input side.
type jobQueue struct {
	mu   sync.Mutex
	jobs []func()
}

func (q *jobQueue) push(job func()) {
	q.mu.Lock()
	q.jobs = append(q.jobs, job)
	q.mu.Unlock()
}

// run runs the queued jobs in the order they came.
func (q *jobQueue) run() {
	q.mu.Lock()
	jobs := q.jobs
	q.jobs = nil
	q.mu.Unlock()
	for _, job := range jobs {
		job()
	}
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
		ReturnFocus()
	}
}

func (w *sdlWindow) followFocus() {
	if !isWayland {
		FollowNewProgram()
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

// out is where the input side sends display and window work.
func (app *App) out() uiSink {
	if app.ui == nil {
		return directUI{app}
	}
	return app.ui
}

// publish hands main a copy of the keyboard state to draw. Draw then reads
// only its own copy (IsFlashed writes to it), never the input side's state.
func (app *App) publish() {
	c := *app.kb
	if c.AccentPopup != nil {
		popup := *c.AccentPopup
		c.AccentPopup = &popup
	}
	app.frame.Store(&c)
	app.frameSeq.Add(1)
	app.wakeMain()
}

func (app *App) wakeMain() {
	select {
	case app.wake <- struct{}{}:
	default:
	}
}

// stop ends the app (SIGTERM, SIGINT): both loops see it at once.
func (app *App) stop() {
	app.running.Store(false)
	app.wakeMain()
	app.wakeInput()
}

// requestToggle shows or hides the keyboard on the input side's next pass,
// and wakes it for that pass. Safe from any goroutine.
func (app *App) requestToggle() {
	app.lock.Lock()
	app.togglePending = true
	app.lock.Unlock()
	app.wakeInput()
}

func (app *App) wakeInput() {
	select {
	case app.inputWake <- struct{}{}:
	default:
	}
}

// runThreaded is the running app: input on its own goroutine, the window and
// drawing here on the main thread, so a slow Present cannot delay a key.
func (app *App) runThreaded() {
	app.ui = queuedUI{app}
	app.publish()
	done := make(chan struct{})
	go func() {
		defer close(done)
		app.inputLoop()
	}()
	app.mainLoop()
	app.running.Store(false)
	app.wakeInput()
	<-done         // the input side has let go of every held key before Run closes the injector
	app.jobs.run() // config saves queued just before the exit
}

// inputLoop: the pad, key sending and timers.
func (app *App) inputLoop() {
	for app.running.Load() {
		app.applyToggle()
		app.pollPad()
		app.checkGrab()
		app.checkReconnect()
		app.checkTimers()
		app.expireKeyFlash()
		if app.frameDirty {
			app.frameDirty = false
			app.publish()
		}
		app.waitInput()
	}
	app.releaseHeld()
}

// mainLoop: SDL events, window jobs and drawing.
func (app *App) mainLoop() {
	var seen uint64
	for app.running.Load() {
		app.pollWindow()
		app.jobs.run()
		app.expireTextFlash()
		if s := app.frameSeq.Load(); s != seen {
			seen = s
			app.rend.MarkDirty()
		}
		if app.shown {
			before := app.rend.dirtyFrames
			if kb := app.frame.Load(); kb != nil {
				app.win.draw(kb)
			}
			// Vsync in Draw paces the frames still to draw. A Draw that drew
			// nothing (layer surface not configured yet) waits below instead.
			if app.rend.dirtyFrames > 0 && app.rend.dirtyFrames < before {
				continue
			}
		}
		idle := 16 * time.Millisecond // keep the SDL event queue moving
		if !app.shown {
			idle = hiddenIdle
		}
		select {
		case <-app.wake:
		case <-time.After(idle):
		}
	}
}

// loop runs until the app stops: one pass per frame or poll interval, all on
// one goroutine (tests; runThreaded is the running app).
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
		app.out().MarkDirty()
		app.out().Later(func() { app.shown = true; app.win.show() })
		return
	}
	app.releaseHeld()
	// Cancel any pending grab so it can't fire after hide.
	app.grabPending = false
	app.pad.Ungrab()
	app.out().MarkDirty()
	app.out().Later(func() { app.shown = false; app.win.hide() })
}

// releaseHeld clears every modifier and lets go of every key and mouse
// button the app holds: on hide and when the input side stops.
func (app *App) releaseHeld() {
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
}

// pollWindow handles SDL events and a layer surface the compositor closed.
// followEvery is how often, while shown, pollWindow looks for a program
// started since the focus was saved.
const followEvery = 250 * time.Millisecond

func (app *App) pollWindow() {
	if app.win.quitRequested() {
		app.running.Store(false)
	}

	// A game started while the keyboard is up: hide gives the focus back to it,
	// not to the window that had it at show (checked 4 times a second)
	if now := app.clock(); app.shown && now.Sub(app.followedAt) >= followEvery {
		app.followedAt = now
		app.win.followFocus()
	}

	// The compositor closed the keyboard (its output went away): show it
	// again where the compositor chooses, or hide it if that keeps failing
	if app.win.layerClosed() && app.shown {
		app.win.dropLayer()
		now := app.clock()
		if reattachAfterClose(now, app.layerClosedAt) {
			log.Printf("Compositor closed the keyboard (output removed?), showing it again")
			app.win.reattach()
		} else {
			log.Printf("Compositor closed the keyboard again, hiding it: press the toggle combo to show it")
			app.requestToggle()
		}
		app.layerClosedAt = now
	}
}

// pollPad reads the pad (evdev: works regardless of window focus) and lets
// go of held keys when the pad goes away.
func (app *App) pollPad() {
	prevFd := app.pad.Fd()
	for _, action := range app.pad.ReadEvents() {
		app.handleAction(action, app.kb, app.inj, app.out())
	}
	if prevFd >= 0 && app.pad.Fd() < 0 {
		app.releasePadHolds(app.kb, app.inj)
		app.out().Flash("Controller lost")
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
		app.out().Flash("Controller connected")
		if app.visible && app.config.Gamepad.Grab {
			app.pad.Grab()
		}
	}
}

// checkTimers: long press, the Alt+Tab auto-release (3 s idle) and key repeat.
func (app *App) checkTimers() {
	kb, cfg := app.kb, app.config
	if app.visible && kb.CheckLongPress(cfg.Gamepad.LongPressMs) {
		app.out().MarkDirty()
	}
	if kb.CheckAltTabTimeout(app.inj) {
		app.out().MarkDirty()
	}
	if app.repeatAction == ActionNone {
		return
	}
	now := app.clock()
	elapsed := now.Sub(app.repeatStart).Milliseconds()
	if app.repeatInitial {
		if elapsed >= int64(cfg.Keys.RepeatDelayMs) {
			app.handleAction(Action{Type: app.repeatAction}, kb, app.inj, app.out())
			app.repeatLast = now
			app.repeatInitial = false
		}
	} else if now.Sub(app.repeatLast).Milliseconds() >= int64(cfg.Keys.RepeatRateMs) {
		app.handleAction(Action{Type: app.repeatAction}, kb, app.inj, app.out())
		app.repeatLast = now
	}
}

// expireFlash triggers the final redraw when a flash message or key flash ends.
func (app *App) expireFlash() {
	app.expireTextFlash()
	app.expireKeyFlash()
}

// expireTextFlash ends the status bar message (main thread: the renderer).
func (app *App) expireTextFlash() {
	rend := app.rend
	if app.shown && rend.flashText != "" && app.clock().After(rend.flashEnd) {
		rend.MarkDirty()
		rend.flashText = ""
		rend.flashGlyphText = ""
	}
}

// expireKeyFlash ends a key flash (input side: the keyboard state).
func (app *App) expireKeyFlash() {
	kb := app.kb
	if app.visible && kb.FlashCode != 0 && app.clock().After(kb.FlashUntil) {
		app.out().MarkDirty()
		kb.FlashCode = 0
	}
}

// padWaiter is a pad that can say when it has events (GamepadReader).
type padWaiter interface {
	Ready() (ch <-chan struct{}, ok bool)
}

// hiddenIdle is how often main pumps SDL events while hidden: every show,
// job and frame wakes it at once, so this only drains the queue.
const hiddenIdle = 250 * time.Millisecond

// maxIdleWait bounds the input side's wait when no timer is due, so a wake
// that never comes costs at most this.
const maxIdleWait = time.Second

// waitInput sleeps until a pad event, a wake (toggle, stop) or the next
// timer. A pad that cannot say when it has events is polled as before.
func (app *App) waitInput() {
	var ready <-chan struct{}
	w, ok := app.pad.(padWaiter)
	if ok {
		ready, ok = w.Ready()
	}
	if !ok {
		if ms := app.pollMs(); ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
		return
	}
	if app.pad.NeedsPolling() {
		ready = nil // polled every 4 ms: a pad sending 1000 reports a second must not wake each one
	}
	t := time.NewTimer(app.idleWait())
	defer t.Stop()
	select {
	case <-ready:
	case <-app.inputWake:
	case <-t.C:
	}
}

// idleWait is how long the input side may wait for a pad event: until the
// next timer (long press, Alt+Tab release, key flash, grab, reconnect), at
// most maxIdleWait. A stick off center or a key repeat polls as before.
func (app *App) idleWait() time.Duration {
	if app.pad.NeedsPolling() || app.repeatAction != ActionNone {
		return time.Duration(app.pollMs()) * time.Millisecond
	}
	d := maxIdleWait
	until := func(left time.Duration) {
		if left <= 0 {
			left = 16 * time.Millisecond // due but still pending: poll as before, never spin
		}
		d = min(d, left)
	}
	kb, now := app.kb, app.clock() // time.Now when running, as the keyboard's timers
	if app.visible && kb.LongPressActive {
		until(kb.LongPressStart.Add(time.Duration(app.config.Gamepad.LongPressMs) * time.Millisecond).Sub(now))
	}
	if kb.AltTabHeld {
		until(kb.AltTabLastTab.Add(3 * time.Second).Sub(now))
	}
	if app.visible && kb.FlashCode != 0 {
		until(kb.FlashUntil.Sub(now))
	}
	if app.grabPending {
		until(app.grabPendingUntil.Sub(app.clock()))
	}
	if app.pad.Fd() < 0 {
		until(app.reconnectLast.Add(2 * time.Second).Sub(app.clock()))
	}
	return d
}

// sleepMs is how long the loop sleeps after a pass. Vsync in Draw paces
// active rendering. Sleep when hidden, when visible but idle (nothing to
// draw), or when the stick is active (sub-vsync polling for smooth cursor).
func (app *App) sleepMs() int {
	if app.visible && app.rend.dirtyFrames > 0 && !app.pad.NeedsPolling() {
		return 0
	}
	return app.pollMs()
}

// pollMs is how long the input side sleeps between pad polls.
func (app *App) pollMs() int {
	polling := app.pad.NeedsPolling()
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
	app.out().SetTheme(Themes[name])
	app.out().Later(func() { SaveTheme(name) })
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
	app.out().FlashGlyph(glyph, strconv.Itoa(s))
	app.out().Later(func() { SaveSensitivity(s) })
}
