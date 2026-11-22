package main

import (
	"bytes"
	"log"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// fakeMonitors makes GetPrimaryMonitor see the given displays (id: bounds,
// ok false: bounds cannot be read) and primary.
func fakeMonitors(t *testing.T, ids []uint32, primary uint32, bounds map[uint32]Rect) {
	t.Helper()
	d, p, b := sdlDisplays, sdlPrimaryDisplay, sdlDisplayBounds
	t.Cleanup(func() { sdlDisplays, sdlPrimaryDisplay, sdlDisplayBounds = d, p, b })
	sdlDisplays = func() []uint32 { return ids }
	sdlPrimaryDisplay = func() uint32 { return primary }
	sdlDisplayBounds = func(id uint32) (Rect, bool) { r, ok := bounds[id]; return r, ok }
}

// U80: the keyboard opens on the primary monitor, wherever it is; then the
// first display; with none, 1920x1080. The owner's desktop: three monitors,
// the primary in the middle at +1920+0.
func TestPrimaryMonitor(t *testing.T) {
	three := map[uint32]Rect{
		1: {X: 0, Y: 221, W: 1920, H: 1080},
		2: {X: 1920, Y: 0, W: 2560, H: 1440},
		3: {X: 4480, Y: 220, W: 1920, H: 1080},
	}
	for _, c := range []struct {
		name    string
		ids     []uint32
		primary uint32
		bounds  map[uint32]Rect
		want    MonitorRect
	}{
		{"primary in the middle", []uint32{1, 2, 3}, 2, three, MonitorRect{1920, 0, 2560, 1440}},
		{"primary on the right", []uint32{1, 2, 3}, 3, three, MonitorRect{4480, 220, 1920, 1080}},
		{"no primary: the first display", []uint32{3, 1, 2}, 0, three, MonitorRect{4480, 220, 1920, 1080}},
		{"primary unreadable: the first display", []uint32{1, 2}, 2,
			map[uint32]Rect{1: three[1]}, MonitorRect{0, 221, 1920, 1080}},
		{"first unreadable: the largest readable", []uint32{1, 3, 2}, 0,
			map[uint32]Rect{3: three[3], 2: three[2]}, MonitorRect{1920, 0, 2560, 1440}},
		{"none readable", []uint32{1, 2}, 0, nil, MonitorRect{0, 0, 1920, 1080}},
		{"no displays", nil, 0, nil, MonitorRect{0, 0, 1920, 1080}},
	} {
		t.Run(c.name, func(t *testing.T) {
			fakeMonitors(t, c.ids, c.primary, c.bounds)
			if got := GetPrimaryMonitor(); got != c.want {
				t.Errorf("%+v, want %+v", got, c.want)
			}
		})
	}
}

// struts returns a fake panel reader with fixed insets, and how often it was asked.
func struts(top, bottom, left, right int32) (func(MonitorRect) (int32, int32, int32, int32), *int) {
	n := 0
	return func(MonitorRect) (int32, int32, int32, int32) { n++; return top, bottom, left, right }, &n
}

// U81: the area the keyboard is placed in: the workarea cut to the monitor;
// the panels' struts only when the workarea left nothing out (the owner's
// desktop: one 6400x1440 workarea over three monitors, a 35 px bottom panel).
func TestPlacementArea(t *testing.T) {
	mon := MonitorRect{X: 1920, Y: 0, W: 2560, H: 1440}
	none, _ := struts(0, 0, 0, 0)
	panel, asked := struts(0, 35, 0, 0)
	for _, c := range []struct {
		name   string
		wa     MonitorRect
		ok     bool
		struts func(MonitorRect) (int32, int32, int32, int32)
		want   MonitorRect
	}{
		{"no workarea (Wayland, no EWMH)", MonitorRect{}, false, panel, mon},
		{"empty workarea", MonitorRect{X: 0, Y: 0, W: 0, H: 1440}, true, panel, mon},
		{"workarea leaves out a bottom panel", MonitorRect{X: 0, Y: 0, W: 6400, H: 1405}, true, panel,
			MonitorRect{X: 1920, Y: 0, W: 2560, H: 1405}},
		{"workarea leaves out a top bar (GNOME)", MonitorRect{X: 1920, Y: 32, W: 2560, H: 1408}, true, panel,
			MonitorRect{X: 1920, Y: 32, W: 2560, H: 1408}},
		{"one workarea for all monitors: the struts", MonitorRect{X: 0, Y: 0, W: 6400, H: 1440}, true, panel,
			MonitorRect{X: 1920, Y: 0, W: 2560, H: 1405}},
		{"one workarea, no panels", MonitorRect{X: 0, Y: 0, W: 6400, H: 1440}, true, none, mon},
		{"workarea off this monitor: the monitor", MonitorRect{X: 0, Y: 0, W: 1920, H: 1080}, true, none, mon},
	} {
		if got := placementArea(mon, c.wa, c.ok, c.struts); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
	*asked = 0
	placementArea(mon, MonitorRect{X: 0, Y: 0, W: 6400, H: 1405}, true, panel)
	if *asked != 0 {
		t.Error("the struts were read although the workarea already left the panel out")
	}
	// The bench reads these lines (the last "effective:" is the area used).
	var out bytes.Buffer
	v := Verbose
	log.SetOutput(&out)
	Verbose = true
	placementArea(mon, MonitorRect{X: 0, Y: 0, W: 6400, H: 1440}, true, panel)
	Verbose = v
	log.SetOutput(os.Stderr)
	for _, want := range []string{"Workarea: 6400x1440+0+0, monitor: 2560x1440+1920+0, effective: 2560x1440+1920+0",
		"Strut fallback: top=0 bottom=35 left=0 right=0, effective: 2560x1405+1920+0"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("log has no %q: %q", want, out.String())
		}
	}
	sides, _ := struts(30, 40, 50, 60)
	got := placementArea(mon, MonitorRect{X: 0, Y: 0, W: 6400, H: 1440}, true, sides)
	if want := (MonitorRect{X: 1920 + 50, Y: 30, W: 2560 - 50 - 60, H: 1440 - 30 - 40}); got != want {
		t.Errorf("panels on every side: %+v, want %+v", got, want)
	}
}

// placedApp is an app placed on the owner's primary monitor, a 35 px bottom
// panel, margin 20, a 546 px high window; fullscreen says whether the focused
// window is fullscreen.
func placedApp(t *testing.T, top bool, fullscreen *bool) *App {
	t.Helper()
	f := fullscreenActive
	t.Cleanup(func() { fullscreenActive = f })
	fullscreenActive = func() bool { return *fullscreen }
	return &App{mon: MonitorRect{X: 1920, Y: 0, W: 2560, H: 1440}, posArea: MonitorRect{X: 1920, Y: 0, W: 2560, H: 1405},
		winH: 546, margin: 20, posTop: top, rend: &Renderer{}}
}

// U82: the Y of the window: margin px inside the area (the panel left out);
// over a fullscreen window (a game) margin px from the bare monitor edge.
func TestComputeY(t *testing.T) {
	for _, c := range []struct {
		top, fullscreen bool
		want            int32
	}{
		{false, false, 1405 - 546 - 20}, // above the panel
		{false, true, 1440 - 546 - 20},  // the game covers the panel
		{true, false, 20},
		{true, true, 20},
	} {
		fs := c.fullscreen
		if got := placedApp(t, c.top, &fs).computeY(); got != c.want {
			t.Errorf("top %v fullscreen %v: y %d, want %d", c.top, c.fullscreen, got, c.want)
		}
	}
	for _, c := range []struct {
		fullscreen bool
		want       int32
	}{{false, 52}, {true, 20}} { // a game covers the top bar too
		fs := c.fullscreen
		app := placedApp(t, true, &fs)
		app.posArea = MonitorRect{X: 1920, Y: 32, W: 2560, H: 1408} // a 32 px top bar
		if got := app.computeY(); got != c.want {
			t.Errorf("top, 32 px top bar, fullscreen %v: y %d, want %d", c.fullscreen, got, c.want)
		}
	}
}

// fakeMove records where togglePosition moves the window, on X11 or with
// layer shell, and the window's hide count at each move (the threaded test).
type fakeMove struct {
	layer     bool
	hides     func() int
	mu        sync.Mutex
	layerTops []bool
	hidesAt   []int
	tids      []int
	ys        []int32
}

func (m *fakeMove) anchors() []bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]bool(nil), m.layerTops...)
}

func fakeMoves(t *testing.T, layer bool) *fakeMove {
	t.Helper()
	m := &fakeMove{layer: layer}
	u, ml, mw := layerShellUp, moveLayer, moveWindow
	t.Cleanup(func() { layerShellUp, moveLayer, moveWindow = u, ml, mw })
	layerShellUp = func() bool { return m.layer }
	moveLayer = func(top bool, margin int32, _ *Window) {
		if margin != 20 {
			t.Errorf("layer moved with margin %d, want 20", margin)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		m.layerTops = append(m.layerTops, top)
		m.tids = append(m.tids, syscall.Gettid())
		if m.hides != nil {
			m.hidesAt = append(m.hidesAt, m.hides())
		}
	}
	moveWindow = func(_ *Window, x, y int32) {
		if x != 2562 {
			t.Errorf("window moved to x %d, want it centred at 2562", x)
		}
		m.ys = append(m.ys, y)
	}
	return m
}

// U83: Start (togglePosition) flips the side, says so, moves the window and
// saves the side: on X11 by Y, with layer shell by the anchor.
func TestTogglePosition(t *testing.T) {
	for _, layer := range []bool{false, true} {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		m := fakeMoves(t, layer)
		fs := false
		app := placedApp(t, false, &fs)
		app.winX = 2562
		for i, want := range []struct {
			top   bool
			flash string
			y     int32
		}{{true, "↑ TOP", 20}, {false, "↓ BOTTOM", 1405 - 546 - 20}} {
			app.togglePosition()
			if app.posTop != want.top || app.rend.flashText != want.flash {
				t.Errorf("layer %v, toggle %d: top %v flash %q, want %v %q", layer, i+1, app.posTop, app.rend.flashText, want.top, want.flash)
			}
			if v := userConfigValue(t, "position"); v != map[bool]string{true: "top", false: "bottom"}[want.top] {
				t.Errorf("layer %v, toggle %d: saved position %q", layer, i+1, v)
			}
			if layer {
				if len(m.layerTops) != i+1 || m.layerTops[i] != want.top || len(m.ys) != 0 {
					t.Errorf("layer shell, toggle %d: anchors %v, X11 moves %v", i+1, m.layerTops, m.ys)
				}
			} else if len(m.ys) != i+1 || m.ys[i] != want.y || app.winY != want.y || len(m.layerTops) != 0 {
				t.Errorf("X11, toggle %d: moves %v (winY %d), want y %d; layer moves %v", i+1, m.ys, app.winY, want.y, m.layerTops)
			}
		}
	}
}

// U84: Start in the running app (a job on main): twice quickly ends where it
// began, moved twice; while hidden it does nothing. Start, then a hide while
// main is busy drawing: both wait in the queue, and the move runs before the
// hide.
func TestPositionToggleThreaded(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := fakeMoves(t, true)
	l := startThreaded(t, func(app *App) { app.margin = 20 })
	m.hides = func() int { _, hides, _ := l.win.counts(); return hides }
	start := Action{Type: ActionPositionToggle}
	toggle := func() {
		l.app.lock.Lock()
		l.app.togglePending = true // as the IPC toggle does
		l.app.lock.Unlock()
	}
	queued := func() int {
		l.app.jobs.mu.Lock()
		defer l.app.jobs.mu.Unlock()
		return len(l.app.jobs.jobs)
	}

	l.pad.send(start, start)
	if !waitFor(func() bool { return len(m.anchors()) >= 2 }) {
		t.Fatalf("Start twice: anchors %v, want 2 moves", m.anchors())
	}
	time.Sleep(50 * time.Millisecond)
	if a := m.anchors(); len(a) != 2 || !a[0] || a[1] {
		t.Errorf("Start twice: anchors %v, want [top bottom]", a)
	}

	toggle() // hide
	if !waitFor(func() bool { return m.hides() == 1 }) {
		t.Fatal("the keyboard did not hide")
	}
	l.pad.send(start)
	time.Sleep(100 * time.Millisecond)
	if a := m.anchors(); len(a) != 2 {
		t.Errorf("Start while hidden moved the keyboard: anchors %v", a)
	}

	toggle() // show
	if !waitFor(func() bool { shows, _, _ := l.win.counts(); return shows == 1 }) {
		t.Fatal("the keyboard did not show again")
	}
	l.win.slowMs.Store(1500)                        // released below as soon as both jobs wait
	l.pad.send(Action{Type: ActionNavigate, DX: 1}) // a redraw, slow: main is busy in it
	if !waitFor(func() bool { return l.win.inDraw.Load() }) {
		t.Fatal("main never started the slow draw")
	}
	l.pad.send(start)
	if !waitFor(func() bool { return queued() == 1 }) {
		t.Fatalf("Start queued %d jobs while main was drawing, want 1", queued())
	}
	toggle() // hide, queued behind the move
	if !waitFor(func() bool { return queued() == 2 }) {
		t.Fatalf("%d jobs queued after Start and hide, want 2", queued())
	}
	l.win.slowMs.Store(0)
	if !waitFor(func() bool { return m.hides() == 2 && len(m.anchors()) == 3 }) {
		t.Fatalf("Start then hide: anchors %v, hides %d; want 3 moves and 2 hides", m.anchors(), m.hides())
	}
	m.mu.Lock()
	at, tids := m.hidesAt[2], m.tids
	m.mu.Unlock()
	if at != 1 {
		t.Errorf("the move ran after the hide (hides at the move: %d, want 1)", at)
	}
	for i, tid := range tids {
		if tid != l.win.mainTid {
			t.Errorf("move %d ran on thread %d, not main (%d)", i+1, tid, l.win.mainTid)
		}
	}
	l.stop()
	if v := userConfigValue(t, "position"); v != "top" {
		t.Errorf("saved position %q after three moves, want top", v)
	}
}
