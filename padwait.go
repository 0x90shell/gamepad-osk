package main

import (
	"os"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// padWatch tells the input side when the pad has events, so it can sleep
// between them instead of polling. One watcher goroutine per open fd, parked
// in Go's netpoller (no thread of its own).
type padWatch struct {
	ready chan struct{} // the fd has events (or an error) to read
	cur   *watchRun     // the watcher of the open fd; nil when none
}

// watchRun is one watcher's own state, so the watcher of a closed fd cannot
// mark the next fd's watch broken or take its drained signal.
type watchRun struct {
	drained chan struct{} // the input side read until EAGAIN
	stop    chan struct{} // closed just before the fd is closed
	broken  atomic.Bool   // the fd cannot be watched: poll instead
}

func newPadWatch() *padWatch {
	return &padWatch{ready: make(chan struct{}, 1)}
}

func notify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// start watches f until end is called for it.
func (w *padWatch) start(f *os.File) {
	run := &watchRun{drained: make(chan struct{}, 1), stop: make(chan struct{})}
	w.cur = run
	go w.watch(f, run)
}

// end stops the watcher; call it before closing the fd.
func (w *padWatch) end() {
	if w.cur != nil {
		close(w.cur.stop)
		w.cur = nil
	}
}

// drainedAll: the input side read the fd until EAGAIN.
func (w *padWatch) drainedAll() {
	if w.cur != nil {
		notify(w.cur.drained)
	}
}

// fail makes Ready report nothing to wait on, so the input side polls.
func (w *padWatch) fail() {
	if w.cur != nil {
		w.failRun(w.cur)
	}
}

func (w *padWatch) failRun(run *watchRun) {
	run.broken.Store(true)
	notify(w.ready)
}

// broken: the open fd cannot be watched.
func (w *padWatch) broken() bool {
	return w.cur != nil && w.cur.broken.Load()
}

func (w *padWatch) watch(f *os.File, run *watchRun) {
	// A regular file always polls readable: watching it would spin
	if fi, err := f.Stat(); err != nil || fi.Mode().IsRegular() {
		w.failRun(run)
		return
	}
	rc, err := f.SyscallConn()
	if err != nil {
		w.failRun(run)
		return
	}
	for {
		// RawConn.Read resets the netpoller's ready flag, then calls the
		// func: a check of the fd itself, so events that came before the
		// reset are not lost. false parks until the next edge.
		err := rc.Read(fdReadable)
		select {
		case <-run.stop:
			return
		default:
		}
		if err != nil {
			w.failRun(run)
			return
		}
		notify(w.ready)
		select {
		case <-run.drained:
		case <-run.stop:
			return
		}
	}
}

// fdReadable: the fd has data, or an error or hangup for read to find.
func fdReadable(fd uintptr) bool {
	pfd := struct {
		fd             int32
		events, revent int16
	}{int32(fd), 0x1, 0} //nolint:gosec // G115: fd fits in int32; 0x1 is POLLIN
	var ts syscall.Timespec
	for {
		n, _, errno := syscall.Syscall6(syscall.SYS_PPOLL,
			uintptr(unsafe.Pointer(&pfd)), 1, //nolint:gosec // G103: ppoll takes a pollfd array
			uintptr(unsafe.Pointer(&ts)), 0, 0, 0) //nolint:gosec // G103: and a timespec
		if errno != syscall.EINTR {
			return errno != 0 || n > 0
		}
	}
}
