package main

import (
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
	notify(w.drained)
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
	notify(w.drained) // nothing read
	if !waitReady(w, time.Second) {
		t.Fatal("no ready for data left unread")
	}
}

// end before close stops the watcher without making the pad unwatchable.
func TestPadWatchEndIsNotAFailure(t *testing.T) {
	w, pr, _ := watchedPipe(t)
	w.start(pr)
	time.Sleep(20 * time.Millisecond)
	w.end()
	_ = pr.Close()
	time.Sleep(50 * time.Millisecond)
	if w.broken.Load() {
		t.Error("a closed pad marked the watch broken")
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
	if !w.broken.Load() {
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

func TestIdleWait(t *testing.T) {
	now := time.Now()
	cfg := DefaultConfig()
	ValidateConfig(&cfg)
	near := func(got, want time.Duration) bool { return got <= want && got > want-5*time.Millisecond }
	cases := []struct {
		name string
		set  func(app *App, p *fakePad)
		want time.Duration
	}{
		{"idle", func(*App, *fakePad) {}, maxIdleWait},
		{"stick off center", func(_ *App, p *fakePad) { p.polling = true }, 4 * time.Millisecond},
		{"key repeat", func(app *App, _ *fakePad) {
			app.repeatAction, app.repeatInitial, app.repeatStart = ActionNavigate, false, now
			app.repeatLast = now
		}, 16 * time.Millisecond}, // as pollMs: repeat polls as before
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
		if got := app.idleWait(); !near(got, c.want) {
			t.Errorf("%s: idleWait %v, want about %v", c.name, got, c.want)
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

func startWaiting(t *testing.T, visible bool) (*threadedLoop, *waitPad) {
	t.Helper()
	var wp *waitPad
	l := startThreaded(t, func(app *App) {
		wp = &waitPad{syncPad: app.pad.(*syncPad), ready: make(chan struct{}, 1)}
		app.pad = wp
		app.visible, app.shown = visible, visible
	})
	return l, wp
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

// The stop wakes the hidden input side at once, not after maxIdleWait.
func TestStopWakesInputSide(t *testing.T) {
	l, _ := startWaiting(t, false)
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	l.stop()
	if d := time.Since(start); d > 500*time.Millisecond { // main notices within hiddenIdle
		t.Errorf("stop took %v; want well under maxIdleWait (%v)", d, maxIdleWait)
	}
}
