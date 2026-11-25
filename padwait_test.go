package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func waitReady(w *padWatch, within time.Duration) bool {
	select {
	case <-w.ready:
		return true
	case <-time.After(within):
		return false
	}
}

func watchedPipe(t *testing.T) (*padWatch, *os.File, *os.File) {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w := newPadWatch()
	t.Cleanup(func() { w.end(); _ = pr.Close(); _ = pw.Close() })
	return w, pr, pw
}

// The watcher fires on data, stays quiet once the reader drained it, and
// fires again on the next write.
func TestPadWatchSignalsEachEvent(t *testing.T) {
	w, pr, pw := watchedPipe(t)
	w.start(pr)
	if waitReady(w, 50*time.Millisecond) {
		t.Fatal("ready with nothing to read")
	}
	_, _ = pw.Write([]byte{1})
	if !waitReady(w, time.Second) {
		t.Fatal("no ready after a write")
	}
	_, _ = pr.Read(make([]byte, 8))
	w.drainedAll()
	if waitReady(w, 50*time.Millisecond) {
		t.Fatal("ready again after the data was drained")
	}
	_, _ = pw.Write([]byte{2})
	if !waitReady(w, time.Second) {
		t.Fatal("no ready after the second write")
	}
}

// Data that came before the watcher started is not lost.
func TestPadWatchSeesDataFromBeforeStart(t *testing.T) {
	w, pr, pw := watchedPipe(t)
	_, _ = pw.Write([]byte{1})
	w.start(pr)
	if !waitReady(w, time.Second) {
		t.Fatal("no ready for data written before start")
	}
}

// Data still unread when the reader says it drained fires again at once.
func TestPadWatchUnreadDataFiresAgain(t *testing.T) {
	w, pr, pw := watchedPipe(t)
	w.start(pr)
	_, _ = pw.Write([]byte{1, 2})
	if !waitReady(w, time.Second) {
		t.Fatal("no ready after a write")
	}
	w.drainedAll() // nothing read
	if !waitReady(w, time.Second) {
		t.Fatal("no ready for data left unread")
	}
}

// The watcher of a closed fd never marks the next fd's watch broken, even
// when the fd closes before that watcher first runs (pad gone at once).
func TestPadWatchOldWatcherLeavesNewAlone(t *testing.T) {
	w := newPadWatch()
	defer w.end()
	for i := range 50 {
		old, ow, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		w.start(old)
		w.end()
		_ = old.Close()
		_ = ow.Close()
		pr, pw, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		w.start(pr)
		broken := false
		for end := time.Now().Add(20 * time.Millisecond); time.Now().Before(end) && !broken; time.Sleep(time.Millisecond) {
			broken = w.broken() // the old watcher has run by the end of this
		}
		w.end()
		_ = pr.Close()
		_ = pw.Close()
		if broken {
			t.Fatalf("round %d: the new watch is broken", i+1)
		}
	}
}

// A regular file is never waited on: the input side polls instead.
func TestPadWatchRegularFileFails(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "pad"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	w := newPadWatch()
	defer w.end()
	w.start(f)
	waitReady(w, time.Second)
	if !w.broken() {
		t.Error("a regular file is watched")
	}
}

func TestGamepadReaderReady(t *testing.T) {
	gp := NewGamepadReader(DefaultConfig())
	if ch, ok := gp.Ready(); ch != nil || !ok {
		t.Errorf("no pad open: want (nil, true), got (%v, %v)", ch, ok)
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pw.Close() }()
	gp.fd = pr
	gp.watch.start(pr)
	if ch, ok := gp.Ready(); ch == nil || !ok {
		t.Errorf("pad open: want its ready channel, got (%v, %v)", ch, ok)
	}
	gp.watch.fail()
	if _, ok := gp.Ready(); ok {
		t.Error("broken watch: Ready says wait")
	}
	gp.Close()
}

// After ReadEvents drains the pad, the next event fires ready again.
func TestGamepadReaderReadyAfterRead(t *testing.T) {
	var fds [2]int
	if err := syscall.Pipe2(fds[:], syscall.O_NONBLOCK|syscall.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	pr, pw := os.NewFile(uintptr(fds[0]), "pad"), os.NewFile(uintptr(fds[1]), "w")
	defer func() { _ = pw.Close() }()
	gp := NewGamepadReader(DefaultConfig())
	gp.fd = pr
	gp.watch.start(pr)
	defer gp.Close()
	ev := make([]byte, 24) // EV_SYN: nothing to act on
	for i := range 3 {
		_, _ = pw.Write(ev)
		ch, _ := gp.Ready()
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("event %d: no ready", i+1)
		}
		gp.ReadEvents()
	}
}

// A pad fd that reads nothing (EOF) yet always polls readable is polled,
// not watched: watching it would spin.
func TestGamepadReaderEOFPolls(t *testing.T) {
	var fds [2]int
	if err := syscall.Pipe2(fds[:], syscall.O_NONBLOCK|syscall.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	pr := os.NewFile(uintptr(fds[0]), "pad")
	_ = syscall.Close(fds[1]) // no writer: reads return 0
	gp := NewGamepadReader(DefaultConfig())
	gp.fd = pr
	gp.watch.start(pr)
	defer gp.Close()
	gp.ReadEvents()
	if _, ok := gp.Ready(); ok {
		t.Error("an fd at EOF is still waited on")
	}
}

// A read error (the pad is gone) stops the watcher when the pad closes.
func TestGamepadReaderDisconnectEndsWatch(t *testing.T) {
	var fds [2]int
	if err := syscall.Pipe2(fds[:], syscall.O_NONBLOCK|syscall.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Close(fds[0]) }()
	pw := os.NewFile(uintptr(fds[1]), "pad") // write end: read fails with EBADF
	gp := NewGamepadReader(DefaultConfig())
	gp.fd = pw
	gp.watch.start(pw)
	gp.ReadEvents()
	if gp.fd != nil {
		t.Fatal("the read error did not close the pad")
	}
	if gp.watch.cur != nil {
		t.Error("the watcher was not stopped when the pad closed")
	}
}

// Open keeps the pad fd non-blocking after Fd(), which ReadEvents calls:
// a blocking fd would hang the input side on an idle pad. A FIFO goes into
// Go's netpoller like an evdev node.
func TestOpenKeepsPadNonBlocking(t *testing.T) {
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	fifo := filepath.Join(t.TempDir(), "pad")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	wr, err := os.OpenFile(fifo, os.O_RDWR, 0) // a writer, so a blocking open would not hang
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = wr.Close() }()
	gp := NewGamepadReader(DefaultConfig())
	if !gp.Open(fifo) {
		t.Fatal("cannot open the FIFO")
	}
	defer gp.Close()
	fd := gp.Fd()
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_GETFL, 0) //nolint:gosec // G115: fd
	if errno != 0 {
		t.Fatal(errno)
	}
	if flags&syscall.O_NONBLOCK == 0 {
		t.Error("the pad fd is blocking after Fd()")
	}
}

func TestIdleWait(t *testing.T) {
	now := time.Now()
	cfg := DefaultConfig()
	ValidateConfig(&cfg)
	cases := []struct {
		name string
		set  func(app *App, p *fakePad)
		want time.Duration
	}{
		{"idle", func(*App, *fakePad) {}, time.Second}, // the cap: a lost wake costs at most 1 s
		{"stick off center", func(_ *App, p *fakePad) { p.polling = true }, 4 * time.Millisecond},
		{"key repeat", func(app *App, _ *fakePad) {
			app.repeatAction, app.repeatInitial, app.repeatStart = ActionNavigate, false, now
			app.repeatLast = now
		}, 16 * time.Millisecond}, // as pollMs: repeat polls as before
		{"key repeat near due", func(app *App, _ *fakePad) {
			app.repeatAction, app.repeatInitial, app.repeatStart = ActionNavigate, false, now
			app.repeatLast = now.Add(-time.Duration(cfg.Keys.RepeatRateMs-10) * time.Millisecond)
		}, 10 * time.Millisecond},
		{"first repeat near due", func(app *App, _ *fakePad) {
			app.repeatAction, app.repeatInitial = ActionNavigate, true
			app.repeatStart = now.Add(-time.Duration(cfg.Keys.RepeatDelayMs-10) * time.Millisecond)
		}, 10 * time.Millisecond},
		{"long press", func(app *App, _ *fakePad) {
			app.kb.LongPressActive, app.kb.LongPressStart = true, now.Add(-100*time.Millisecond)
		}, time.Duration(cfg.Gamepad.LongPressMs-100) * time.Millisecond},
		{"long press hidden", func(app *App, _ *fakePad) {
			app.visible = false
			app.kb.LongPressActive, app.kb.LongPressStart = true, now
		}, maxIdleWait},
		{"long press overdue", func(app *App, _ *fakePad) {
			app.kb.LongPressActive, app.kb.LongPressStart = true, now.Add(-time.Hour)
		}, 16 * time.Millisecond},
		{"alt+tab", func(app *App, _ *fakePad) {
			app.kb.AltTabHeld, app.kb.AltTabLastTab = true, now.Add(-2500*time.Millisecond)
		}, 500 * time.Millisecond},
		{"key flash", func(app *App, _ *fakePad) {
			app.kb.FlashCode, app.kb.FlashUntil = KEY_A, now.Add(150*time.Millisecond)
		}, 150 * time.Millisecond},
		{"key flash hidden", func(app *App, _ *fakePad) {
			app.visible = false
			app.kb.FlashCode, app.kb.FlashUntil = KEY_A, now.Add(150*time.Millisecond)
		}, maxIdleWait},
		{"two timers: the nearer one", func(app *App, _ *fakePad) {
			app.kb.FlashCode, app.kb.FlashUntil = KEY_A, now.Add(150*time.Millisecond)
			app.grabPending, app.grabPendingUntil = true, now.Add(300*time.Millisecond)
		}, 150 * time.Millisecond},
		{"two timers, other order", func(app *App, _ *fakePad) {
			app.kb.FlashCode, app.kb.FlashUntil = KEY_A, now.Add(300*time.Millisecond)
			app.grabPending, app.grabPendingUntil = true, now.Add(150*time.Millisecond)
		}, 150 * time.Millisecond},
		{"grab pending", func(app *App, _ *fakePad) {
			app.grabPending, app.grabPendingUntil = true, now.Add(300*time.Millisecond)
		}, 300 * time.Millisecond},
		{"pad lost", func(app *App, p *fakePad) {
			p.fd, app.reconnectLast = -1, now.Add(-1500*time.Millisecond)
		}, 500 * time.Millisecond},
		{"pad lost, reconnect due", func(_ *App, p *fakePad) { p.fd = -1 }, 16 * time.Millisecond},
	}
	for _, c := range cases {
		p := &fakePad{fd: 3}
		app := &App{config: cfg, visible: true, kb: NewKeyboardState(LayoutQWERTY), pad: p,
			now: func() time.Time { return now }}
		c.set(app, p)
		if got := app.idleWait(); got != c.want {
			t.Errorf("%s: idleWait %v, want %v", c.name, got, c.want)
		}
	}
}

// waitPad is a syncPad that can say when it has events, like GamepadReader.
type waitPad struct {
	*syncPad
	ready chan struct{}
	reads atomic.Int64
}

func (w *waitPad) Ready() (<-chan struct{}, bool) { return w.ready, true }
func (w *waitPad) ReadEvents() []Action {
	w.reads.Add(1)
	return w.syncPad.ReadEvents()
}

func startWaiting(t *testing.T, visible bool, setup ...func(app *App, wp *waitPad)) (*threadedLoop, *waitPad) {
	t.Helper()
	var wp *waitPad
	l := startThreaded(t, func(app *App) {
		wp = &waitPad{syncPad: app.pad.(*syncPad), ready: make(chan struct{}, 1)}
		app.pad = wp
		app.visible, app.shown = visible, visible
		for _, f := range setup {
			f(app, wp)
		}
	})
	return l, wp
}

// within polls cond every ms; false when d passed first.
func within(d time.Duration, cond func() bool) bool {
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(time.Millisecond) {
		if cond() {
			return true
		}
	}
	return cond()
}

// Hidden with a pad that says when it has events, the input side sleeps:
// a few passes a second, not one every 16 ms.
func TestHiddenInputSideSleeps(t *testing.T) {
	_, wp := startWaiting(t, false)
	time.Sleep(100 * time.Millisecond)
	before := wp.reads.Load()
	time.Sleep(500 * time.Millisecond)
	if n := wp.reads.Load() - before; n > 2 {
		t.Errorf("%d passes in 500 ms hidden; want at most 2", n)
	}
}

// Hidden, main pumps SDL events every hiddenIdle, not every 16 ms.
func TestHiddenMainSleeps(t *testing.T) {
	l, _ := startWaiting(t, false)
	time.Sleep(100 * time.Millisecond)
	before := l.win.polls.Load()
	time.Sleep(500 * time.Millisecond)
	if n := l.win.polls.Load() - before; n > 3 {
		t.Errorf("%d main passes in 500 ms hidden; want at most 3", n)
	}
}

// Shown, main still runs every 16 ms (SDL events, flash expiry, focus follow).
func TestShownMainPolls(t *testing.T) {
	l, _ := startWaiting(t, true)
	time.Sleep(100 * time.Millisecond)
	before := l.win.polls.Load()
	time.Sleep(500 * time.Millisecond)
	if n := l.win.polls.Load() - before; n < 15 {
		t.Errorf("%d main passes in 500 ms shown; want about 31", n)
	}
}

// The toggle combo, read on the input side itself, shows the keyboard at
// once while the buttons are still held (no further pad events).
func TestComboShowsAtOnce(t *testing.T) {
	l, wp := startWaiting(t, false)
	time.Sleep(100 * time.Millisecond)
	l.pad.send(Action{Type: ActionToggle})
	notify(wp.ready)
	if !within(100*time.Millisecond, func() bool { shows, _, _ := l.win.counts(); return shows > 0 }) {
		t.Fatal("the keyboard did not show within 100 ms of the combo")
	}
}

// Close in daemon mode hides the keyboard at once.
func TestDaemonCloseHidesAtOnce(t *testing.T) {
	l, wp := startWaiting(t, true, func(app *App, _ *waitPad) { app.daemon = true })
	time.Sleep(100 * time.Millisecond)
	l.pad.send(Action{Type: ActionClose})
	notify(wp.ready)
	if !within(100*time.Millisecond, func() bool { _, hides, _ := l.win.counts(); return hides > 0 }) {
		t.Fatal("the keyboard did not hide within 100 ms of Close")
	}
}

// While a stick is off center the input side polls every 4 ms: a pad that
// reports every ms does not get a pass per report.
func TestStickPollsNotPerReport(t *testing.T) {
	_, wp := startWaiting(t, true, func(_ *App, wp *waitPad) { wp.p.polling = true })
	time.Sleep(50 * time.Millisecond)
	before := wp.reads.Load()
	for end := time.Now().Add(200 * time.Millisecond); time.Now().Before(end); time.Sleep(time.Millisecond) {
		notify(wp.ready)
	}
	if n := wp.reads.Load() - before; n > 80 || n < 20 {
		t.Errorf("%d passes in 200 ms with reports every ms; want about 50 (4 ms poll)", n)
	}
}

// A pad event wakes the input side at once.
func TestPadEventWakesInputSide(t *testing.T) {
	l, wp := startWaiting(t, true)
	time.Sleep(100 * time.Millisecond)
	before := wp.reads.Load()
	start := time.Now()
	l.pad.send(Action{Type: ActionNavigate, DX: 1})
	notify(wp.ready)
	for wp.reads.Load() == before {
		if time.Since(start) > 100*time.Millisecond {
			t.Fatal("the input side did not read the pad within 100 ms of an event")
		}
		time.Sleep(time.Millisecond)
	}
}

// An IPC toggle wakes the hidden input side at once.
func TestToggleWakesInputSide(t *testing.T) {
	l, _ := startWaiting(t, false)
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	l.app.requestToggle() // as the IPC toggle
	for {
		if shows, _, _ := l.win.counts(); shows > 0 {
			break
		}
		if time.Since(start) > 100*time.Millisecond {
			t.Fatal("the keyboard did not show within 100 ms of a toggle")
		}
		time.Sleep(time.Millisecond)
	}
}

// stop (SIGTERM) wakes both loops itself: main may be busy in a slow draw
// and unable to pass the wake on.
func TestStopWakesBothLoops(t *testing.T) {
	app := &App{wake: make(chan struct{}, 1), inputWake: make(chan struct{}, 1)}
	app.running.Store(true)
	app.stop()
	if app.running.Load() {
		t.Error("still running")
	}
	for name, ch := range map[string]chan struct{}{"main": app.wake, "input side": app.inputWake} {
		select {
		case <-ch:
		default:
			t.Errorf("stop did not wake the %s", name)
		}
	}
}

// Main quits (window closed while shown): the input side, asleep with no
// pad events, stops at once, not after maxIdleWait.
func TestWindowQuitStopsAtOnce(t *testing.T) {
	l, _ := startWaiting(t, true)
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	l.win.quit.Store(true)
	select {
	case <-l.stopped:
	case <-time.After(time.Second):
		t.Fatal("the app did not stop")
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("quit took %v; want at once", d)
	}
}

// The stop (SIGTERM) wakes both hidden loops at once.
func TestStopWakesInputSide(t *testing.T) {
	l, _ := startWaiting(t, false)
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	l.stop()
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("stop took %v; want at once, not after hiddenIdle or maxIdleWait", d)
	}
}
