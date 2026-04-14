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

static int x11_suppress_focus_errors(Display *dpy, XErrorEvent *ev) {
    (void)dpy; (void)ev;
    // XSetInputFocus can only produce BadMatch (window not viewable, e.g. Wine
    // fullscreen) or BadWindow (window destroyed or invalid). Both are expected
    // when restoring focus to a window saved at startup.
    return 0;
}

void restore_focus() {
    if (prev_focused == 0) return;
    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return;
    XErrorHandler prev = XSetErrorHandler(x11_suppress_focus_errors);
    XSetInputFocus(dpy, prev_focused, RevertToParent, CurrentTime);
    XSync(dpy, False); // round-trip: flushes request and fires error handler before we restore it
    XSetErrorHandler(prev);
    XCloseDisplay(dpy);
}

// Get _NET_WORKAREA from root window. Returns 1 on success, 0 on failure.
// out_x, out_y, out_w, out_h are the usable desktop area (excludes panels).
int get_workarea(int *out_x, int *out_y, int *out_w, int *out_h) {
    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return 0;

    Window root = DefaultRootWindow(dpy);
    Atom wa = XInternAtom(dpy, "_NET_WORKAREA", True);
    if (wa == None) { XCloseDisplay(dpy); return 0; }

    Atom type_ret;
    int fmt_ret;
    unsigned long nitems, bytes_left;
    unsigned char *data = NULL;
    if (XGetWindowProperty(dpy, root, wa, 0, 4, False, XA_CARDINAL,
            &type_ret, &fmt_ret, &nitems, &bytes_left, &data) != Success
            || nitems < 4 || data == NULL) {
        if (data) XFree(data);
        XCloseDisplay(dpy);
        return 0;
    }
    long *vals = (long *)data;
    *out_x = (int)vals[0];
    *out_y = (int)vals[1];
    *out_w = (int)vals[2];
    *out_h = (int)vals[3];
    XFree(data);
    XCloseDisplay(dpy);
    return 1;
}

void set_prev_focused_for_test(unsigned long wid) {
    prev_focused = (Window)wid;
}

void set_no_focus_hints(unsigned long window_id) {
    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return;

    Window win = (Window)window_id;
    Window root = DefaultRootWindow(dpy);

    // Override-redirect bypasses the WM entirely: the window is never added to
    // _NET_CLIENT_LIST, never triggers _NET_ACTIVE_WINDOW changes, and never
    // causes xfce4-panel's intelligent autohide to re-appear over fullscreen
    // apps. Must be set before first map (SDL_ShowWindow). Persists across
    // hide/show cycles (XUnmapWindow/XMapRaised).
    XSetWindowAttributes oa;
    oa.override_redirect = True;
    XChangeWindowAttributes(dpy, win, CWOverrideRedirect, &oa);

    // _NET_WM_WINDOW_TYPE_NOTIFICATION: retained for semantic correctness. The
    // WM ignores hints on override-redirect windows, but some compositors still
    // read the type for effects (shadows, etc.).
    Atom wm_window_type = XInternAtom(dpy, "_NET_WM_WINDOW_TYPE", False);
    Atom type_notification = XInternAtom(dpy, "_NET_WM_WINDOW_TYPE_NOTIFICATION", False);
    XChangeProperty(dpy, win, wm_window_type, XA_ATOM, 32, PropModeReplace,
                    (unsigned char*)&type_notification, 1);

    Atom wm_state = XInternAtom(dpy, "_NET_WM_STATE", False);
    Atom above = XInternAtom(dpy, "_NET_WM_STATE_ABOVE", False);
    Atom skip_tb = XInternAtom(dpy, "_NET_WM_STATE_SKIP_TASKBAR", False);
    Atom skip_pg = XInternAtom(dpy, "_NET_WM_STATE_SKIP_PAGER", False);

    // Set _NET_WM_STATE directly on the window. Do NOT use XSendEvent to root
    // with SubstructureRedirectMask: xfwm4 processes ALL such client messages
    // (even for override-redirect windows), which triggers fullscreen stack
    // re-evaluation and causes xfce4-panel to re-appear. The WM cannot honor
    // EWMH state changes for override-redirect windows anyway.
    Atom atoms[3] = {above, skip_tb, skip_pg};
    XChangeProperty(dpy, win, wm_state, XA_ATOM, 32, PropModeReplace,
                    (unsigned char*)atoms, 3);

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
)

var hasX11 bool

func initX11Detection() {
	// Check after SDL init: if the video driver is wayland, skip X11 hints.
	// XWayland (SDL using x11 backend inside Sway) still needs X11 hints.
	driver := SDL3GetCurrentVideoDriver()
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

// GetWorkarea returns the usable desktop area from _NET_WORKAREA (excludes panels).
// Returns zero rect if not available (Wayland, or WM doesn't set it).
func GetWorkarea() (x, y, w, h int32, ok bool) {
	if !hasX11 {
		return 0, 0, 0, 0, false
	}
	var cx, cy, cw, ch C.int
	if C.get_workarea(&cx, &cy, &cw, &ch) == 0 {
		return 0, 0, 0, 0, false
	}
	return int32(cx), int32(cy), int32(cw), int32(ch), true
}

func setPrevFocusedForTest(id uint64) {
	C.set_prev_focused_for_test(C.ulong(id)) //nolint:gosec // G115: test helper, value is controlled
}

func SetNoFocusHints(window *Window) {
	if !hasX11 {
		return
	}
	props := SDL3GetWindowProperties(window)
	if props == 0 {
		log.Printf("Warning: cannot get window properties")
		return
	}
	xwin := SDL3GetNumberProperty(props, "SDL.window.x11.window")
	if xwin == 0 {
		log.Printf("Warning: no X11 window ID")
		return
	}
	C.set_no_focus_hints(C.ulong(xwin))
}
