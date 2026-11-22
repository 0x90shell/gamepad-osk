package main

import (
	"encoding/binary"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// syncPad is a fakePad the test feeds while the input goroutine reads it.
type syncPad struct {
	mu sync.Mutex
	p  fakePad
}

func (s *syncPad) send(a ...Action) {
	s.mu.Lock()
	s.p.events = append(s.p.events, a...)
	s.mu.Unlock()
}

func (s *syncPad) ReadEvents() []Action {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.p.ReadEvents()
}

func (s *syncPad) Fd() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.p.Fd()
}

func (s *syncPad) AnyButtonHeld() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.p.AnyButtonHeld()
}

func (s *syncPad) Grab() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.p.Grab()
}

func (s *syncPad) Ungrab() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.p.Ungrab()
}

func (s *syncPad) Reconnect() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.p.Reconnect()
}

func (s *syncPad) NeedsPolling() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.p.NeedsPolling()
}

func (s *syncPad) SetSensitivity(v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.p.SetSensitivity(v)
}

// threadWindow is a fake window for the threaded app. It records each call
// made off the main thread, and draw reads the frame like Renderer.Draw.
type threadWindow struct {
	rend    *Renderer
	mainTid int // set on the main goroutine before runThreaded

	mu          sync.Mutex
	shows       int
	hides       int
	draws       int // frames drawn
	drawCalls   int // every call, drawn or not
	reattaches  int
	drops       int
	sinceAttach int // frames drawn since the last reattach
	offMain     []string

	quit        atomic.Bool
	closed      atomic.Bool
	inDraw      atomic.Bool
	drawnCol    atomic.Int64 // cursor column of the last frame drawn
	drawnShift  atomic.Bool  // Shift on in the last frame drawn
	notReady    atomic.Bool  // like a layer surface not configured yet: draw draws nothing
	slowMs      atomic.Int64
	drawStarted chan struct{}
}

func (w *threadWindow) record(call string, count *int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if tid := syscall.Gettid(); tid != w.mainTid {
		w.offMain = append(w.offMain, call)
	}
	if count != nil {
		*count++
	}
}

// show and reattach mark the frame dirty, as sdlWindow does.
func (w *threadWindow) show()               { w.record("show", &w.shows); w.rend.MarkDirty() }
func (w *threadWindow) hide()               { w.record("hide", &w.hides) }
func (w *threadWindow) quitRequested() bool { w.record("quitRequested", nil); return w.quit.Load() }
func (w *threadWindow) layerClosed() bool   { w.record("layerClosed", nil); return w.closed.Swap(false) }
func (w *threadWindow) dropLayer()          { w.record("dropLayer", &w.drops) }
func (w *threadWindow) reattach() {
	w.record("reattach", &w.reattaches)
	w.mu.Lock()
	w.sinceAttach = 0
	w.mu.Unlock()
	w.rend.MarkDirty()
}

// draw skips like Renderer.Draw: nothing when no frame is dirty or the
// surface is not ready.
func (w *threadWindow) draw(kb *KeyboardState) {
	w.record("draw", &w.drawCalls)
	if w.rend.dirtyFrames <= 0 || w.notReady.Load() {
		return
	}
	w.record("draw", &w.draws)
	w.mu.Lock()
	w.sinceAttach++
	w.mu.Unlock()
	_ = kb.IsFlashed(kb.Layout[kb.CursorRow][kb.CursorCol]) // Draw writes FlashCode
	w.drawnCol.Store(int64(kb.CursorCol))
	w.drawnShift.Store(kb.ShiftActive)
	w.rend.dirtyFrames--
	select {
	case w.drawStarted <- struct{}{}:
	default:
	}
	if ms := w.slowMs.Load(); ms > 0 {
		w.inDraw.Store(true)
		// slow until the time is up or the test sets slowMs back to 0
		for end := time.Now().Add(time.Duration(ms) * time.Millisecond); time.Now().Before(end) && w.slowMs.Load() > 0; {
			time.Sleep(2 * time.Millisecond)
		}
		w.inDraw.Store(false)
	}
}

func (w *threadWindow) counts() (shows, hides, draws int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.shows, w.hides, w.draws
}

// timedKey is one EV_KEY event and when the recorder read it.
type timedKey struct {
	keyEvent
	at     time.Time
	inDraw bool // the fake Draw was blocked when it arrived
}

// threadedLoop runs the app as Run does (runThreaded) on fakes, with the
// real clock and a recorder on the injector pipe.
type threadedLoop struct {
	app     *App
	pad     *syncPad
	win     *threadWindow
	pw      *os.File
	stopped chan struct{}
	read    chan struct{} // closed when the recorder reached EOF

	mu   sync.Mutex
	evs  []timedKey
	tst  *testing.T
	once sync.Once
}

// startThreaded builds the app, lets setup change it, then starts it.
func startThreaded(t *testing.T, setup func(app *App)) *threadedLoop {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	ValidateConfig(&cfg)
	rend := &Renderer{}
	l := &threadedLoop{pad: &syncPad{p: fakePad{fd: 3}}, pw: pw, tst: t,
		stopped: make(chan struct{}), read: make(chan struct{})}
	l.win = &threadWindow{rend: rend, drawStarted: make(chan struct{}, 1)}
	l.app = &App{config: cfg, visible: true, shown: true,
		kb: NewKeyboardState(LayoutQWERTY), inj: &Injector{fd: pw}, rend: rend,
		pad: l.pad, win: l.win}
	l.app.running.Store(true)
	if setup != nil {
		setup(l.app)
	}

	go func() {
		defer close(l.read)
		defer func() { _ = pr.Close() }()
		buf := make([]byte, 24)
		for {
			if _, err := io.ReadFull(pr, buf); err != nil {
				return
			}
			if binary.LittleEndian.Uint16(buf[16:]) != 1 { // EV_KEY only
				continue
			}
			e := timedKey{keyEvent{binary.LittleEndian.Uint16(buf[18:]),
				int32(binary.LittleEndian.Uint32(buf[20:]))}, //nolint:gosec // G115: event value
				time.Now(), l.win.inDraw.Load()}
			l.mu.Lock()
			l.evs = append(l.evs, e)
			l.mu.Unlock()
		}
	}()
	go func() {
		runtime.LockOSThread() // the main thread, as main.go's init does
		l.win.mainTid = syscall.Gettid()
		l.app.runThreaded()
		_ = l.pw.Close() // Run closes the injector after runThreaded returns
		close(l.stopped)
	}()
	t.Cleanup(func() { l.stop() })
	return l
}

// stop ends the app the way SIGTERM does and waits for Run's part to return.
func (l *threadedLoop) stop() {
	l.once.Do(func() {
		l.win.slowMs.Store(0)
		l.app.running.Store(false)
		l.waitStopped()
	})
}

func (l *threadedLoop) waitStopped() {
	l.tst.Helper()
	select {
	case <-l.stopped:
	case <-time.After(5 * time.Second):
		l.tst.Fatal("the app did not stop within 5 s")
	}
	select {
	case <-l.read:
	case <-time.After(time.Second):
		l.tst.Fatal("the injector pipe was not closed")
	}
}

func (l *threadedLoop) events() []timedKey {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]timedKey(nil), l.evs...)
}

// waitKey waits for an EV_KEY event with code and value after from.
func (l *threadedLoop) waitKey(code uint16, value int32, from time.Time, timeout time.Duration) (timedKey, bool) {
	deadline := time.Now().Add(timeout)
	for {
		for _, e := range l.events() {
			if e.code == code && e.value == value && !e.at.Before(from) {
				return e, true
			}
		}
		if !time.Now().Before(deadline) {
			return timedKey{}, false
		}
		time.Sleep(time.Millisecond)
	}
}

// waitFor polls cond for up to 1 s.
func waitFor(cond func() bool) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return cond()
}

// stillHeld lists the codes whose last event is a press.
func stillHeld(evs []timedKey) []uint16 {
	last := map[uint16]int32{}
	var order []uint16
	for _, e := range evs {
		if _, ok := last[e.code]; !ok {
			order = append(order, e.code)
		}
		last[e.code] = e.value
	}
	var held []uint16
	for _, c := range order {
		if last[c] != 0 {
			held = append(held, c)
		}
	}
	return held
}

// holdEverything sets up a held Shift, Alt+Tab, mouse drag and key repeat.
func holdEverything(t *testing.T, l *threadedLoop) {
	t.Helper()
	// Alt+Tab first: with Shift on, the AltTab key does something else
	l.pad.send(Action{Type: ActionPressStart}, Action{Type: ActionPress}) // cursor on AltTab
	if _, ok := l.waitKey(KEY_TAB, 1, time.Time{}, time.Second); !ok {
		t.Fatal("no Alt+Tab")
	}
	l.pad.send(Action{Type: ActionShiftOn}, Action{Type: ActionLeftClick}, Action{Type: ActionBackspace})
	if _, ok := l.waitKey(0x110, 1, time.Time{}, time.Second); !ok {
		t.Fatal("no mouse button press")
	}
	// The repeat is running: Backspace comes again
	first, _ := l.waitKey(KEY_BACKSPACE, 1, time.Time{}, time.Second)
	if _, ok := l.waitKey(KEY_BACKSPACE, 1, first.at.Add(time.Nanosecond), time.Second); !ok {
		t.Fatal("Backspace does not repeat")
	}
}

func onAltTab(t *testing.T) func(app *App) {
	return func(app *App) {
		app.config.Keys.RepeatDelayMs, app.config.Keys.RepeatRateMs = 30, 20
		cursorOn(t, app.kb, "AltTab")
	}
}

// L07: pad events while Draw is blocked for 1.5 s reach the injector at once.
func TestKeysDoNotWaitForDraw(t *testing.T) {
	l := startThreaded(t, func(app *App) { app.config.Keys.RepeatDelayMs = 5000 })
	l.win.slowMs.Store(1500)
	l.pad.send(Action{Type: ActionNavigate, DX: 1}) // a new frame: main starts a slow draw
	select {
	case <-l.win.drawStarted:
	case <-time.After(time.Second):
		t.Fatal("no draw")
	}
	waitFor(l.win.inDraw.Load)

	for i := range 3 {
		sent := time.Now()
		l.pad.send(Action{Type: ActionBackspace}, Action{Type: ActionBackspaceRelease})
		e, ok := l.waitKey(KEY_BACKSPACE, 1, sent, time.Second)
		if !ok {
			t.Fatalf("press %d: no Backspace within 1 s", i)
		}
		if d := e.at.Sub(sent); d > 50*time.Millisecond {
			t.Errorf("press %d: Backspace %v after the pad event, want under 50 ms", i, d)
		}
		if !e.inDraw {
			t.Fatalf("press %d: the draw was not blocked when the key came; the test proves nothing", i)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// L08: window, renderer and file work from the input side is queued, runs
// on the main thread, and runs in order.
func TestWindowWorkIsQueued(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	l := newTestLoop(t)
	app := l.app
	app.ui = queuedUI{app}
	app.wake = make(chan struct{}, 1)
	app.config.Gamepad.Grab = false
	rend := app.rend
	theme := rend.theme
	posTop := app.posTop

	app.cycleTheme(1)
	app.changeSensitivity(2)
	app.kb.OnNotice = func(msg string) { app.out().Flash(msg) }
	app.kb.OnNotice("notice")
	app.handleAction(Action{Type: ActionPositionToggle}, app.kb, app.inj, app.out())
	app.togglePending = true
	app.applyToggle() // hide
	app.togglePending = true
	app.applyToggle() // show

	if !reflect.DeepEqual(rend.theme, theme) || rend.flashText != "" || rend.flashGlyphText != "" ||
		app.posTop != posTop || l.win.shows != 0 || l.win.hides != 0 || !app.shown {
		t.Fatal("the input side changed the renderer or the window itself")
	}
	if _, err := os.Stat(UserConfigPath()); err == nil {
		t.Fatal("the input side wrote the config file itself")
	}
	if l.pad.sensitivity != app.config.Mouse.Sensitivity {
		t.Error("the pad did not get the new sensitivity at once")
	}

	// Run the queue the way main does; positionToggle needs SDL, so check
	// that job by count and leave it out.
	app.jobs.mu.Lock()
	jobs := app.jobs.jobs
	app.jobs.jobs = nil
	app.jobs.mu.Unlock()
	// theme, theme save, sensitivity flash, sensitivity save, notice, position, hide, show
	if len(jobs) != 8 {
		t.Fatalf("%d jobs queued, want 8", len(jobs))
	}
	for i, job := range jobs {
		if i == 5 {
			continue
		}
		job()
		switch i {
		case 0:
			if rend.theme.Name != Themes[themeOrder[app.themeIdx]].Name {
				t.Errorf("theme job: theme %q", rend.theme.Name)
			}
		case 1:
			if _, err := os.Stat(UserConfigPath()); err != nil {
				t.Errorf("theme save job wrote nothing: %v", err)
			}
		case 2:
			if rend.flashGlyphText == "" || rend.flashText != strconv.Itoa(app.config.Mouse.Sensitivity) {
				t.Errorf("sensitivity flash job: glyph %q text %q", rend.flashGlyphText, rend.flashText)
			}
		case 3:
			if v := userConfigValue(t, "sensitivity"); v != strconv.Itoa(app.config.Mouse.Sensitivity) {
				t.Errorf("sensitivity save job: config has %q, want %d", v, app.config.Mouse.Sensitivity)
			}
		case 4:
			if rend.flashText != "notice" {
				t.Errorf("notice job: flash %q", rend.flashText)
			}
		case 6:
			if app.shown || l.win.hides != 1 {
				t.Errorf("hide job: shown %v, hides %d", app.shown, l.win.hides)
			}
		case 7:
			if !app.shown || l.win.shows != 1 {
				t.Errorf("show job: shown %v, shows %d", app.shown, l.win.shows)
			}
		}
	}
}

// L08: in the running app, every window call is made on the main thread.
func TestWindowCallsOnMainThread(t *testing.T) {
	l := startThreaded(t, nil)
	l.pad.send(Action{Type: ActionNavigate, DX: 1})
	toggle := func() {
		l.app.lock.Lock()
		l.app.togglePending = true
		l.app.lock.Unlock()
	}
	toggle() // hide
	if !waitFor(func() bool { _, h, _ := l.win.counts(); return h == 1 }) {
		t.Fatal("no hide")
	}
	toggle() // show
	if !waitFor(func() bool { s, _, _ := l.win.counts(); return s == 1 }) {
		t.Fatal("no show")
	}
	time.Sleep(100 * time.Millisecond) // the frames from the show are drawn
	l.win.closed.Store(true)           // the compositor closes the layer surface: main reattaches
	if !waitFor(func() bool {
		l.win.mu.Lock()
		defer l.win.mu.Unlock()
		return l.win.reattaches == 1 && l.win.drops == 1
	}) {
		t.Fatal("no drop and reattach after the layer surface closed")
	}
	if !waitFor(func() bool {
		l.win.mu.Lock()
		defer l.win.mu.Unlock()
		return l.win.sinceAttach > 0
	}) {
		t.Error("no draw after the reattach")
	}
	l.pad.send(Action{Type: ActionNavigate, DX: 1})
	l.app.out().Flash("x")
	time.Sleep(50 * time.Millisecond)
	l.stop()
	l.win.mu.Lock()
	defer l.win.mu.Unlock()
	if len(l.win.offMain) > 0 {
		t.Errorf("window calls off the main thread: %v", l.win.offMain)
	}
}

// L09: hide during a held Shift, Alt+Tab, mouse drag and repeat: nothing stays held.
func TestHideReleasesEverything(t *testing.T) {
	l := startThreaded(t, onAltTab(t))
	holdEverything(t, l)
	l.app.lock.Lock()
	l.app.togglePending = true // IPC toggle
	l.app.lock.Unlock()
	if !waitFor(func() bool { _, h, _ := l.win.counts(); return h == 1 }) {
		t.Fatal("no hide")
	}
	hidden := time.Now()
	// The hide publishes a frame: its copy of the state shows what hide cleared
	// (reading app.kb itself would race with the input side)
	if f := l.app.frame.Load(); f.ShiftActive || f.ShiftHeld || f.AltTabHeld || f.CapsActive {
		t.Errorf("frame after hide: Shift %v held %v, Alt+Tab %v, Caps %v",
			f.ShiftActive, f.ShiftHeld, f.AltTabHeld, f.CapsActive)
	}
	time.Sleep(150 * time.Millisecond) // several repeat periods
	if e, ok := l.waitKey(KEY_BACKSPACE, 1, hidden, 0); ok {
		t.Errorf("Backspace repeated %v after hide", e.at.Sub(hidden))
	}
	if held := stillHeld(l.events()); len(held) > 0 {
		t.Errorf("held after hide: %v", held)
	}
	for _, code := range []uint16{KEY_LEFTSHIFT, KEY_LEFTALT, 0x110} {
		if _, ok := l.waitKey(code, 0, hidden.Add(-time.Second), 0); !ok {
			t.Errorf("hide did not release 0x%x", code)
		}
	}
}

// L10: on exit (SIGTERM, or the window's quit event) the input goroutine lets
// go of everything before Run closes the injector: a release sent after the
// close is lost, so the key would show as held here.
func TestExitReleasesBeforeClose(t *testing.T) {
	for _, how := range []string{"sigterm", "quit event"} {
		t.Run(how, func(t *testing.T) {
			l := startThreaded(t, onAltTab(t))
			holdEverything(t, l)
			if how == "sigterm" {
				l.app.running.Store(false)
			} else {
				l.win.quit.Store(true)
			}
			l.waitStopped()
			// The recorder read to EOF: what is here was written before the close.
			if held := stillHeld(l.events()); len(held) > 0 {
				t.Errorf("held when the injector closed: %v", held)
			}
		})
	}
}

// L11: an IPC toggle while the input side handles a stream of actions gives
// exactly one hide, then one show, with no race.
func TestIPCToggleDuringActions(t *testing.T) {
	l := startThreaded(t, nil)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() { // the pad keeps the input side busy
		for {
			select {
			case <-stop:
				return
			default:
			}
			l.pad.send(Action{Type: ActionNavigate, DX: 1}, Action{Type: ActionMouseMove, DX: 1})
			time.Sleep(time.Millisecond)
		}
	})
	ipc := func() {
		l.app.lock.Lock()
		l.app.togglePending = true
		l.app.lock.Unlock()
	}
	time.Sleep(50 * time.Millisecond)
	ipc()
	if !waitFor(func() bool { _, h, _ := l.win.counts(); return h == 1 }) {
		t.Fatal("no hide")
	}
	time.Sleep(50 * time.Millisecond)
	ipc()
	if !waitFor(func() bool { s, _, _ := l.win.counts(); return s == 1 }) {
		t.Fatal("no show")
	}
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
	l.stop()
	if s, h, _ := l.win.counts(); s != 1 || h != 1 {
		t.Errorf("shows %d, hides %d; want 1 each", s, h)
	}
	if !l.app.visible || !l.app.shown {
		t.Errorf("visible %v, shown %v after hide and show", l.app.visible, l.app.shown)
	}
}

// The frame main draws is a full copy: every field Draw reads, and its own
// accent popup, so IsFlashed and the popup never touch the input side's state.
func TestPublishCopiesState(t *testing.T) {
	l := newTestLoop(t)
	app, kb := l.app, l.app.kb
	cursorOn(t, kb, "a")
	kb.ShiftActive, kb.CtrlActive, kb.CapsActive = true, true, true
	kb.AccentPopup = &AccentPopupState{Accents: accentA, Selected: 2}
	kb.FlashCode, kb.FlashUntil = KEY_SPACE, time.Now().Add(time.Second)
	app.publish()
	f := app.frame.Load()
	if f == kb || f.AccentPopup == kb.AccentPopup {
		t.Fatal("the frame shares the state or the popup with the input side")
	}
	a, b := *f, *kb
	a.AccentPopup, b.AccentPopup = nil, nil
	a.OnThemeCycle, a.OnThemeCycleReverse, a.OnSensitivityUp, a.OnSensitivityDown, a.OnNotice = nil, nil, nil, nil, nil
	b.OnThemeCycle, b.OnThemeCycleReverse, b.OnSensitivityUp, b.OnSensitivityDown, b.OnNotice = nil, nil, nil, nil, nil
	if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(*f.AccentPopup, *kb.AccentPopup) {
		t.Error("the frame differs from the state")
	}
	if app.frameSeq.Load() != 1 {
		t.Errorf("frameSeq %d after one publish", app.frameSeq.Load())
	}
}

// Main runs queued jobs in the order they came, so a show never runs after
// a hide that came later.
func TestJobQueueOrder(t *testing.T) {
	var q jobQueue
	var got []int
	for i := range 3 {
		q.push(func() { got = append(got, i) })
	}
	q.run()
	q.run() // nothing left
	if !reflect.DeepEqual(got, []int{0, 1, 2}) {
		t.Errorf("jobs ran as %v, want [0 1 2]", got)
	}
}

// A change on the input side reaches the frame main draws.
func TestFrameFollowsState(t *testing.T) {
	l := startThreaded(t, nil)
	if !waitFor(func() bool { _, _, d := l.win.counts(); return d > 0 }) {
		t.Fatal("no first draw")
	}
	start := l.win.drawnCol.Load()
	l.pad.send(Action{Type: ActionNavigate, DX: 1})
	if !waitFor(func() bool { return l.win.drawnCol.Load() == start+1 }) {
		t.Errorf("drawn cursor column %d after a move right from %d", l.win.drawnCol.Load(), start)
	}
}

// Main acts on its own copy of visible: a layer surface closed while a show
// is still queued is left for the show job, not reattached early.
func TestLayerCloseUsesMainsShown(t *testing.T) {
	l := newTestLoop(t)
	app := l.app
	app.visible, app.shown = true, false // shown on the input side, show job not run yet
	l.win.closed = true
	app.pollWindow()
	if l.win.reattaches != 0 || l.win.drops != 0 || app.togglePending {
		t.Errorf("hidden on main: reattaches %d, drops %d, toggle %v", l.win.reattaches, l.win.drops, app.togglePending)
	}

	// The other way: hidden on the input side, the hide job not run yet. Main
	// still shows the keyboard, so it reattaches; the hide job follows.
	app.visible, app.shown = false, true
	l.win.closed = true
	app.pollWindow()
	if l.win.reattaches != 1 || l.win.drops != 1 {
		t.Errorf("shown on main: reattaches %d, drops %d, want 1 each", l.win.reattaches, l.win.drops)
	}
}

// userConfigValue reads key from the user config file ("" if missing).
func userConfigValue(t *testing.T, key string) string {
	t.Helper()
	data, err := os.ReadFile(UserConfigPath())
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == key {
			v, _, _ = strings.Cut(v, "#")
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// A setting changed just before the exit is still saved: main runs the
// queued save after the input side stops.
func TestExitKeepsQueuedSave(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	l := startThreaded(t, func(app *App) {
		cursorOn(t, app.kb, themeKeyLabel)
		app.kb.OnThemeCycle = func() { app.cycleTheme(1) }
	})
	l.win.slowMs.Store(300) // main is in a slow frame when the save is queued
	l.pad.send(Action{Type: ActionNavigate, DX: 0})
	if !waitFor(l.win.inDraw.Load) {
		t.Fatal("no slow draw")
	}
	l.pad.send(Action{Type: ActionPressStart}, Action{Type: ActionPress})
	time.Sleep(50 * time.Millisecond) // the input side handled it; main still drawing
	if !l.win.inDraw.Load() {
		t.Fatal("main finished the frame before the exit; the test proves nothing")
	}
	l.app.running.Store(false)
	l.waitStopped()
	want := themeOrder[l.app.themeIdx]
	if got := userConfigValue(t, "name"); got != want {
		t.Errorf("theme in the config after exit: %q, want %q", got, want)
	}
}

// After hide and show, main draws the cleared state, not the frame from
// before the hide (Shift was on).
func TestShowDrawsClearedState(t *testing.T) {
	l := startThreaded(t, nil)
	l.pad.send(Action{Type: ActionShiftOn})
	if !waitFor(l.win.drawnShift.Load) {
		t.Fatal("Shift never drawn")
	}
	toggle := func() {
		l.app.lock.Lock()
		l.app.togglePending = true
		l.app.lock.Unlock()
	}
	toggle()
	if !waitFor(func() bool { _, h, _ := l.win.counts(); return h == 1 }) {
		t.Fatal("no hide")
	}
	toggle()
	if !waitFor(func() bool { s, _, _ := l.win.counts(); return s == 1 }) {
		t.Fatal("no show")
	}
	time.Sleep(100 * time.Millisecond)
	if l.win.drawnShift.Load() {
		t.Error("after hide and show the frame still shows Shift on")
	}
}

// While the surface cannot draw (layer surface not configured), main waits
// between tries instead of spinning.
func TestNoSpinWhileNotDrawable(t *testing.T) {
	l := startThreaded(t, nil)
	if !waitFor(func() bool { _, _, d := l.win.counts(); return d > 0 }) {
		t.Fatal("no first draw")
	}
	start := l.win.drawnCol.Load()
	l.win.notReady.Store(true)
	l.pad.send(Action{Type: ActionNavigate, DX: 1})
	time.Sleep(20 * time.Millisecond)
	l.win.mu.Lock()
	before := l.win.drawCalls
	l.win.mu.Unlock()
	time.Sleep(200 * time.Millisecond)
	l.win.mu.Lock()
	n := l.win.drawCalls - before
	l.win.mu.Unlock()
	if n > 40 { // a 16 ms wait gives about 13
		t.Errorf("%d draw tries in 200 ms while not drawable: main spins", n)
	}
	l.win.notReady.Store(false)
	if !waitFor(func() bool { return l.win.drawnCol.Load() == start+1 }) {
		t.Error("no frame once the surface was ready")
	}
}

func TestSlowDrawSetting(t *testing.T) {
	for v, want := range map[string]time.Duration{"": 0, "0": 0, "-5": 0, "x": 0, "1.5": 0,
		"1500": 1500 * time.Millisecond, "1": time.Millisecond} {
		if got := slowDrawSetting(v); got != want {
			t.Errorf("slowDrawSetting(%q) = %v, want %v", v, got, want)
		}
	}
}

// main.go keeps the main goroutine on the first OS thread (SDL needs it).
// Nothing at run time shows a missing lock reliably, so check the source.
func TestMainLocksOSThread(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "init" || fn.Recv != nil {
			continue
		}
		for _, st := range fn.Body.List { // a statement of init itself, not inside an if or a goroutine
			es, ok := st.(*ast.ExprStmt)
			if !ok {
				continue
			}
			call, ok := es.X.(*ast.CallExpr)
			if !ok {
				continue
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "LockOSThread" {
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "runtime" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("main.go has no init that calls runtime.LockOSThread")
	}
}

// The input side makes no SDL, X11, layer-shell, renderer, window or config
// file call itself: each goes through a job (a func literal passed to Later).
// The race detector cannot see a call on the wrong thread, so check the source.
func TestInputSideMakesNoWindowCalls(t *testing.T) {
	inputSide := map[string]bool{"inputLoop": true, "applyToggle": true, "releaseHeld": true, "pollPad": true,
		"checkGrab": true, "checkReconnect": true, "checkTimers": true, "expireKeyFlash": true, "pollMs": true,
		"handleAction": true, "cycleTheme": true, "changeSensitivity": true, "publish": true,
		"releasePadHolds": true, "startRepeat": true, "stopRepeat": true, "wakeMain": true, "out": true}
	banned := func(name string) bool {
		for _, p := range []string{"SDL3", "TTF3", "Save", "Restore", "SetNoFocus", "Warp", "attachLayer",
			"destroyLayer", "repositionLayer", "forgetTarget", "applyOpacity", "setLayer", "IsFullscreen",
			"IsSaved", "computeY", "togglePosition", "hasLayerShell", "probeLayerShell", "takeLayerSurface",
			"createWaylandWindow", "cleanupLayerShell", "GetWorkarea", "GetStrutInsets", "initX11"} {
			if strings.HasPrefix(name, p) {
				return true
			}
		}
		return false
	}
	var bad []string
	check := func(where string, body ast.Node) {
		ast.Inspect(body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Later" {
					return false // the job runs on main
				}
				var name string
				switch fn := call.Fun.(type) {
				case *ast.Ident:
					name = fn.Name
				case *ast.SelectorExpr:
					name = fn.Sel.Name
				}
				if banned(name) {
					bad = append(bad, where+": "+name)
				}
			}
			// Any use of the renderer or window, also a copy into a local
			if sel, ok := n.(*ast.SelectorExpr); ok && (sel.Sel.Name == "rend" || sel.Sel.Name == "win" ||
				sel.Sel.Name == "window") {
				bad = append(bad, where+": ."+sel.Sel.Name)
			}
			return true
		})
	}
	seen := map[string]bool{}
	for _, file := range []string{"loop.go", "app.go", "keyboard.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			// keyboard.go is all input side; elsewhere the listed App methods
			if file == "keyboard.go" || (fn.Recv != nil && inputSide[fn.Name.Name]) {
				check(file+" "+fn.Name.Name, fn.Body)
				seen[fn.Name.Name] = true
			}
		}
	}
	for name := range inputSide {
		if !seen[name] {
			t.Errorf("input-side function %s not found: the list is out of date", name)
		}
	}
	if len(bad) > 0 {
		t.Errorf("window or SDL work on the input side:\n  %s", strings.Join(bad, "\n  "))
	}
}
