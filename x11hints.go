package main

/*
#cgo pkg-config: x11
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <X11/Xatom.h>
#include <string.h>

// Store the previously focused window so we can restore focus
static Window prev_focused = 0;

void save_focused_window() {
    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return;
    int revert;
    XGetInputFocus(dpy, &prev_focused, &revert);
    XCloseDisplay(dpy);
}

void restore_focus() {
    if (prev_focused == 0) return;
    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return;
    XSetInputFocus(dpy, prev_focused, RevertToParent, CurrentTime);
    XFlush(dpy);
    XCloseDisplay(dpy);
}

void set_no_focus_hints(unsigned long window_id) {
    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return;

    Window win = (Window)window_id;
    Window root = DefaultRootWindow(dpy);

    Atom wm_state = XInternAtom(dpy, "_NET_WM_STATE", False);
    Atom above = XInternAtom(dpy, "_NET_WM_STATE_ABOVE", False);
    Atom skip_tb = XInternAtom(dpy, "_NET_WM_STATE_SKIP_TASKBAR", False);
    Atom skip_pg = XInternAtom(dpy, "_NET_WM_STATE_SKIP_PAGER", False);

    // Set property for initial mapping
    Atom atoms[3] = {above, skip_tb, skip_pg};
    XChangeProperty(dpy, win, wm_state, XA_ATOM, 32, PropModeReplace,
                    (unsigned char*)atoms, 3);

    // Send client messages for already-mapped windows (survives hide/show)
    Atom state_atoms[3] = {above, skip_tb, skip_pg};
    for (int i = 0; i < 3; i++) {
        XEvent ev;
        memset(&ev, 0, sizeof(ev));
        ev.xclient.type = ClientMessage;
        ev.xclient.window = win;
        ev.xclient.message_type = wm_state;
        ev.xclient.format = 32;
        ev.xclient.data.l[0] = 1; // _NET_WM_STATE_ADD
        ev.xclient.data.l[1] = state_atoms[i];
        ev.xclient.data.l[2] = 0;
        ev.xclient.data.l[3] = 1; // source: application
        XSendEvent(dpy, root, False,
                   SubstructureRedirectMask | SubstructureNotifyMask, &ev);
    }

    // WM_HINTS: input = False
    XWMHints hints;
    memset(&hints, 0, sizeof(hints));
    hints.flags = InputHint;
    hints.input = False;
    XSetWMHints(dpy, win, &hints);

    XFlush(dpy);
    XCloseDisplay(dpy);
}
*/
import "C"

import (
	"log"

	"github.com/veandco/go-sdl2/sdl"
)

var hasX11 bool

func initX11Detection() {
	// Check after SDL init: if the video driver is wayland, skip X11 hints.
	// XWayland (SDL using x11 backend inside Sway) still needs X11 hints.
	driver, _ := sdl.GetCurrentVideoDriver()
	hasX11 = driver == "x11"
	if !hasX11 {
		log.Printf("Video driver: %s, skipping X11 window hints", driver)
	}
}

func SaveFocusedWindow() {
	if !hasX11 {
		return
	}
	C.save_focused_window()
}

func RestoreFocus() {
	if !hasX11 {
		return
	}
	C.restore_focus()
}

func SetNoFocusHints(window *sdl.Window) {
	if !hasX11 {
		return
	}
	info, err := window.GetWMInfo()
	if err != nil {
		log.Printf("Warning: cannot get WM info: %v", err)
		return
	}
	x11 := info.GetX11Info()
	if x11.Window == 0 {
		log.Printf("Warning: no X11 window ID")
		return
	}
	C.set_no_focus_hints(C.ulong(x11.Window))
}
