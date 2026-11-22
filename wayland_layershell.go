package main

/*
#cgo pkg-config: wayland-client
#include <wayland-client.h>
#include "wlr-layer-shell-client.h"
#include "alpha-modifier-v1-client.h"
#include <stdlib.h>
#include <string.h>
#include <SDL3/SDL.h>
#include <SDL3/SDL_video.h>
#include <SDL3/SDL_properties.h>

// --- Layer shell state ---
static struct zwlr_layer_shell_v1 *layer_shell = NULL;
static struct zwlr_layer_surface_v1 *layer_surface = NULL;
static struct wl_surface *wl_surf = NULL;
static uint32_t configure_serial = 0;
static int configured = 0;
static int surface_closed = 0; // the compositor sent closed (e.g. its output went away)

// --- Opacity (wp_alpha_modifier_v1) ---
// The compositor multiplies the surface's alpha by alpha_factor. Window
// opacity has no effect on a layer surface, so this is how it turns
// see-through. The alpha surface lives as long as wl_surf.
static struct wp_alpha_modifier_v1 *alpha_modifier = NULL;
static struct wp_alpha_modifier_surface_v1 *alpha_surface = NULL;
static uint32_t alpha_factor = UINT32_MAX;

// --- Output: the wl_output of the monitor the app chose ---
// SDL knows each display's wl_output (and its position, from xdg-output).
// wl_output's own geometry is no help: wlroots compositors report x=0, y=0
// for every output.
static struct wl_output *target_output = NULL;
static SDL_DisplayID target_display = 0;

// --- Registry listener: discover layer_shell ---
static void registry_global(void *data, struct wl_registry *registry,
    uint32_t name, const char *interface, uint32_t version) {
    (void)data;
    if (strcmp(interface, zwlr_layer_shell_v1_interface.name) == 0) {
        layer_shell = wl_registry_bind(registry, name,
            &zwlr_layer_shell_v1_interface, version < 4 ? version : 4);
    }
    if (strcmp(interface, wp_alpha_modifier_v1_interface.name) == 0) {
        alpha_modifier = wl_registry_bind(registry, name,
            &wp_alpha_modifier_v1_interface, 1);
    }
}

static void registry_global_remove(void *data, struct wl_registry *registry, uint32_t name) {
    (void)data; (void)registry; (void)name;
}

static const struct wl_registry_listener registry_listener = {
    .global = registry_global,
    .global_remove = registry_global_remove,
};

// --- Probe: does the compositor offer layer-shell? ---
static void probe_global(void *data, struct wl_registry *registry,
    uint32_t name, const char *interface, uint32_t version) {
    (void)registry; (void)name; (void)version;
    if (strcmp(interface, zwlr_layer_shell_v1_interface.name) == 0) {
        *(int *)data = 1;
    }
}

static const struct wl_registry_listener probe_listener = {
    .global = probe_global,
    .global_remove = registry_global_remove,
};

// Ask the compositor for its globals on a throwaway connection, before SDL
// starts. Returns 1 if it offers wlr-layer-shell, 0 if not, -1 if there is no
// Wayland display to connect to.
static int probe_layer_shell(void) {
    struct wl_display *d = wl_display_connect(NULL);
    if (!d) return -1;
    int found = 0;
    struct wl_registry *reg = wl_display_get_registry(d);
    wl_registry_add_listener(reg, &probe_listener, &found);
    wl_display_roundtrip(d);
    wl_registry_destroy(reg);
    wl_display_disconnect(d);
    return found;
}

// --- Layer surface listener ---
static void layer_surface_configure(void *data,
    struct zwlr_layer_surface_v1 *surface,
    uint32_t serial, uint32_t width, uint32_t height) {
    (void)data; (void)width; (void)height;
    configure_serial = serial;
    configured = 1;
    zwlr_layer_surface_v1_ack_configure(surface, serial);
}

// The surface is dead after closed. The main loop destroys it and decides
// whether to attach a new one (take_layer_surface_closed).
static void layer_surface_closed(void *data,
    struct zwlr_layer_surface_v1 *surface) {
    (void)data; (void)surface;
    surface_closed = 1;
    configured = 0;
}

static int take_layer_surface_closed() {
    int c = surface_closed;
    surface_closed = 0;
    return c;
}

// After a close the target output may be gone (the registry is not kept,
// so removals are not seen). NULL lets the compositor choose an output.
static void forget_target_output() {
    target_output = NULL;
}

static const struct zwlr_layer_surface_v1_listener layer_surface_listener = {
    .configure = layer_surface_configure,
    .closed = layer_surface_closed,
};

// Select the target display's wl_output (SDL's own; never destroyed here).
static void select_target_output() {
    target_output = NULL;
    if (target_display == 0) return;
    SDL_PropertiesID props = SDL_GetDisplayProperties(target_display);
    if (props) {
        target_output = SDL_GetPointerProperty(props,
            SDL_PROP_DISPLAY_WAYLAND_WL_OUTPUT_POINTER, NULL);
    }
}

// Discover layer_shell global and outputs. Only called once (first attach).
// Safe to roundtrip here because no renderer exists yet.
static int discover_globals(SDL_Window *sdl_win) {
    SDL_PropertiesID gprops = SDL_GetGlobalProperties();
    struct wl_display *display = SDL_GetPointerProperty(gprops,
        SDL_PROP_GLOBAL_VIDEO_WAYLAND_WL_DISPLAY_POINTER, NULL);
    if (!display) return -2;

    struct wl_registry *registry = wl_display_get_registry(display);
    wl_registry_add_listener(registry, &registry_listener, NULL);
    wl_display_roundtrip(display); // discover globals

    wl_registry_destroy(registry);

    if (!layer_shell) return -1;

    select_target_output();
    return 0;
}

// Attach layer-shell role asynchronously (no roundtrip wait).
// Configure arrives via SDL's event pump. Check is_layer_shell_ready() before rendering.
// Returns 0 on success, -1 if layer_shell not available, -2 on other error.
static int attach_layer_shell_async(SDL_Window *sdl_win, int width, int height,
    int anchor_top, int margin, int panel_avoid) {

    // Get wl_display from SDL3
    SDL_PropertiesID gprops = SDL_GetGlobalProperties();
    struct wl_display *display = SDL_GetPointerProperty(gprops,
        SDL_PROP_GLOBAL_VIDEO_WAYLAND_WL_DISPLAY_POINTER, NULL);
    if (!display) return -2;

    // Get wl_surface from SDL3 window (only on first call, SDL owns it)
    if (!wl_surf) {
        SDL_PropertiesID wprops = SDL_GetWindowProperties(sdl_win);
        wl_surf = SDL_GetPointerProperty(wprops,
            SDL_PROP_WINDOW_WAYLAND_SURFACE_POINTER, NULL);
        if (!wl_surf) return -2;
    }

    // Discover globals on first call only
    if (!layer_shell) {
        int ret = discover_globals(sdl_win);
        if (ret != 0) return ret;
    }

    // Destroy existing layer surface if re-attaching (e.g. daemon toggle)
    if (layer_surface) {
        zwlr_layer_surface_v1_destroy(layer_surface);
        layer_surface = NULL;
        configured = 0;
    }
    surface_closed = 0;

    // Clear any pending EGL buffer state before attaching layer-shell role.
    // SDL's eglSwapBuffers may have queued a buffer on the wl_surface from
    // a previous frame. Attaching NULL ensures the commit below doesn't
    // carry a stale buffer that would violate the layer-shell protocol.
    wl_surface_attach(wl_surf, NULL, 0, 0);
    wl_surface_commit(wl_surf);

    // Create layer surface on target output
    layer_surface = zwlr_layer_shell_v1_get_layer_surface(
        layer_shell, wl_surf, target_output,
        ZWLR_LAYER_SHELL_V1_LAYER_OVERLAY, "gamepad-osk");
    if (!layer_surface) return -2;

    zwlr_layer_surface_v1_add_listener(layer_surface, &layer_surface_listener, NULL);

    // Configure anchors, size, margins.
    // Anchor only TOP or BOTTOM (no horizontal anchors) so the compositor
    // centers the surface. KDE left-aligns when LEFT|RIGHT are set with
    // an explicit width; omitting them gives consistent centering everywhere.
    uint32_t anchor = 0;
    if (anchor_top) {
        anchor = ZWLR_LAYER_SURFACE_V1_ANCHOR_TOP;
        zwlr_layer_surface_v1_set_margin(layer_surface, margin, 0, 0, 0);
    } else {
        anchor = ZWLR_LAYER_SURFACE_V1_ANCHOR_BOTTOM;
        zwlr_layer_surface_v1_set_margin(layer_surface, 0, 0, margin, 0);
    }
    zwlr_layer_surface_v1_set_anchor(layer_surface, anchor);
    zwlr_layer_surface_v1_set_size(layer_surface, width, height);

    zwlr_layer_surface_v1_set_keyboard_interactivity(layer_surface,
        ZWLR_LAYER_SURFACE_V1_KEYBOARD_INTERACTIVITY_NONE);
    zwlr_layer_surface_v1_set_exclusive_zone(layer_surface, panel_avoid ? 0 : -1);

    // Commit to trigger configure -- DO NOT WAIT (no roundtrip)
    configured = 0;
    wl_surface_commit(wl_surf);
    wl_display_flush(display);

    return 0;
}

// Reposition the layer surface (top/bottom toggle).
static void reposition_layer_surface(int anchor_top, int margin,
    SDL_Window *sdl_win) {
    if (!layer_surface) return;

    uint32_t anchor = 0;
    if (anchor_top) {
        anchor = ZWLR_LAYER_SURFACE_V1_ANCHOR_TOP;
        zwlr_layer_surface_v1_set_margin(layer_surface, margin, 0, 0, 0);
    } else {
        anchor = ZWLR_LAYER_SURFACE_V1_ANCHOR_BOTTOM;
        zwlr_layer_surface_v1_set_margin(layer_surface, 0, 0, margin, 0);
    }
    zwlr_layer_surface_v1_set_anchor(layer_surface, anchor);

    // Get wl_display for commit
    SDL_PropertiesID gprops = SDL_GetGlobalProperties();
    struct wl_display *display = SDL_GetPointerProperty(gprops,
        SDL_PROP_GLOBAL_VIDEO_WAYLAND_WL_DISPLAY_POINTER, NULL);
    if (wl_surf) {
        wl_surface_commit(wl_surf);
    }
    if (display) {
        wl_display_flush(display);
    }
}

// Set the alpha factor on wl_surf. Takes effect with the next commit.
// Returns 0 when the compositor has no wp_alpha_modifier_v1.
static int set_layer_alpha(uint32_t factor) {
    alpha_factor = factor;
    if (!alpha_modifier || !wl_surf) return 0;
    if (!alpha_surface) {
        alpha_surface = wp_alpha_modifier_v1_get_surface(alpha_modifier, wl_surf);
    }
    wp_alpha_modifier_surface_v1_set_multiplier(alpha_surface, alpha_factor);
    return 1;
}

// Destroy the layer surface (hide or cleanup).
// Preserves layer_shell global and wl_surf for re-attach.
static void destroy_layer_surface() {
    if (layer_surface) {
        zwlr_layer_surface_v1_destroy(layer_surface);
        layer_surface = NULL;
    }
    configured = 0;
    surface_closed = 0;
}

// Full cleanup on exit.
static void cleanup_layer_shell() {
    destroy_layer_surface();
    // Must go before wl_surf, which SDL destroys after this
    if (alpha_surface) {
        wp_alpha_modifier_surface_v1_destroy(alpha_surface);
        alpha_surface = NULL;
    }
    if (alpha_modifier) {
        wp_alpha_modifier_v1_destroy(alpha_modifier);
        alpha_modifier = NULL;
    }
    if (layer_shell) {
        zwlr_layer_shell_v1_destroy(layer_shell);
        layer_shell = NULL;
    }
    target_output = NULL;
    wl_surf = NULL;
}

// Check if layer shell is configured and ready for rendering.
static int is_layer_shell_ready() {
    return layer_surface != NULL && configured;
}

// Check if the compositor offers wp_alpha_modifier_v1 (known after first attach).
static int has_alpha_modifier() {
    return alpha_modifier != NULL;
}

// Check if layer shell was attached (may not be configured yet).
static int has_layer_shell() {
    return layer_surface != NULL;
}

// Set the SDL display the keyboard opens on (0: the compositor chooses).
static void set_target_display(SDL_DisplayID id) {
    target_display = id;
}
*/
import "C"
import (
	"log"
	"math"
	"time"
)

var layerShellActive bool

// probeLayerShell reports whether the Wayland compositor offers
// wlr-layer-shell: 1 yes, 0 no (GNOME), -1 no Wayland display.
func probeLayerShell() int {
	return int(C.probe_layer_shell())
}

// setTargetDisplay sets the SDL display whose output the keyboard opens on
// (0: the compositor chooses).
func setTargetDisplay(id uint32) {
	C.set_target_display(C.SDL_DisplayID(id))
}

// createWaylandWindow creates an SDL3 window with a roleless Wayland surface.
func createWaylandWindow(title string, w, h int32) (*Window, error) {
	window, err := SDL3CreateWindowWithProps(title, 0, 0, w, h,
		true,  // hidden
		true,  // borderless
		false, // always_on_top (layer-shell handles this)
		true,  // waylandRoleCustom
	)
	if err != nil {
		// Fallback: standard window
		log.Printf("Warning: roleless window creation failed: %v, falling back to standard window", err)
		return SDL3CreateWindowWithProps(title, 0, 0, w, h,
			true, true, true, false)
	}
	return window, nil
}

// attachLayerShellAsync attaches a wlr-layer-shell overlay role asynchronously.
// Configure arrives via SDL's event pump. Check IsLayerShellReady() before rendering.
func attachLayerShellAsync(window *Window, w, h int32, top bool, margin int32, panelAvoid bool) {
	anchorTop := 0
	if top {
		anchorTop = 1
	}
	pa := 0
	if panelAvoid {
		pa = 1
	}
	ret := int(C.attach_layer_shell_async(
		(*C.SDL_Window)(window.ptr),
		C.int(w), C.int(h),
		C.int(anchorTop), C.int(margin), C.int(pa),
	))

	switch ret {
	case 0:
		layerShellActive = true
		log.Printf("Layer-shell overlay requested (waiting for configure)")
	case -1:
		log.Printf("Warning: compositor does not support wlr-layer-shell, using standard window")
		layerShellActive = false
	default:
		log.Printf("Warning: failed to attach layer-shell (error %d), using standard window", ret)
		layerShellActive = false
	}
}

// IsLayerShellReady returns true if the layer surface is configured and ready for rendering.
func IsLayerShellReady() bool {
	return bool(C.is_layer_shell_ready() != 0)
}

// repositionLayerSurface changes the layer surface anchor between top and bottom.
func repositionLayerSurface(top bool, margin int32, window *Window) {
	if !layerShellActive {
		return
	}
	anchorTop := 0
	if top {
		anchorTop = 1
	}
	C.reposition_layer_surface(C.int(anchorTop), C.int(margin),
		(*C.SDL_Window)(window.ptr))
}

// destroyLayerSurface destroys the layer surface (for hide).
// Preserves layer_shell global for re-attach.
func destroyLayerSurface() {
	if layerShellActive {
		C.destroy_layer_surface()
		layerShellActive = false
	}
}

// takeLayerSurfaceClosed reports (once) that the compositor closed the
// layer surface since the last call.
func takeLayerSurfaceClosed() bool {
	return C.take_layer_surface_closed() != 0
}

// forgetTargetOutput makes the next attach let the compositor pick the output.
func forgetTargetOutput() {
	C.forget_target_output()
}

// reattachAfterClose decides what to do when the compositor closes the
// keyboard: show it again once, but hide it if it was closed again within
// 2 s (no output takes it), so it cannot loop and the pad grab is released.
func reattachAfterClose(now, lastClose time.Time) bool {
	return lastClose.IsZero() || now.Sub(lastClose) >= 2*time.Second
}

// cleanupLayerShell fully cleans up all layer-shell resources (exit).
func cleanupLayerShell() {
	C.cleanup_layer_shell()
	layerShellActive = false
}

// setLayerOpacity makes the layer surface see-through through the
// compositor's alpha modifier. Returns false if the compositor has none.
// Globals are found on the first attach, so call it after attachLayerShellAsync.
func setLayerOpacity(opacity float64) bool {
	return C.set_layer_alpha(C.uint32_t(alphaFactor(opacity))) != 0
}

// hasAlphaModifier reports whether the compositor can make the layer surface
// see-through. Only known after the first attachLayerShellAsync.
func hasAlphaModifier() bool {
	return C.has_alpha_modifier() != 0
}

// alphaFactor maps opacity 0..1 to the protocol's 0..UINT32_MAX.
func alphaFactor(opacity float64) uint32 {
	opacity = max(0, min(1, opacity))
	return uint32(math.Round(opacity * math.MaxUint32))
}

// hasLayerShell returns true if layer-shell was attached (may not be configured).
func hasLayerShell() bool {
	return layerShellActive
}
