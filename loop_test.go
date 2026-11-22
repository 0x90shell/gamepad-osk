package main

import (
	"encoding/binary"
	"io"
	"os"
	"testing"
	"time"
)

// fakePad stands in for GamepadReader in loop tests.
type fakePad struct {
	fd          int
	held        bool
	polling     bool
	reconnectOK bool
	grabs       int
	ungrabs     int
	reconnects  int
	sensitivity int
	events      []Action
	loseOnRead  bool // the next ReadEvents finds the pad gone (fd -1)
}

func (p *fakePad) ReadEvents() []Action {
	ev := p.events
	p.events = nil
	if p.loseOnRead {
		p.fd, p.loseOnRead = -1, false
	}
	return ev
}
func (p *fakePad) Fd() int             { return p.fd }
func (p *fakePad) AnyButtonHeld() bool { return p.held }
func (p *fakePad) Grab()               { p.grabs++ }
func (p *fakePad) Ungrab()             { p.ungrabs++ }
func (p *fakePad) NeedsPolling() bool  { return p.polling }
func (p *fakePad) SetSensitivity(v int) {
	p.sensitivity = v
}
func (p *fakePad) Reconnect() bool {
	p.reconnects++
	if p.reconnectOK {
		p.fd = 3
	}
	return p.reconnectOK
}

// fakeWindow records the window calls the loop makes.
type fakeWindow struct {
	shows, hides, draws, reattaches, drops int
	quit, closed                           bool
}

func (w *fakeWindow) show()                 { w.shows++ }
func (w *fakeWindow) hide()                 { w.hides++ }
func (w *fakeWindow) quitRequested() bool   { return w.quit }
func (w *fakeWindow) layerClosed() bool     { c := w.closed; w.closed = false; return c }
func (w *fakeWindow) dropLayer()            { w.drops++ }
func (w *fakeWindow) reattach()             { w.reattaches++ }
func (w *fakeWindow) draw(_ *KeyboardState) { w.draws++ }

// keyEvent is one EV_KEY event the injector wrote.
type keyEvent struct {
	code  uint16
	value int32
}

// testLoop is an App wired to fakes: a pipe injector, a fake pad and window,
// and a clock the test moves.
type testLoop struct {
	app *App
	pad *fakePad
	win *fakeWindow
	t   time.Time
	pr  *os.File
	pw  *os.File
	tst *testing.T
}

func newTestLoop(t *testing.T) *testLoop {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pr.Close(); _ = pw.Close() })
	cfg := DefaultConfig()
	ValidateConfig(&cfg)
	l := &testLoop{pad: &fakePad{fd: 3}, win: &fakeWindow{}, t: time.Unix(1000, 0), pr: pr, pw: pw, tst: t}
	l.app = &App{config: cfg, visible: true,
		kb: NewKeyboardState(LayoutQWERTY), inj: &Injector{fd: pw}, rend: &Renderer{},
		pad: l.pad, win: l.win}
	l.app.now = func() time.Time { return l.t }
	l.app.running.Store(true)
	return l
}

func (l *testLoop) advance(ms int) { l.t = l.t.Add(time.Duration(ms) * time.Millisecond) }

// keys returns the EV_KEY events written since the last call.
func (l *testLoop) keys() []keyEvent {
	l.tst.Helper()
	_ = l.pw.Close()
	data, err := io.ReadAll(l.pr)
	if err != nil {
		l.tst.Fatal(err)
	}
	var out []keyEvent
	for i := 0; i+24 <= len(data); i += 24 {
		if binary.LittleEndian.Uint16(data[i+16:]) == 1 { // EV_KEY
			out = append(out, keyEvent{binary.LittleEndian.Uint16(data[i+18:]),
				int32(binary.LittleEndian.Uint32(data[i+20:]))}) //nolint:gosec // G115: event value
		}
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		l.tst.Fatal(err)
	}
	l.pr, l.pw = pr, pw
	l.app.inj.fd = pw
	l.tst.Cleanup(func() { _ = pr.Close(); _ = pw.Close() })
	return out
}

// backspaces counts the Backspace presses in evs.
func backspaces(evs []keyEvent) int {
	n := 0
	for _, e := range evs {
		if e.code == KEY_BACKSPACE && e.value == 1 {
			n++
		}
	}
	return n
}

// L01: the sleep after a pass, from today's numbers.
func TestSleepMs(t *testing.T) {
	cases := []struct {
		name      string
		visible   bool
		dirty     int
		polling   bool
		repeat    bool
		initial   bool
		sinceMs   int // since repeatStart (initial) or repeatLast
		want      int
		delayRate [2]int
	}{
		{name: "hidden", visible: false, want: 16},
		{name: "shown, idle", visible: true, dirty: 0, want: 16},
		{name: "shown, frame to draw", visible: true, dirty: 3, want: 0},
		{name: "hidden, stick moving", visible: false, polling: true, want: 4},
		{name: "shown, frame to draw, stick moving", visible: true, dirty: 3, polling: true, want: 4},
		{name: "repeat due in 3 ms", visible: true, repeat: true, sinceMs: 47, want: 3, delayRate: [2]int{400, 50}},
		{name: "repeat in initial delay", visible: true, repeat: true, initial: true, sinceMs: 390, want: 10,
			delayRate: [2]int{400, 50}},
		{name: "repeat overdue", visible: true, repeat: true, sinceMs: 80, want: 16, delayRate: [2]int{400, 50}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := newTestLoop(t)
			app := l.app
			app.visible = c.visible
			app.rend.dirtyFrames = c.dirty
			l.pad.polling = c.polling
			if c.repeat {
				app.config.Keys.RepeatDelayMs, app.config.Keys.RepeatRateMs = c.delayRate[0], c.delayRate[1]
				app.repeatAction = ActionBackspace
				app.repeatInitial = c.initial
				app.repeatStart = l.t.Add(-time.Duration(c.sinceMs) * time.Millisecond)
				app.repeatLast = app.repeatStart
			}
			if got := app.sleepMs(); got != c.want {
				t.Errorf("sleepMs = %d, want %d", got, c.want)
			}
		})
	}
}

// L02: show arms the deferred grab; hide lets go of everything.
func TestApplyToggle(t *testing.T) {
	l := newTestLoop(t)
	app := l.app
	app.visible = false
	app.config.Gamepad.Grab = true

	app.togglePending = true
	app.applyToggle()
	if !app.visible || l.win.shows != 1 {
		t.Fatalf("show: visible=%v shows=%d", app.visible, l.win.shows)
	}
	if !app.grabPending || !app.grabPendingUntil.Equal(l.t.Add(500*time.Millisecond)) {
		t.Errorf("show: grab pending %v until %v, want 500 ms ahead", app.grabPending, app.grabPendingUntil)
	}

	app.applyToggle() // nothing pending: no change
	if !app.visible || l.win.shows != 1 || l.win.hides != 0 {
		t.Fatal("applyToggle with nothing pending changed the window")
	}

	kb := app.kb
	kb.CapsActive, kb.ShiftActive, kb.ShiftHeld = true, true, true
	kb.CtrlActive, kb.AltActive, kb.MetaActive = true, true, true
	app.startRepeat(ActionBackspace)
	_ = l.keys()
	app.togglePending = true
	app.applyToggle()
	if app.visible || l.win.hides != 1 || l.pad.ungrabs != 1 {
		t.Fatalf("hide: visible=%v hides=%d ungrabs=%d", app.visible, l.win.hides, l.pad.ungrabs)
	}
	if kb.CapsActive || kb.ShiftActive || kb.ShiftHeld || kb.CtrlActive || kb.AltActive || kb.MetaActive {
		t.Error("hide left a modifier on")
	}
	if app.repeatAction != ActionNone || app.grabPending {
		t.Errorf("hide: repeat %v, grab pending %v", app.repeatAction, app.grabPending)
	}
	released := map[uint16]bool{}
	for _, e := range l.keys() {
		if e.value == 0 {
			released[e.code] = true
		}
	}
	for _, code := range []uint16{KEY_LEFTSHIFT, KEY_LEFTCTRL, KEY_LEFTALT, KEY_LEFTMETA, 0x110, 0x111} {
		if !released[code] {
			t.Errorf("hide did not release key 0x%x", code)
		}
	}

	// A held Alt+Tab ends on hide too
	app.togglePending = true
	app.applyToggle() // show
	kb.AltTabHeld = true
	app.togglePending = true
	app.applyToggle() // hide
	if kb.AltTabHeld {
		t.Error("hide left Alt+Tab held")
	}
}

// L03: first repeat after repeat_delay_ms, then every repeat_rate_ms, none after release.
func TestKeyRepeatTiming(t *testing.T) {
	l := newTestLoop(t)
	app := l.app
	app.config.Keys.RepeatDelayMs, app.config.Keys.RepeatRateMs = 400, 50

	app.handleAction(Action{Type: ActionBackspace}, app.kb, app.inj, app.rend)
	if n := backspaces(l.keys()); n != 1 {
		t.Fatalf("press sent %d Backspace, want 1", n)
	}
	l.advance(399)
	app.checkTimers()
	if n := backspaces(l.keys()); n != 0 {
		t.Fatalf("repeat after 399 ms (delay 400): %d", n)
	}
	l.advance(1)
	app.checkTimers()
	if n := backspaces(l.keys()); n != 1 {
		t.Fatalf("at 400 ms: %d repeats, want 1", n)
	}
	l.advance(49)
	app.checkTimers()
	if n := backspaces(l.keys()); n != 0 {
		t.Fatalf("49 ms after the first repeat (rate 50): %d", n)
	}
	l.advance(1)
	app.checkTimers()
	if n := backspaces(l.keys()); n != 1 {
		t.Fatalf("50 ms after the first repeat: %d, want 1", n)
	}
	app.handleAction(Action{Type: ActionBackspaceRelease}, app.kb, app.inj, app.rend)
	l.advance(1000)
	app.checkTimers()
	if n := backspaces(l.keys()); n != 0 {
		t.Errorf("after release: %d repeats", n)
	}
}

// L04: the deferred grab waits for released buttons or the deadline, never after hide.
func TestCheckGrab(t *testing.T) {
	l := newTestLoop(t)
	app := l.app
	app.grabPending, app.grabPendingUntil = true, l.t.Add(500*time.Millisecond)

	l.pad.held = true
	app.checkGrab()
	if l.pad.grabs != 0 {
		t.Fatal("grabbed while a button is held, before the deadline")
	}
	l.pad.held = false
	app.checkGrab()
	if l.pad.grabs != 1 || app.grabPending {
		t.Fatalf("after release: grabs=%d pending=%v", l.pad.grabs, app.grabPending)
	}
	app.checkGrab()
	if l.pad.grabs != 1 {
		t.Fatal("grabbed twice")
	}

	app.grabPending, app.grabPendingUntil = true, l.t.Add(500*time.Millisecond)
	l.pad.held = true
	l.advance(501)
	app.checkGrab()
	if l.pad.grabs != 2 {
		t.Fatal("held past the deadline: no grab")
	}

	app.grabPending, app.grabPendingUntil = true, l.t.Add(500*time.Millisecond)
	app.togglePending = true
	app.applyToggle() // hide
	l.pad.held = false
	app.checkGrab()
	if l.pad.grabs != 2 {
		t.Error("grabbed after hide")
	}
}

// L05: reconnect at most every 2 s; on success flash, and grab only when shown with grab on.
func TestCheckReconnect(t *testing.T) {
	l := newTestLoop(t)
	app := l.app
	app.config.Gamepad.Grab = true
	l.pad.fd = -1

	app.checkReconnect()
	l.advance(1999)
	app.checkReconnect()
	if l.pad.reconnects != 1 {
		t.Fatalf("%d attempts in 1999 ms, want 1", l.pad.reconnects)
	}
	l.pad.reconnectOK = true
	l.advance(1)
	app.checkReconnect()
	if l.pad.reconnects != 2 || l.pad.grabs != 1 || app.rend.flashText != "Controller connected" {
		t.Fatalf("success when shown: attempts=%d grabs=%d flash=%q", l.pad.reconnects, l.pad.grabs,
			app.rend.flashText)
	}
	l.advance(5000)
	app.checkReconnect()
	if l.pad.reconnects != 2 {
		t.Error("tried to reconnect a connected pad")
	}

	l.pad.fd, app.visible = -1, false
	l.advance(2000)
	app.checkReconnect()
	if l.pad.grabs != 1 {
		t.Error("grabbed a reconnected pad while hidden")
	}

	app.config.Gamepad.Grab, app.visible, l.pad.fd = false, true, -1
	l.advance(2000)
	app.checkReconnect()
	if l.pad.grabs != 1 {
		t.Error("grabbed a reconnected pad with grab = false")
	}
}

// L06: an ended flash marks the frame dirty once and clears.
func TestExpireFlash(t *testing.T) {
	l := newTestLoop(t)
	app := l.app
	rend := app.rend
	rend.flashText, rend.flashGlyphText, rend.flashEnd = "x", "g", l.t.Add(2*time.Second)
	app.kb.FlashCode, app.kb.FlashUntil = KEY_SPACE, l.t.Add(150*time.Millisecond)

	app.expireFlash()
	if rend.dirtyFrames != 0 || rend.flashText == "" || app.kb.FlashCode == 0 {
		t.Fatal("cleared before the end")
	}
	l.advance(151)
	app.expireFlash()
	if rend.dirtyFrames == 0 || app.kb.FlashCode != 0 || rend.flashText == "" {
		t.Fatalf("key flash: dirty=%d code=%d text=%q", rend.dirtyFrames, app.kb.FlashCode, rend.flashText)
	}
	rend.dirtyFrames = 0
	l.advance(2000)
	app.expireFlash()
	if rend.dirtyFrames == 0 || rend.flashText != "" || rend.flashGlyphText != "" {
		t.Fatalf("flash: dirty=%d text=%q glyph=%q", rend.dirtyFrames, rend.flashText, rend.flashGlyphText)
	}
	rend.dirtyFrames = 0
	app.expireFlash()
	if rend.dirtyFrames != 0 {
		t.Error("marked dirty again after the flash ended")
	}

	app.visible = false
	rend.flashText, rend.flashEnd = "y", l.t
	l.advance(1)
	app.expireFlash()
	if rend.flashText != "y" {
		t.Error("hidden: flash handled (the loop skips it while hidden)")
	}
}

// The loop's parts in order: a pad press while shown reaches the injector and draws.
func TestLoopPass(t *testing.T) {
	l := newTestLoop(t)
	app := l.app
	l.pad.events = []Action{{Type: ActionBackspace}, {Type: ActionBackspaceRelease}}
	l.win.quit = true // one pass, then stop
	app.loop()
	if n := backspaces(l.keys()); n != 1 {
		t.Errorf("one pass sent %d Backspace, want 1", n)
	}
	if l.win.draws != 1 {
		t.Errorf("%d draws, want 1", l.win.draws)
	}
	if app.running.Load() {
		t.Error("quit event did not stop the loop")
	}
}

// A closed layer surface is shown again once, then hidden if it closes again soon.
func TestLayerClosed(t *testing.T) {
	l := newTestLoop(t)
	app := l.app
	l.win.closed = true
	app.pollWindow()
	if l.win.drops != 1 || l.win.reattaches != 1 || app.togglePending {
		t.Fatalf("first close: drops=%d reattaches=%d toggle=%v", l.win.drops, l.win.reattaches,
			app.togglePending)
	}
	l.advance(100)
	l.win.closed = true
	app.pollWindow()
	if l.win.drops != 2 || l.win.reattaches != 1 || !app.togglePending {
		t.Errorf("second close soon after: drops=%d reattaches=%d toggle=%v", l.win.drops, l.win.reattaches,
			app.togglePending)
	}
}

// Theme and sensitivity steps keep their ranges.
func TestCycleAndSensitivity(t *testing.T) {
	l := newTestLoop(t)
	app := l.app
	app.config.Mouse.Sensitivity = 49
	app.changeSensitivity(2)
	if app.config.Mouse.Sensitivity != 50 || l.pad.sensitivity != 50 {
		t.Errorf("up from 49: %d (pad %d), want 50", app.config.Mouse.Sensitivity, l.pad.sensitivity)
	}
	app.config.Mouse.Sensitivity = 2
	app.changeSensitivity(-2)
	if app.config.Mouse.Sensitivity != 1 || app.rend.flashGlyphText != "⟾" {
		t.Errorf("down from 2: %d glyph %q, want 1 and the down glyph", app.config.Mouse.Sensitivity,
			app.rend.flashGlyphText)
	}
}

// Sensitivity steps land on even values with 1 at the bottom, from any start.
func TestSensitivitySteps(t *testing.T) {
	walk := func(start, step, n int) []int {
		l := newTestLoop(t)
		l.app.config.Mouse.Sensitivity = start
		var got []int
		for range n {
			l.app.changeSensitivity(step)
			got = append(got, l.app.config.Mouse.Sensitivity)
		}
		return got
	}
	cases := []struct {
		name        string
		start, step int
		want        []int
	}{
		{"up from 31", 31, 2, []int{33, 35}},
		{"up to the top", 47, 2, []int{49, 50, 50}},
		{"down from 31", 31, -2, []int{29, 27}},
		{"down to the bottom", 4, -2, []int{2, 1, 1}},
		{"up from 1", 1, 2, []int{3, 5}},
	}
	for _, c := range cases {
		got := walk(c.start, c.step, len(c.want))
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("%s: %v, want %v", c.name, got, c.want)
				break
			}
		}
	}
}

// U48: the pad's X, Y and RT (Backspace, Space, Enter) always send the plain
// key: no armed Ctrl, Alt or Super, and no Shift even with LT held (LT + RT
// must not become Shift+Enter, a new line in chat apps). They leave armed
// modifiers for the next on-screen key.
func TestPadButtonsIgnoreModifiers(t *testing.T) {
	for _, c := range []struct {
		action ActionType
		code   uint16
	}{{ActionBackspace, KEY_BACKSPACE}, {ActionSpace, KEY_SPACE}, {ActionEnter, KEY_ENTER}} {
		l := newTestLoop(t)
		app, kb := l.app, l.app.kb
		kb.CtrlActive, kb.AltActive, kb.MetaActive = true, true, true
		app.handleAction(Action{Type: ActionShiftOn}, kb, app.inj, app.rend) // LT held
		app.handleAction(Action{Type: c.action}, kb, app.inj, app.rend)
		app.stopRepeat()
		var down []uint16
		for _, ev := range l.keys() {
			if ev.value == 1 {
				down = append(down, ev.code)
			}
		}
		if len(down) != 1 || down[0] != c.code {
			t.Errorf("action %v: pressed %v, want only key %d", c.action, down, c.code)
		}
		if !kb.CtrlActive || !kb.AltActive || !kb.MetaActive {
			t.Errorf("action %v: armed modifiers cleared (ctrl=%v alt=%v super=%v), want them kept", c.action, kb.CtrlActive, kb.AltActive, kb.MetaActive)
		}
	}
}

// U49: the pad goes away while shown, mid-sentence: what a held button or
// trigger did ends (held LT, repeat, the accent popup), but the armed Ctrl,
// Alt, Super and Caps stay for the next key once a pad is back, and the
// keyboard stays up and says so.
func TestPadLostKeepsArmedModifiers(t *testing.T) {
	l := newTestLoop(t)
	app, kb := l.app, l.app.kb
	kb.CtrlActive, kb.AltActive, kb.MetaActive, kb.CapsActive = true, true, true, true
	app.handleAction(Action{Type: ActionShiftOn}, kb, app.inj, app.rend) // LT held
	cursorOn(t, kb, "e")
	kb.AccentPopup = &AccentPopupState{Accents: accentE}
	app.startRepeat(ActionBackspace)
	l.pad.loseOnRead = true
	app.pollPad()
	if l.pad.Fd() >= 0 {
		t.Fatal("the fake pad was not lost")
	}
	if !kb.CtrlActive || !kb.AltActive || !kb.MetaActive || !kb.CapsActive {
		t.Errorf("armed cleared on pad loss: ctrl=%v alt=%v super=%v caps=%v", kb.CtrlActive, kb.AltActive, kb.MetaActive, kb.CapsActive)
	}
	if kb.ShiftHeld || kb.ShiftActive || kb.AccentPopup != nil || app.repeatAction != ActionNone {
		t.Errorf("held state left: shift held %v on %v, popup %v, repeat %v", kb.ShiftHeld, kb.ShiftActive, kb.AccentPopup != nil, app.repeatAction)
	}
	if !app.visible || app.rend.flashText != "Controller lost" {
		t.Errorf("visible %v, status %q; want shown and \"Controller lost\"", app.visible, app.rend.flashText)
	}
}
