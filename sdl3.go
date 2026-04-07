package main

/*
#cgo pkg-config: sdl3
#include <SDL3/SDL.h>
#include <SDL3/SDL_video.h>
#include <SDL3/SDL_render.h>
#include <SDL3/SDL_events.h>
#include <SDL3/SDL_hints.h>
#include <SDL3/SDL_properties.h>
#include <stdlib.h>

// Helper to access event.type since "type" is a Go keyword
static Uint32 sdl_event_type(SDL_Event *e) { return e->type; }

// Helper to create a window with properties, avoiding Go/C string interop issues
// with SDL3's #define string constants.
static SDL_Window* create_window_props(const char *title, int x, int y, int w, int h,
    bool hidden, bool borderless, bool always_on_top, bool wayland_role_custom) {
    SDL_PropertiesID props = SDL_CreateProperties();
    if (!props) return NULL;

    SDL_SetStringProperty(props, SDL_PROP_WINDOW_CREATE_TITLE_STRING, title);
    SDL_SetNumberProperty(props, SDL_PROP_WINDOW_CREATE_X_NUMBER, x);
    SDL_SetNumberProperty(props, SDL_PROP_WINDOW_CREATE_Y_NUMBER, y);
    SDL_SetNumberProperty(props, SDL_PROP_WINDOW_CREATE_WIDTH_NUMBER, w);
    SDL_SetNumberProperty(props, SDL_PROP_WINDOW_CREATE_HEIGHT_NUMBER, h);

    if (hidden) SDL_SetBooleanProperty(props, SDL_PROP_WINDOW_CREATE_HIDDEN_BOOLEAN, true);
    if (borderless) SDL_SetBooleanProperty(props, SDL_PROP_WINDOW_CREATE_BORDERLESS_BOOLEAN, true);
    if (always_on_top) SDL_SetBooleanProperty(props, SDL_PROP_WINDOW_CREATE_ALWAYS_ON_TOP_BOOLEAN, true);
    if (wayland_role_custom) {
        SDL_SetBooleanProperty(props, SDL_PROP_WINDOW_CREATE_WAYLAND_SURFACE_ROLE_CUSTOM_BOOLEAN, true);
        SDL_SetBooleanProperty(props, SDL_PROP_WINDOW_CREATE_WAYLAND_CREATE_EGL_WINDOW_BOOLEAN, true);
    }

    SDL_Window *win = SDL_CreateWindowWithProperties(props);
    SDL_DestroyProperties(props);
    return win;
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// --- Init / Quit / Delay ---

func SDL3Init(flags uint32) error {
	if !C.SDL_Init(C.SDL_InitFlags(flags)) {
		return fmt.Errorf("SDL_Init: %s", C.GoString(C.SDL_GetError()))
	}
	return nil
}

func SDL3Quit() {
	C.SDL_Quit()
}

func SDL3Delay(ms uint32) {
	C.SDL_Delay(C.Uint32(ms))
}

// --- Window ---

func SDL3CreateWindow(title string, w, h int32, flags uint64) (*Window, error) {
	ct := C.CString(title)
	defer C.free(unsafe.Pointer(ct))
	win := C.SDL_CreateWindow(ct, C.int(w), C.int(h), C.SDL_WindowFlags(flags))
	if win == nil {
		return nil, fmt.Errorf("SDL_CreateWindow: %s", C.GoString(C.SDL_GetError()))
	}
	return &Window{ptr: unsafe.Pointer(win)}, nil
}

// SDL3CreateWindowWithProps creates a window using the properties API.
// This is required for Wayland roleless surfaces (layer-shell).
func SDL3CreateWindowWithProps(title string, x, y, w, h int32, hidden, borderless, alwaysOnTop, waylandRoleCustom bool) (*Window, error) {
	ct := C.CString(title)
	defer C.free(unsafe.Pointer(ct))
	win := C.create_window_props(ct, C.int(x), C.int(y), C.int(w), C.int(h),
		C.bool(hidden), C.bool(borderless), C.bool(alwaysOnTop), C.bool(waylandRoleCustom))
	if win == nil {
		return nil, fmt.Errorf("SDL_CreateWindowWithProperties: %s", C.GoString(C.SDL_GetError()))
	}
	return &Window{ptr: unsafe.Pointer(win)}, nil
}

func SDL3DestroyWindow(w *Window) {
	if w != nil && w.ptr != nil {
		C.SDL_DestroyWindow((*C.SDL_Window)(w.ptr))
	}
}

func SDL3ShowWindow(w *Window) {
	C.SDL_ShowWindow((*C.SDL_Window)(w.ptr))
}

func SDL3HideWindow(w *Window) {
	C.SDL_HideWindow((*C.SDL_Window)(w.ptr))
}

func SDL3RaiseWindow(w *Window) {
	C.SDL_RaiseWindow((*C.SDL_Window)(w.ptr))
}

func SDL3SetWindowPosition(w *Window, x, y int32) {
	C.SDL_SetWindowPosition((*C.SDL_Window)(w.ptr), C.int(x), C.int(y))
}

func SDL3GetWindowPosition(w *Window) (int32, int32) {
	var x, y C.int
	C.SDL_GetWindowPosition((*C.SDL_Window)(w.ptr), &x, &y)
	return int32(x), int32(y)
}

func SDL3SetWindowOpacity(w *Window, opacity float32) {
	C.SDL_SetWindowOpacity((*C.SDL_Window)(w.ptr), C.float(opacity))
}

func SDL3GetWindowProperties(w *Window) uint32 {
	return uint32(C.SDL_GetWindowProperties((*C.SDL_Window)(w.ptr)))
}

// --- Properties ---

func SDL3GetPointerProperty(props uint32, name string) unsafe.Pointer {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	return C.SDL_GetPointerProperty(C.SDL_PropertiesID(props), cn, nil)
}

func SDL3GetNumberProperty(props uint32, name string) int64 {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	return int64(C.SDL_GetNumberProperty(C.SDL_PropertiesID(props), cn, 0))
}

func SDL3GetGlobalProperties() uint32 {
	return uint32(C.SDL_GetGlobalProperties())
}

// --- Renderer ---

func SDL3CreateRenderer(w *Window) (*SDLRenderer, error) {
	r := C.SDL_CreateRenderer((*C.SDL_Window)(w.ptr), nil)
	if r == nil {
		return nil, fmt.Errorf("SDL_CreateRenderer: %s", C.GoString(C.SDL_GetError()))
	}
	return &SDLRenderer{ptr: unsafe.Pointer(r)}, nil
}

func SDL3SetRenderVSync(r *SDLRenderer, vsync int) {
	C.SDL_SetRenderVSync((*C.SDL_Renderer)(r.ptr), C.int(vsync))
}

func SDL3SetRenderDrawBlendMode(r *SDLRenderer, mode uint32) {
	C.SDL_SetRenderDrawBlendMode((*C.SDL_Renderer)(r.ptr), C.SDL_BlendMode(mode))
}

func SDL3GetRendererName(r *SDLRenderer) string {
	name := C.SDL_GetRendererName((*C.SDL_Renderer)(r.ptr))
	if name == nil {
		return "unknown"
	}
	return C.GoString(name)
}

func SDL3DestroyRenderer(r *SDLRenderer) {
	if r != nil && r.ptr != nil {
		C.SDL_DestroyRenderer((*C.SDL_Renderer)(r.ptr))
	}
}

func SDL3SetRenderDrawColor(r *SDLRenderer, c Color) {
	C.SDL_SetRenderDrawColor((*C.SDL_Renderer)(r.ptr), C.Uint8(c.R), C.Uint8(c.G), C.Uint8(c.B), C.Uint8(c.A))
}

func SDL3RenderClear(r *SDLRenderer) {
	C.SDL_RenderClear((*C.SDL_Renderer)(r.ptr))
}

func SDL3RenderPresent(r *SDLRenderer) {
	C.SDL_RenderPresent((*C.SDL_Renderer)(r.ptr))
}

func SDL3RenderFillRect(r *SDLRenderer, rect FRect) {
	cr := C.SDL_FRect{x: C.float(rect.X), y: C.float(rect.Y), w: C.float(rect.W), h: C.float(rect.H)}
	C.SDL_RenderFillRect((*C.SDL_Renderer)(r.ptr), &cr)
}

func SDL3RenderRect(r *SDLRenderer, rect FRect) {
	cr := C.SDL_FRect{x: C.float(rect.X), y: C.float(rect.Y), w: C.float(rect.W), h: C.float(rect.H)}
	C.SDL_RenderRect((*C.SDL_Renderer)(r.ptr), &cr)
}

func SDL3RenderTexture(r *SDLRenderer, tex *Texture, src, dst *FRect) {
	// Declare both rects at function scope to ensure pointer stability for cgo
	var srcRect, dstRect C.SDL_FRect
	var csrc, cdst *C.SDL_FRect
	if src != nil {
		srcRect = C.SDL_FRect{x: C.float(src.X), y: C.float(src.Y), w: C.float(src.W), h: C.float(src.H)}
		csrc = &srcRect
	}
	if dst != nil {
		dstRect = C.SDL_FRect{x: C.float(dst.X), y: C.float(dst.Y), w: C.float(dst.W), h: C.float(dst.H)}
		cdst = &dstRect
	}
	C.SDL_RenderTexture((*C.SDL_Renderer)(r.ptr), (*C.SDL_Texture)(tex.ptr), csrc, cdst)
}

func SDL3GetRenderOutputSize(r *SDLRenderer) (int32, int32) {
	var w, h C.int
	C.SDL_GetRenderOutputSize((*C.SDL_Renderer)(r.ptr), &w, &h)
	return int32(w), int32(h)
}

func SDL3CreateTextureFromSurface(r *SDLRenderer, s *Surface) *Texture {
	tex := C.SDL_CreateTextureFromSurface((*C.SDL_Renderer)(r.ptr), (*C.SDL_Surface)(s.ptr))
	if tex == nil {
		return nil
	}
	return &Texture{ptr: unsafe.Pointer(tex)}
}

func SDL3DestroyTexture(t *Texture) {
	if t != nil && t.ptr != nil {
		C.SDL_DestroyTexture((*C.SDL_Texture)(t.ptr))
	}
}

// --- Surface ---

func SDL3DestroySurface(s *Surface) {
	if s != nil && s.ptr != nil {
		C.SDL_DestroySurface((*C.SDL_Surface)(s.ptr))
	}
}

func SDL3SurfaceWidth(s *Surface) int32 {
	return int32((*C.SDL_Surface)(s.ptr).w)
}

func SDL3SurfaceHeight(s *Surface) int32 {
	return int32((*C.SDL_Surface)(s.ptr).h)
}

// --- Events ---

// SDL3PollEvent returns the event type and true if an event was available.
//nolint:gocritic // dupSubExpr: false positive from cgo generated code
func SDL3PollEvent() (uint32, bool) {
	var event C.SDL_Event
	if C.SDL_PollEvent(&event) {
		return uint32(C.sdl_event_type(&event)), true
	}
	return 0, false
}

//nolint:revive // Match SDL3 naming convention
const (
	SDL_EVENT_QUIT uint32 = 0x100
)

// --- Display ---

func SDL3GetPrimaryDisplay() uint32 {
	return uint32(C.SDL_GetPrimaryDisplay())
}

func SDL3GetDisplays() []uint32 {
	var count C.int
	ids := C.SDL_GetDisplays(&count)
	if ids == nil || count == 0 {
		return nil
	}
	defer C.SDL_free(unsafe.Pointer(ids))
	n := int(count)
	result := make([]uint32, n)
	slice := unsafe.Slice(ids, n)
	for i := range n {
		result[i] = uint32(slice[i])
	}
	return result
}

func SDL3GetDisplayBounds(displayID uint32) (Rect, bool) {
	var r C.SDL_Rect
	if C.SDL_GetDisplayBounds(C.SDL_DisplayID(displayID), &r) {
		return Rect{X: int32(r.x), Y: int32(r.y), W: int32(r.w), H: int32(r.h)}, true
	}
	return Rect{}, false
}

// --- Hints ---

func SDL3SetHint(name, value string) {
	cn := C.CString(name)
	cv := C.CString(value)
	defer C.free(unsafe.Pointer(cn))
	defer C.free(unsafe.Pointer(cv))
	C.SDL_SetHint(cn, cv)
}

func SDL3GetCurrentVideoDriver() string {
	d := C.SDL_GetCurrentVideoDriver()
	if d == nil {
		return ""
	}
	return C.GoString(d)
}

// --- Window flag constants ---

//nolint:revive // Match SDL3 naming convention
const (
	SDL_WINDOW_HIDDEN        uint64 = 0x0000000000000008
	SDL_WINDOW_BORDERLESS    uint64 = 0x0000000000000010
	SDL_WINDOW_ALWAYS_ON_TOP uint64 = 0x0000000000010000
)

// --- SDL_INIT constants ---

//nolint:revive // Match SDL3 naming convention
const (
	SDL_INIT_VIDEO uint32 = 0x00000020
)
