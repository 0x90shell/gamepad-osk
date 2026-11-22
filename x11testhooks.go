package main

/*
#cgo pkg-config: x11
#include <X11/Xlib.h>
#include <X11/Xatom.h>

// For tests only (cgo cannot be used in _test.go files): a fake desktop on
// Xvfb. Windows live as long as this connection.
static Display *tdpy = NULL;

static Display *tdisplay(void) {
    if (!tdpy) tdpy = XOpenDisplay(NULL);
    return tdpy;
}

static unsigned long test_window(unsigned long parent, int x, int y, int w, int h) {
    Display *d = tdisplay();
    if (!d) return 0;
    Window p = parent ? (Window)parent : DefaultRootWindow(d);
    Window win = XCreateSimpleWindow(d, p, x, y, (unsigned)w, (unsigned)h, 0, 0, 0);
    XMapWindow(d, win);
    XSync(d, False);
    return win;
}

static void test_fullscreen(unsigned long win) {
    Display *d = tdisplay();
    Atom state = XInternAtom(d, "_NET_WM_STATE", False);
    Atom fs = XInternAtom(d, "_NET_WM_STATE_FULLSCREEN", False);
    XChangeProperty(d, (Window)win, state, XA_ATOM, 32, PropModeReplace, (unsigned char *)&fs, 1);
    XSync(d, False);
}

static void test_focus(unsigned long win) {
    Display *d = tdisplay();
    XSetInputFocus(d, (Window)win, RevertToPointerRoot, CurrentTime);
    XSync(d, False);
}

static unsigned long test_focused(void) {
    Display *d = tdisplay();
    Window w; int revert;
    XGetInputFocus(d, &w, &revert);
    return w;
}

// The EWMH atoms a window manager would have made (the app only looks them up).
static void test_atoms(void) {
    Display *d = tdisplay();
    XInternAtom(d, "_NET_WM_STATE", False);
    XInternAtom(d, "_NET_WM_STATE_FULLSCREEN", False);
    XInternAtom(d, "_NET_ACTIVE_WINDOW", False);
    XSync(d, False);
}

static void test_map(unsigned long win) {
    Display *d = tdisplay();
    XMapWindow(d, (Window)win);
    XSync(d, False);
}

static void test_unmap(unsigned long win) {
    Display *d = tdisplay();
    XUnmapWindow(d, (Window)win);
    XSync(d, False);
}

static void test_destroy(unsigned long win) {
    Display *d = tdisplay();
    XDestroyWindow(d, (Window)win);
    XSync(d, False);
}

// Act as the window manager: receive the requests clients send to the root.
static void test_watch_root(void) {
    Display *d = tdisplay();
    XSelectInput(d, DefaultRootWindow(d), SubstructureRedirectMask | SubstructureNotifyMask);
    XSync(d, False);
}

// 1 if a _NET_ACTIVE_WINDOW request for win arrived since the last call.
static int test_saw_activate(unsigned long win) {
    Display *d = tdisplay();
    Atom active = XInternAtom(d, "_NET_ACTIVE_WINDOW", False);
    int seen = 0;
    XSync(d, False);
    while (XPending(d)) {
        XEvent ev;
        XNextEvent(d, &ev);
        if (ev.type == ClientMessage && ev.xclient.message_type == active && ev.xclient.window == (Window)win) seen = 1;
    }
    return seen;
}

static void test_move_pointer(int x, int y) {
    Display *d = tdisplay();
    XWarpPointer(d, None, DefaultRootWindow(d), 0, 0, 0, 0, x, y);
    XSync(d, False);
}

static void test_pointer(int *x, int *y) {
    Display *d = tdisplay();
    Window r, c; int wx, wy; unsigned int m;
    XQueryPointer(d, DefaultRootWindow(d), &r, &c, x, y, &wx, &wy, &m);
}
*/
import "C"

// Test hooks: a fake desktop on Xvfb for the focus and pointer tests.

func testWindow(parent uint64, x, y, w, h int) uint64 {
	return uint64(C.test_window(C.ulong(parent), C.int(x), C.int(y), C.int(w), C.int(h)))
}

func testFullscreen(win uint64)       { C.test_fullscreen(C.ulong(win)) }
func testFocus(win uint64)            { C.test_focus(C.ulong(win)) }
func testFocused() uint64             { return uint64(C.test_focused()) }
func testDestroy(win uint64)          { C.test_destroy(C.ulong(win)) }
func testUnmap(win uint64)            { C.test_unmap(C.ulong(win)) }
func testWindowMap(win uint64)        { C.test_map(C.ulong(win)) }
func testAtoms()                      { C.test_atoms() }
func testWatchRoot()                  { C.test_watch_root() }
func testSawActivate(win uint64) bool { return C.test_saw_activate(C.ulong(win)) != 0 }
func testMovePointer(x, y int)        { C.test_move_pointer(C.int(x), C.int(y)) }

func testPointer() (x, y int) {
	var cx, cy C.int
	C.test_pointer(&cx, &cy)
	return int(cx), int(cy)
}
