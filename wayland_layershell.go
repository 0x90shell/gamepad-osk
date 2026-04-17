package main

/*
#cgo pkg-config: wayland-client
#include <wayland-client.h>
#include "wlr-layer-shell-client.h"
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

// --- Output matching ---
static struct wl_output *target_output = NULL;
static int target_x = 0, target_y = 0;

// Track outputs during registry discovery
#define MAX_OUTPUTS 8
static struct {
    struct wl_output *output;
    int x, y;
    int done; // geometry received
} discovered_outputs[MAX_OUTPUTS];
static int num_outputs = 0;

static void output_geometry(void *data, struct wl_output *output,
    int32_t x, int32_t y, int32_t pw, int32_t ph,
    int32_t subpixel, const char *make, const char *model, int32_t transform) {
    (void)pw; (void)ph; (void)subpixel; (void)make; (void)model; (void)transform;
    // Find this output in our list and store geometry
    for (int i = 0; i < num_outputs; i++) {
        if (discovered_outputs[i].output == output) {
            discovered_outputs[i].x = x;
            discovered_outputs[i].y = y;
            discovered_outputs[i].done = 1;
            break;
        }
    }
}

static void output_mode(void *data, struct wl_output *output,
    uint32_t flags, int32_t w, int32_t h, int32_t refresh) {
    (void)data; (void)output; (void)flags; (void)w; (void)h; (void)refresh;
}
static void output_done(void *data, struct wl_output *output) {
    (void)data; (void)output;
}
static void output_scale(void *data, struct wl_output *output, int32_t factor) {
    (void)data; (void)output; (void)factor;
}
static void output_name(void *data, struct wl_output *output, const char *name) {
    (void)data; (void)output; (void)name;
}
static void output_description(void *data, struct wl_output *output, const char *desc) {
    (void)data; (void)output; (void)desc;
}

static const struct wl_output_listener output_listener = {
    .geometry = output_geometry,
    .mode = output_mode,
    .done = output_done,
    .scale = output_scale,
    .name = output_name,
    .description = output_description,
};

// --- Registry listener: discover layer_shell and outputs ---
static void registry_global(void *data, struct wl_registry *registry,
    uint32_t name, const char *interface, uint32_t version) {
    (void)data;
    if (strcmp(interface, zwlr_layer_shell_v1_interface.name) == 0) {
        layer_shell = wl_registry_bind(registry, name,
            &zwlr_layer_shell_v1_interface, version < 4 ? version : 4);
    }
    if (strcmp(interface, "wl_output") == 0 && num_outputs < MAX_OUTPUTS) {
        struct wl_output *out = wl_registry_bind(registry, name,
            &wl_output_interface, version < 4 ? version : 4);
        discovered_outputs[num_outputs].output = out;
        discovered_outputs[num_outputs].done = 0;
        wl_output_add_listener(out, &output_listener, NULL);
        num_outputs++;
    }
}

static void registry_global_remove(void *data, struct wl_registry *registry, uint32_t name) {
    (void)data; (void)registry; (void)name;
}

static const struct wl_registry_listener registry_listener = {
    .global = registry_global,
    .global_remove = registry_global_remove,
};

// --- Layer surface listener ---
static void layer_surface_configure(void *data,
    struct zwlr_layer_surface_v1 *surface,
    uint32_t serial, uint32_t width, uint32_t height) {
    (void)data; (void)width; (void)height;
    configure_serial = serial;
    configured = 1;
    zwlr_layer_surface_v1_ack_configure(surface, serial);
}

static void layer_surface_closed(void *data,
    struct zwlr_layer_surface_v1 *surface) {
    (void)data; (void)surface;
}

static const struct zwlr_layer_surface_v1_listener layer_surface_listener = {
    .configure = layer_surface_configure,
    .closed = layer_surface_closed,
};

// Select the wl_output matching the target monitor position.
static void select_target_output() {
    target_output = NULL;
    for (int i = 0; i < num_outputs; i++) {
        if (discovered_outputs[i].done &&
            discovered_outputs[i].x == target_x &&
            discovered_outputs[i].y == target_y) {
            target_output = discovered_outputs[i].output;
            return;
        }
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
    wl_display_roundtrip(display); // receive output geometry events

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

// Destroy the layer surface (hide or cleanup).
// Preserves layer_shell global and wl_surf for re-attach.
static void destroy_layer_surface() {
    if (layer_surface) {
        zwlr_layer_surface_v1_destroy(layer_surface);
        layer_surface = NULL;
    }
    configured = 0;
}

// Full cleanup on exit.
static void cleanup_layer_shell() {
    destroy_layer_surface();
    if (layer_shell) {
        zwlr_layer_shell_v1_destroy(layer_shell);
        layer_shell = NULL;
    }
    for (int i = 0; i < num_outputs; i++) {
        if (discovered_outputs[i].output) {
            wl_output_destroy(discovered_outputs[i].output);
            discovered_outputs[i].output = NULL;
        }
    }
    num_outputs = 0;
    target_output = NULL;
    wl_surf = NULL;
}

// Check if layer shell is configured and ready for rendering.
static int is_layer_shell_ready() {
    return layer_surface != NULL && configured;
}

// Check if layer shell was attached (may not be configured yet).
static int has_layer_shell() {
    return layer_surface != NULL;
}

// Set the target monitor position for output matching.
static void set_target_monitor(int x, int y) {
    target_x = x;
    target_y = y;
}
*/
import "C"
import (
	"log"
)

var layerShellActive bool

// setTargetMonitor sets the target monitor position for wl_output matching.
func setTargetMonitor(x, y int32) {
	C.set_target_monitor(C.int(x), C.int(y))
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

// cleanupLayerShell fully cleans up all layer-shell resources (exit).
func cleanupLayerShell() {
	C.cleanup_layer_shell()
	layerShellActive = false
}

// hasLayerShell returns true if layer-shell was attached (may not be configured).
func hasLayerShell() bool {
	return layerShellActive
}
