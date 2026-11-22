package main

/*
#cgo pkg-config: x11
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <X11/Xatom.h>
#include <string.h>
#include <stdlib.h>

// Store the previously focused window so we can restore focus, and the
// window the WM called active then (the game's client window).
static Window prev_focused = 0;
static Window prev_active = 0;

// The windows open at the save (_NET_CLIENT_LIST), to tell a program started
// since (a game launched after the keyboard) from one that was already open.
#define MAX_SAVED_CLIENTS 512
static Window saved_clients[MAX_SAVED_CLIENTS];
static int n_saved_clients = -1; // -1: no list (no EWMH window manager)

static Window read_active_window(Display *dpy);
static void save_client_list(Display *dpy);

void save_focused_window() {
    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return;
    int revert;
    XGetInputFocus(dpy, &prev_focused, &revert);
    prev_active = read_active_window(dpy);
    save_client_list(dpy);
    XCloseDisplay(dpy);
}

static int x11_suppress_focus_errors(Display *dpy, XErrorEvent *ev) {
    (void)dpy; (void)ev;
    // XSetInputFocus can only produce BadMatch (window not viewable, e.g. Wine
    // fullscreen) or BadWindow (window destroyed or invalid). Both are expected
    // when restoring focus to a window saved at startup. Reads of other
    // apps' windows use it too: those windows can close at any time.
    return 0;
}

static int window_viewable(Display *dpy, Window w);
static void request_activate(Display *dpy, Window w);

void restore_focus() {
    if (prev_focused == 0) return;
    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return;
    XErrorHandler prev = XSetErrorHandler(x11_suppress_focus_errors);
    // A game that lost focus to a click may have minimized itself (SDL does
    // in exclusive fullscreen): X refuses focus to an unmapped window, so ask
    // the window manager to activate it, as Alt+Tab or a taskbar click would.
    if (prev_active != 0 && !window_viewable(dpy, prev_active)) {
        request_activate(dpy, prev_active);
        XSync(dpy, False);
        XSetErrorHandler(prev);
        XCloseDisplay(dpy);
        return;
    }
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

static Window find_toplevel(Display *dpy, Window w);

// Check if a specific window has _NET_WM_STATE_FULLSCREEN.
// Returns 1 if fullscreen, 0 otherwise.
static int check_window_fullscreen(Display *dpy, Window w) {
    if (w == 0) return 0;

    Atom net_wm_state = XInternAtom(dpy, "_NET_WM_STATE", True);
    Atom fullscreen = XInternAtom(dpy, "_NET_WM_STATE_FULLSCREEN", True);
    if (net_wm_state == None || fullscreen == None) return 0;

    Atom type_ret;
    int fmt_ret;
    unsigned long nitems, bytes_left;
    unsigned char *data = NULL;

    if (XGetWindowProperty(dpy, w, net_wm_state, 0, 32, False, XA_ATOM,
            &type_ret, &fmt_ret, &nitems, &bytes_left, &data) != Success
            || data == NULL) {
        if (data) XFree(data);
        return 0;
    }

    int found = 0;
    Atom *atoms = (Atom *)data;
    for (unsigned long i = 0; i < nitems; i++) {
        if (atoms[i] == fullscreen) { found = 1; break; }
    }
    XFree(data);
    return found;
}

// Check if the currently focused window has _NET_WM_STATE_FULLSCREEN.
// Returns 1 if fullscreen, 0 otherwise. Safe on non-EWMH WMs.
int is_fullscreen_active() {
    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return 0;

    Window root = DefaultRootWindow(dpy);

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

    // The active window can close between the two reads (BadWindow): with
    // Xlib's default handler that error ends the process
    XErrorHandler prev = XSetErrorHandler(x11_suppress_focus_errors);
    int result = check_window_fullscreen(dpy, active);
    XSync(dpy, False);
    XSetErrorHandler(prev);
    XCloseDisplay(dpy);
    return result;
}

// Check if the saved (prev_focused) window has _NET_WM_STATE_FULLSCREEN.
// Also checks _NET_ACTIVE_WINDOW as fallback (prev_focused sub-window may not
// resolve to the correct toplevel for all apps).
// Uses error handler to survive stale window IDs.
int is_saved_window_fullscreen(unsigned long *out_saved, unsigned long *out_top,
                                unsigned long *out_active, int *out_method) {
    *out_saved = 0; *out_top = 0; *out_active = 0; *out_method = 0;
    if (prev_focused == 0) return 0;
    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return 0;

    XErrorHandler prev_handler = XSetErrorHandler(x11_suppress_focus_errors);

    *out_saved = (unsigned long)prev_focused;
    Window top = find_toplevel(dpy, prev_focused);
    *out_top = (unsigned long)top;
    XSync(dpy, False); // flush errors from find_toplevel

    // Method 1: check saved window's toplevel
    if (top != 0 && check_window_fullscreen(dpy, top)) {
        *out_method = 1;
        XSetErrorHandler(prev_handler);
        XCloseDisplay(dpy);
        return 1;
    }

    // Method 2: the window that was active at show (the game's client
    // window). A window manager puts it in a frame, so the toplevel above has
    // no state; and after a stray click it is no longer the active window.
    // (Not the active window now: that may be another program, 2026-11-22.)
    *out_active = (unsigned long)prev_active;
    if (prev_active != 0 && check_window_fullscreen(dpy, prev_active)) {
        *out_method = 2;
        XSetErrorHandler(prev_handler);
        XCloseDisplay(dpy);
        return 1;
    }

    XSetErrorHandler(prev_handler);
    XCloseDisplay(dpy);
    return 0;
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

static int window_viewable(Display *dpy, Window w) {
    XWindowAttributes attr;
    if (!XGetWindowAttributes(dpy, w, &attr)) return 1; // gone: nothing to activate
    return attr.map_state == IsViewable;
}

// EWMH: ask the window manager to activate (unminimize, raise, focus) w.
static void request_activate(Display *dpy, Window w) {
    XEvent ev;
    memset(&ev, 0, sizeof(ev));
    ev.xclient.type = ClientMessage;
    ev.xclient.window = w;
    ev.xclient.message_type = XInternAtom(dpy, "_NET_ACTIVE_WINDOW", False);
    ev.xclient.format = 32;
    ev.xclient.data.l[0] = 2; // source: a pager, so the WM does not refuse it
    ev.xclient.data.l[1] = CurrentTime;
    XSendEvent(dpy, DefaultRootWindow(dpy), False,
               SubstructureRedirectMask | SubstructureNotifyMask, &ev);
}

// The window _NET_ACTIVE_WINDOW names, 0 if none or no EWMH window manager.
static Window read_active_window(Display *dpy) {
    Atom net_active = XInternAtom(dpy, "_NET_ACTIVE_WINDOW", True);
    if (net_active == None) return 0;
    Atom type_ret;
    int fmt_ret;
    unsigned long nitems, bytes_left;
    unsigned char *data = NULL;
    Window active = 0;
    if (XGetWindowProperty(dpy, DefaultRootWindow(dpy), net_active, 0, 1, False, XA_WINDOW,
            &type_ret, &fmt_ret, &nitems, &bytes_left, &data) == Success && nitems >= 1 && data) {
        active = *(Window *)data;
    }
    if (data) XFree(data);
    return active;
}

static void save_client_list(Display *dpy) {
    n_saved_clients = -1;
    Atom list = XInternAtom(dpy, "_NET_CLIENT_LIST", True);
    if (list == None) return;
    Atom type_ret;
    int fmt_ret;
    unsigned long nitems, bytes_left;
    unsigned char *data = NULL;
    if (XGetWindowProperty(dpy, DefaultRootWindow(dpy), list, 0, MAX_SAVED_CLIENTS, False, XA_WINDOW,
            &type_ret, &fmt_ret, &nitems, &bytes_left, &data) == Success && data) {
        n_saved_clients = (int)nitems;
        for (unsigned long i = 0; i < nitems; i++) saved_clients[i] = ((Window *)data)[i];
    }
    if (data) XFree(data);
}

// 1 if the active window is a program started since the save: it was not
// open then. 0 for the saved one, one already open (a stray click), none.
int active_window_is_new() {
    if (n_saved_clients < 0) return 0;
    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return 0;
    Window active = read_active_window(dpy);
    XCloseDisplay(dpy);
    if (active == 0 || active == prev_active) return 0;
    for (int i = 0; i < n_saved_clients; i++) {
        if (saved_clients[i] == active) return 0;
    }
    return 1;
}

// Warp pointer to center of prev_focused top-level window, but only if the
// pointer is currently outside that window's geometry (e.g., drifted to another
// monitor via virtual mouse). If pointer is inside, leave it alone.
// Returns 1 if warped, 0 if no warp needed, -1 on error.
int x11_warp_if_outside(unsigned long *out_wid, int *out_x, int *out_y,
                         int *out_w, int *out_h, int *out_px, int *out_py) {
    *out_wid = 0; *out_x = 0; *out_y = 0; *out_w = 0; *out_h = 0; *out_px = 0; *out_py = 0;
    if (prev_focused == 0) return -1;
    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return -1;
    XErrorHandler prev_handler = XSetErrorHandler(x11_suppress_focus_errors);

    // The game: the window active at show, else the one that had focus
    Window top = find_toplevel(dpy, prev_active ? prev_active : prev_focused);
    XSync(dpy, False); // flush errors from find_toplevel
    *out_wid = (unsigned long)top;

    XWindowAttributes attr;
    if (!XGetWindowAttributes(dpy, top, &attr)) {
        XSetErrorHandler(prev_handler);
        XCloseDisplay(dpy);
        return -1;
    }
    *out_x = attr.x; *out_y = attr.y;
    *out_w = attr.width; *out_h = attr.height;

    // Query current pointer position (root coords)
    Window root_ret, child_ret;
    int root_x, root_y, win_x, win_y;
    unsigned int mask;
    if (!XQueryPointer(dpy, DefaultRootWindow(dpy), &root_ret, &child_ret,
                       &root_x, &root_y, &win_x, &win_y, &mask)) {
        XSetErrorHandler(prev_handler);
        XCloseDisplay(dpy);
        return -1;
    }
    *out_px = root_x; *out_py = root_y;

    // Check if pointer is inside the window
    if (root_x >= attr.x && root_x < attr.x + attr.width &&
        root_y >= attr.y && root_y < attr.y + attr.height) {
        XSetErrorHandler(prev_handler);
        XCloseDisplay(dpy);
        return 0; // inside, no warp needed
    }

    // Pointer is outside -- warp to center
    int wx = attr.width / 2;
    int wy = attr.height / 2;
    XWarpPointer(dpy, None, top, 0, 0, 0, 0, wx, wy);
    XSync(dpy, False);
    XSetErrorHandler(prev_handler);
    XCloseDisplay(dpy);
    return 1;
}

// Scan _NET_WM_STRUT_PARTIAL on all client windows to find panel reservations
// that overlap the given monitor region. Fallback for when _NET_WORKAREA spans
// multiple monitors without per-monitor panel subtraction (XFCE multi-monitor).
void get_strut_insets(int mon_x, int mon_y, int mon_w, int mon_h,
                      int *out_top, int *out_bottom, int *out_left, int *out_right) {
    *out_top = 0; *out_bottom = 0; *out_left = 0; *out_right = 0;

    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return;

    Window root = DefaultRootWindow(dpy);
    int screen_w = DisplayWidth(dpy, DefaultScreen(dpy));
    int screen_h = DisplayHeight(dpy, DefaultScreen(dpy));

    Atom client_list = XInternAtom(dpy, "_NET_CLIENT_LIST", True);
    Atom strut_partial = XInternAtom(dpy, "_NET_WM_STRUT_PARTIAL", True);
    if (client_list == None || strut_partial == None) { XCloseDisplay(dpy); return; }

    Atom type_ret;
    int fmt_ret;
    unsigned long nitems, bytes_left;
    unsigned char *data = NULL;

    if (XGetWindowProperty(dpy, root, client_list, 0, 1024, False, XA_WINDOW,
            &type_ret, &fmt_ret, &nitems, &bytes_left, &data) != Success
            || data == NULL) {
        if (data) XFree(data);
        XCloseDisplay(dpy);
        return;
    }

    // A client in the list can close while we read it (BadWindow)
    XErrorHandler prev = XSetErrorHandler(x11_suppress_focus_errors);
    Window *clients = (Window *)data;
    int mon_x2 = mon_x + mon_w;
    int mon_y2 = mon_y + mon_h;

    for (unsigned long i = 0; i < nitems; i++) {
        unsigned char *sd = NULL;
        Atom st; int sf; unsigned long sn, sb;

        if (XGetWindowProperty(dpy, clients[i], strut_partial, 0, 12, False, XA_CARDINAL,
                &st, &sf, &sn, &sb, &sd) != Success || sn < 12 || sd == NULL) {
            if (sd) XFree(sd);
            continue;
        }

        long *s = (long *)sd;
        // s[0..3] = left, right, top, bottom (px from screen edge)
        // s[4..5] = left y range, s[6..7] = right y range
        // s[8..9] = top x range, s[10..11] = bottom x range

        if (s[3] > 0 && (int)s[10] < mon_x2 && (int)s[11] >= mon_x) {
            int panel_top = screen_h - (int)s[3];
            int inset = mon_y2 - panel_top;
            if (inset > 0 && inset > *out_bottom) *out_bottom = inset;
        }
        if (s[2] > 0 && (int)s[8] < mon_x2 && (int)s[9] >= mon_x) {
            int inset = (int)s[2] - mon_y;
            if (inset > 0 && inset > *out_top) *out_top = inset;
        }
        if (s[0] > 0 && (int)s[4] < mon_y2 && (int)s[5] >= mon_y) {
            int inset = (int)s[0] - mon_x;
            if (inset > 0 && inset > *out_left) *out_left = inset;
        }
        if (s[1] > 0 && (int)s[6] < mon_y2 && (int)s[7] >= mon_y) {
            int panel_left = screen_w - (int)s[1];
            int inset = mon_x2 - panel_left;
            if (inset > 0 && inset > *out_right) *out_right = inset;
        }

        XFree(sd);
    }

    XSync(dpy, False);
    XSetErrorHandler(prev);
    XFree(data);
    XCloseDisplay(dpy);
}

void set_prev_focused_for_test(unsigned long wid) {
    prev_focused = (Window)wid;
}

// Test helper: set a WINDOW-type property on the root window of $DISPLAY.
void set_root_windows_for_test(const char *prop, unsigned long *ids, int n) {
    Display *dpy = XOpenDisplay(NULL);
    if (!dpy) return;
    Atom a = XInternAtom(dpy, prop, False);
    XChangeProperty(dpy, DefaultRootWindow(dpy), a, XA_WINDOW, 32, PropModeReplace,
                    (unsigned char *)ids, n);
    XSync(dpy, False);
    XCloseDisplay(dpy);
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
	"unsafe"
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

// ActiveWindowIsNew reports whether the active window is a program started
// since the last SaveFocusedWindow (a game launched after the keyboard).
func ActiveWindowIsNew() bool {
	if !hasX11 {
		return false
	}
	return C.active_window_is_new() != 0
}

// FollowNewProgram saves the focus again when the active window is a program
// started since the last save (a game launched while the keyboard is up), so
// hide returns to it. A window that was already open (a stray click) does
// not count: hide undoes that. Reports whether it saved.
func FollowNewProgram() bool {
	if !ActiveWindowIsNew() {
		return false
	}
	SaveFocusedWindow()
	return true
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

func setRootWindowsForTest(prop string, ids ...uint64) {
	cs := C.CString(prop)
	defer C.free(unsafe.Pointer(cs))
	cids := make([]C.ulong, len(ids))
	for i, id := range ids {
		cids[i] = C.ulong(id) //nolint:gosec // G115: test helper, value is controlled
	}
	C.set_root_windows_for_test(cs, &cids[0], C.int(len(cids)))
}

// GetStrutInsets scans _NET_WM_STRUT_PARTIAL on all client windows to find
// panel reservations overlapping the given monitor. Fallback for when
// _NET_WORKAREA doesn't account for per-monitor panels (multi-monitor XFCE).
func GetStrutInsets(mon MonitorRect) (top, bottom, left, right int32) {
	if !hasX11 {
		return 0, 0, 0, 0
	}
	var ct, cb, cl, cr C.int
	C.get_strut_insets(C.int(mon.X), C.int(mon.Y), C.int(mon.W), C.int(mon.H),
		&ct, &cb, &cl, &cr)
	return int32(ct), int32(cb), int32(cl), int32(cr)
}

// IsFullscreenActive checks if the currently focused X11 window is fullscreen.
// Returns false on Wayland or non-EWMH window managers.
func IsFullscreenActive() bool {
	if !hasX11 {
		return false
	}
	return C.is_fullscreen_active() != 0
}

// IsSavedWindowFullscreen checks if the window saved by SaveFocusedWindow is
// fullscreen: its toplevel, then the window that was active at the save (the
// game inside a window manager frame). Survives stale window IDs.
func IsSavedWindowFullscreen() bool {
	if !hasX11 {
		return false
	}
	var saved, top, active C.ulong
	var method C.int
	result := C.is_saved_window_fullscreen(&saved, &top, &active, &method)
	if result != 0 {
		Debugf("Fullscreen check: saved=0x%x top=0x%x active=0x%x method=%d",
			uint64(saved), uint64(top), uint64(active), int(method))
	} else {
		Debugf("Fullscreen check: saved=0x%x top=0x%x active=0x%x -not fullscreen",
			uint64(saved), uint64(top), uint64(active))
	}
	return result != 0
}

// WarpPointerIfOutside warps the pointer to center of the previously focused
// window, but only if the pointer is currently outside that window's geometry.
// Returns true if the pointer was warped.
func WarpPointerIfOutside() bool {
	if !hasX11 {
		return false
	}
	var wid C.ulong
	var ox, oy, ow, oh, px, py C.int
	result := C.x11_warp_if_outside(&wid, &ox, &oy, &ow, &oh, &px, &py)
	switch result {
	case 1:
		Debugf("Pointer outside window 0x%x (%d,%d %dx%d), was at %d,%d -warped to center",
			uint64(wid), int(ox), int(oy), int(ow), int(oh), int(px), int(py))
		return true
	case 0:
		Debugf("Pointer at %d,%d inside window 0x%x (%d,%d %dx%d) -no warp",
			int(px), int(py), uint64(wid), int(ox), int(oy), int(ow), int(oh))
	default:
		Debugf("Pointer warp check failed (stale window or no display)")
	}
	return false
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
