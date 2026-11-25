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
	ready   chan struct{} // the fd has events (or an error) to read
	drained chan struct{} // the input side read until EAGAIN
	stop    chan struct{} // closed just before the fd is closed
	broken  atomic.Bool   // the fd cannot be watched: poll instead
}

func newPadWatch() *padWatch {
	return &padWatch{ready: make(chan struct{}, 1), drained: make(chan struct{}, 1)}
}

func notify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// start watches f until end is called for it.
func (w *padWatch) start(f *os.File) {
	w.stop = make(chan struct{})
	w.broken.Store(false)
	go w.watch(f, w.stop)
}

// end stops the watcher; call it before closing the fd.
func (w *padWatch) end() {
	if w.stop != nil {
		close(w.stop)
		w.stop = nil
	}
}

func (w *padWatch) watch(f *os.File, stop chan struct{}) {
	// A regular file always polls readable: watching it would spin
	if fi, err := f.Stat(); err != nil || fi.Mode().IsRegular() {
		w.fail()
		return
	}
	rc, err := f.SyscallConn()
	if err != nil {
		w.fail()
		return
	}
	for {
		// RawConn.Read resets the netpoller's ready flag, then calls the
		// func: a check of the fd itself, so events that came before the
		// reset are not lost. false parks until the next edge.
		err := rc.Read(fdReadable)
		select {
		case <-stop:
			return
		default:
		}
		if err != nil {
			w.fail()
			return
		}
		notify(w.ready)
		select {
		case <-w.drained:
		case <-stop:
			return
		}
	}
}

// fail makes Ready report nothing to wait on, so the input side polls.
func (w *padWatch) fail() {
	w.broken.Store(true)
	notify(w.ready)
}

// fdReadable: the fd has data, or an error or hangup for read to find.
func fdReadable(fd uintptr) bool {
	pfd := struct {
		fd             int32
		events, revent int16
	}{int32(fd), 0x1, 0} //nolint:gosec // G115: fd fits in int32; 0x1 is POLLIN
	var ts syscall.Timespec
	n, _, errno := syscall.Syscall6(syscall.SYS_PPOLL,
		uintptr(unsafe.Pointer(&pfd)), 1, //nolint:gosec // G103: ppoll takes a pollfd array
		uintptr(unsafe.Pointer(&ts)), 0, 0, 0) //nolint:gosec // G103: and a timespec
	return errno != 0 || n > 0
}
