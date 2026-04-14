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

// Check if the currently focused window has _NET_WM_STATE_FULLSCREEN.
// Returns 1 if fullscreen, 0 otherwise. Safe on non-EWMH WMs.
int is_fullscreen_active() {
    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return 0;

    Window root = DefaultRootWindow(dpy);

    // Get _NET_ACTIVE_WINDOW from root
    Atom net_active = XInternAtom(dpy, "_NET_ACTIVE_WINDOW", True);
    if (net_active == None) { XCloseDisplay(dpy); return 0; }

    Atom type_ret;
    int fmt_ret;
    unsigned long nitems, bytes_left;
    unsigned char *data = NULL;

    if (XGetWindowProperty(dpy, root, net_active, 0, 1, False, XA_WINDOW,
            &type_ret, &fmt_ret, &nitems, &bytes_left, &data) != Success
            || nitems < 1 || data == NULL) {
        if (data) XFree(data);
        XCloseDisplay(dpy);
        return 0;
    }
    Window active = *(Window *)data;
    XFree(data);

    if (active == 0 || active == root) { XCloseDisplay(dpy); return 0; }

    // Check _NET_WM_STATE for _FULLSCREEN
    Atom net_wm_state = XInternAtom(dpy, "_NET_WM_STATE", True);
    Atom fullscreen = XInternAtom(dpy, "_NET_WM_STATE_FULLSCREEN", True);
    if (net_wm_state == None || fullscreen == None) { XCloseDisplay(dpy); return 0; }

    data = NULL;
    if (XGetWindowProperty(dpy, active, net_wm_state, 0, 32, False, XA_ATOM,
            &type_ret, &fmt_ret, &nitems, &bytes_left, &data) != Success
            || data == NULL) {
        if (data) XFree(data);
        XCloseDisplay(dpy);
        return 0;
    }

    int found = 0;
    Atom *atoms = (Atom *)data;
    for (unsigned long i = 0; i < nitems; i++) {
        if (atoms[i] == fullscreen) { found = 1; break; }
    }
    XFree(data);
    XCloseDisplay(dpy);
    return found;
}

// Walk up from w to find the top-level window (direct child of root).
// XGetInputFocus may return a sub-window; we need the frame for correct geometry.
static Window find_toplevel(Display *dpy, Window w) {
    Window root = DefaultRootWindow(dpy);
    Window parent, *children;
    unsigned int nchildren;
    while (w != root) {
        if (!XQueryTree(dpy, w, &root, &parent, &children, &nchildren))
            return w;
        if (children) XFree(children);
        if (parent == root) return w;
        w = parent;
    }
    return w;
}

// Warp pointer to center of prev_focused top-level window.
// Resolves to top-level first (XGetInputFocus may return a sub-window).
// Error handler suppresses BadWindow if window was destroyed between save and warp.
// Returns the warp target info for logging: window ID, x, y, w, h, warp_x, warp_y.
void x11_warp_pointer_center(unsigned long *out_wid, int *out_x, int *out_y,
                              int *out_w, int *out_h, int *out_wx, int *out_wy) {
    *out_wid = 0; *out_x = 0; *out_y = 0; *out_w = 0; *out_h = 0; *out_wx = 0; *out_wy = 0;
    if (prev_focused == 0) return;
    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return;
    XErrorHandler prev_handler = XSetErrorHandler(x11_suppress_focus_errors);

    Window top = find_toplevel(dpy, prev_focused);
    *out_wid = (unsigned long)top;

    XWindowAttributes attr;
    if (XGetWindowAttributes(dpy, top, &attr)) {
        int wx = attr.width / 2;
        int wy = attr.height / 2;
        *out_x = attr.x; *out_y = attr.y;
        *out_w = attr.width; *out_h = attr.height;
        *out_wx = wx; *out_wy = wy;
        XWarpPointer(dpy, None, top, 0, 0, 0, 0, wx, wy);
    }
    XSync(dpy, False);
    XSetErrorHandler(prev_handler);
    XCloseDisplay(dpy);
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

// IsFullscreenActive checks if the currently focused X11 window is fullscreen.
// Returns false on Wayland or non-EWMH window managers.
func IsFullscreenActive() bool {
	if !hasX11 {
		return false
	}
	return C.is_fullscreen_active() != 0
}

// WarpPointerCenter warps the pointer to the center of the previously focused
// top-level window. Helps fullscreen games recover from pointer displacement.
func WarpPointerCenter() {
	if !hasX11 {
		return
	}
	var wid C.ulong
	var ox, oy, ow, oh, wx, wy C.int
	C.x11_warp_pointer_center(&wid, &ox, &oy, &ow, &oh, &wx, &wy)
	if wid != 0 {
		Debugf("WarpPointer: window=0x%x pos=%d,%d size=%dx%d warp=%d,%d",
			uint64(wid), int(ox), int(oy), int(ow), int(oh), int(wx), int(wy))
	}
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
