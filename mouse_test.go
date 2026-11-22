package main

import (
	"encoding/binary"
	"math"
	"os"
	"testing"
	"time"
)

// padWithEvents returns a reader whose fd is a regular file holding the given
// evdev events. ReadEvents reads them and then hits end of file. A pipe would
// hang: ReadEvents calls fd.Fd(), which puts the pipe in blocking mode, and
// the next read waits forever.
func padWithEvents(t *testing.T, sensitivity int, evs ...[3]int32) *GamepadReader {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Mouse.Sensitivity = sensitivity
	gp := NewGamepadReader(cfg)
	gp.initButtonMap()
	f, err := os.CreateTemp(t.TempDir(), "pad")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	for _, ev := range evs {
		var buf [24]byte
		binary.LittleEndian.PutUint16(buf[16:], uint16(ev[0])) //nolint:gosec // G115: test values are small
		binary.LittleEndian.PutUint16(buf[18:], uint16(ev[1])) //nolint:gosec // G115: test values are small
		binary.LittleEndian.PutUint32(buf[20:], uint32(ev[2])) //nolint:gosec // G115: test values are small
		if _, err := f.Write(buf[:]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	gp.fd = f
	now := time.Unix(0, 0)
	gp.clock = func() time.Time { now = now.Add(mouseStep); return now } // one step per pass
	return gp
}

// mousePixels pushes ABS_RX to the given fraction of full deflection and
// returns the DX of the first loop pass.
func mousePixels(t *testing.T, sensitivity int, push float64) int {
	t.Helper()
	return mouseTotal(t, sensitivity, push, 1)
}

// mouseTotal is the pixels moved over n loop passes with the stick held.
func mouseTotal(t *testing.T, sensitivity int, push float64, n int) int {
	t.Helper()
	gp := padWithEvents(t, sensitivity, [3]int32{evdevEV_ABS, ABS_RX, int32(push * 32767)})
	total := 0
	for range n {
		for _, a := range gp.ReadEvents() {
			if a.Type == ActionMouseMove {
				total += a.DX
			}
		}
	}
	return total
}

// U30: over many passes the pointer moves push * sensitivity pixels per pass
// on average: the fraction of a pixel carries to the next pass (fixed
// 2026-11-21: before, each pass dropped it).
func TestMousePixelsPerPass(t *testing.T) {
	cases := []struct {
		sens int
		push float64
	}{
		{10, 1.0}, {10, 0.55}, {50, 1.0}, {50, 0.65}, {4, 0.55}, {2, 0.6}, {2, 0.4}, {1, 1.0}, {1, 0.9}, {1, 0.3},
	}
	const n = 100
	for _, c := range cases {
		got := mouseTotal(t, c.sens, c.push, n)
		norm := float64(int32(c.push*32767)) / 32767
		want := int(norm * float64(c.sens) * n)
		if got < want-1 || got > want+1 {
			t.Errorf("sensitivity %d push %.2f: %d px in %d passes, want %d (within 1)", c.sens, c.push, got, n, want)
		}
	}
}

// U31: at the lowest settings any push past the deadzone moves the pointer
// within a few passes (sensitivity 1 at 30%: a pixel every 3 or 4 passes).
func TestMouseLowSensitivityStillMoves(t *testing.T) {
	for _, sens := range []int{1, 2, 3} {
		for _, push := range []float64{0.3, 0.5, 0.75, 1.0} {
			if got := mouseTotal(t, sens, push, 10); got < 1 {
				t.Errorf("sensitivity %d push %.2f: moved %d px in 10 passes, want at least 1", sens, push, got)
			}
		}
	}
}

// The carried fraction ends when the stick returns to center: no drift.
func TestMouseNoDriftAfterRelease(t *testing.T) {
	gp := padWithEvents(t, 1, [3]int32{evdevEV_ABS, ABS_RX, 29490}, [3]int32{evdevEV_ABS, ABS_RX, 0})
	gp.ReadEvents() // both events: pushed, then centered
	if gp.mouseRemX != 0 {
		t.Errorf("fraction %v kept after the stick centered", gp.mouseRemX)
	}
	for range 10 {
		for _, a := range gp.ReadEvents() {
			if a.Type == ActionMouseMove && a.DX != 0 {
				t.Fatalf("moved %d px with the stick centered", a.DX)
			}
		}
	}
}

// U32: the move grows with the push and with the sensitivity (never goes down).
func TestMouseMonotonic(t *testing.T) {
	prev := -1
	for _, push := range []float64{0.3, 0.5, 0.75, 1.0} {
		first := mousePixels(t, 20, push)
		if first < prev {
			t.Errorf("push %.2f gave %d px, less than %d at a smaller push", push, first, prev)
		}
		prev = first
	}
	prev = -1
	for _, sens := range []int{1, 2, 5, 10, 25, 50} {
		first := mousePixels(t, sens, 1.0)
		if first < prev {
			t.Errorf("sensitivity %d gave %d px, less than %d at a lower setting", sens, first, prev)
		}
		prev = first
	}
}

// U33: inside the deadzone nothing moves; mouse disabled moves nothing; a negative
// push moves the other way with the same size.
func TestMouseDeadzoneDisabledAndSign(t *testing.T) {
	if first := mousePixels(t, 10, 0.1); first != 0 {
		t.Errorf("push inside the deadzone moved %d px", first)
	}
	gp := padWithEvents(t, 10, [3]int32{evdevEV_ABS, ABS_RX, 32767})
	gp.config.Mouse.Enabled = false
	for _, a := range gp.ReadEvents() {
		if a.Type == ActionMouseMove {
			t.Errorf("mouse disabled but got a move %+v", a)
		}
	}
	pos := mousePixels(t, 10, 0.8)
	neg := mousePixels(t, 10, -0.8)
	if pos <= 0 || neg >= 0 || pos != -neg-1 && pos != -neg {
		t.Errorf("push +0.8 gave %d px, push -0.8 gave %d px, want opposite signs and the same size", pos, neg)
	}
}

// U34: SetSensitivity changes the next pass at once.
func TestMouseSetSensitivityTakesEffect(t *testing.T) {
	gp := padWithEvents(t, 10, [3]int32{evdevEV_ABS, ABS_RX, 32767})
	gp.ReadEvents()
	gp.SetSensitivity(30)
	var dx int
	for _, a := range gp.ReadEvents() {
		if a.Type == ActionMouseMove {
			dx = a.DX
		}
	}
	if dx != 30 {
		t.Errorf("after SetSensitivity(30) the next pass moved %d px, want 30", dx)
	}
}

// One axis centered while the other still moves: no leftover fraction moves
// the centered axis.
func TestMouseNoDriftOneAxis(t *testing.T) {
	gp := padWithEvents(t, 1, [3]int32{evdevEV_ABS, ABS_RX, 29490}, [3]int32{evdevEV_ABS, ABS_RY, 29490})
	gp.ReadEvents() // X and Y pushed 90%: each keeps a 0.9 px fraction
	if gp.mouseRemX == 0 {
		t.Fatal("no fraction carried on X: the test setup is wrong")
	}
	more, err := os.CreateTemp(t.TempDir(), "pad")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = more.Close() })
	var buf [24]byte
	binary.LittleEndian.PutUint16(buf[16:], evdevEV_ABS)
	binary.LittleEndian.PutUint16(buf[18:], ABS_RX) // X back to center
	if _, err := more.Write(buf[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := more.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	gp.fd = more
	dx := 0
	for range 10 {
		for _, a := range gp.ReadEvents() {
			if a.Type == ActionMouseMove {
				dx += a.DX
			}
		}
	}
	if dx != 0 {
		t.Errorf("X centered, Y still pushed: X moved %d px", dx)
	}
}

// pushedPad is a reader with an axis held at push (negative: the other way)
// and a clock that moves only when the test says.
func pushedPad(t *testing.T, sensitivity int, axis uint16, push float64) (*GamepadReader, *time.Time) {
	t.Helper()
	gp := padWithEvents(t, sensitivity, [3]int32{evdevEV_ABS, int32(axis), int32(push * 32767)})
	now := time.Unix(0, 0)
	gp.clock = func() time.Time { return now }
	return gp, &now
}

// passMove is the pointer movement of one pass.
func passMove(gp *GamepadReader) (dx, dy int) {
	for _, a := range gp.ReadEvents() {
		if a.Type == ActionMouseMove {
			dx += a.DX
			dy += a.DY
		}
	}
	return dx, dy
}

// U72: the pointer speed does not follow the poll rate. One second of a held
// stick moves the same distance polled every 4 ms (the input goroutine with
// the stick held) or every 16 ms (one pass per frame): 60 steps of
// sensitivity * push (fixed 2026-11-22: it moved one step per pass, so 4x
// faster once the pad had its own goroutine).
func TestMouseSpeedIgnoresPollRate(t *testing.T) {
	for _, every := range []time.Duration{4 * time.Millisecond, 16 * time.Millisecond, time.Second / 60} {
		for _, c := range []struct {
			sens int
			axis uint16
			push float64
		}{{10, ABS_RX, 1.0}, {10, ABS_RX, 0.5}, {1, ABS_RX, 0.3}, {50, ABS_RX, 0.8}, {10, ABS_RY, -0.7}, {50, ABS_RY, 1.0}} {
			gp, now := pushedPad(t, c.sens, c.axis, c.push)
			moved := func() int {
				dx, dy := passMove(gp)
				if c.axis == ABS_RY {
					return dy
				}
				return dx
			}
			total := moved() // the first pass: one step
			for elapsed := every; elapsed <= time.Second; elapsed += every {
				*now = now.Add(every)
				total += moved()
			}
			step := float64(int32(c.push*32767)) / 32767 * float64(c.sens)
			want := step * 61 // the first step, then 60 a second
			// up to half a step short (16 ms polls end at 992 ms), plus a pixel of rounding
			if d := float64(total) - want; d < -0.5*math.Abs(step)-1 || d > 0.5*math.Abs(step)+1 {
				t.Errorf("polled every %v, sensitivity %d axis %d push %.1f: %d px in 1 s, want %.0f",
					every, c.sens, c.axis, c.push, total, want)
			}
		}
	}
}

// A pass after a stall (the system was busy) moves exactly maxMouseSteps (4)
// steps: the pointer neither jumps nor stops.
func TestMouseStallDoesNotJump(t *testing.T) {
	if maxMouseSteps != 4 {
		t.Fatalf("maxMouseSteps is %v: check the stall values below", maxMouseSteps)
	}
	gp, now := pushedPad(t, 10, ABS_RX, 1.0)
	passMove(gp)
	*now = now.Add(time.Second)
	if dx, _ := passMove(gp); dx != 40 {
		t.Errorf("one pass after a 1 s stall moved %d px, want 4 steps (40)", dx)
	}
}

// Pushed again after centering, the first pass moves one step: the time
// spent centered does not count.
func TestMouseRestartIsOneStep(t *testing.T) {
	gp, now := pushedPad(t, 10, ABS_RX, 1.0)
	passMove(gp)
	gp.mouseX = 0 // centered
	*now = now.Add(time.Second)
	passMove(gp)
	if !gp.mouseLast.IsZero() {
		t.Error("the last move time is kept while the stick is centered")
	}
	gp.mouseX = 1 // full push again, 2 s after the last move
	*now = now.Add(time.Second)
	if dx, _ := passMove(gp); dx != 10 {
		t.Errorf("first pass after a new push moved %d px, want one step (10)", dx)
	}
}
