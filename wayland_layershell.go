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

// --- Registry listener: discover layer_shell global ---
static void registry_global(void *data, struct wl_registry *registry,
    uint32_t name, const char *interface, uint32_t version) {
    (void)data;
    if (strcmp(interface, zwlr_layer_shell_v1_interface.name) == 0) {
        layer_shell = wl_registry_bind(registry, name,
            &zwlr_layer_shell_v1_interface, version < 4 ? version : 4);
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

// Attach layer-shell role to an SDL3 window's wl_surface.
// Returns 0 on success, -1 if layer_shell not available, -2 on other error.
static int attach_layer_shell(SDL_Window *sdl_win, int width, int height,
    int anchor_top, int margin) {

    // Get wl_display from SDL3
    SDL_PropertiesID gprops = SDL_GetGlobalProperties();
    struct wl_display *display = SDL_GetPointerProperty(gprops,
        SDL_PROP_GLOBAL_VIDEO_WAYLAND_WL_DISPLAY_POINTER, NULL);
    if (!display) return -2;

    // Get wl_surface from SDL3 window
    SDL_PropertiesID wprops = SDL_GetWindowProperties(sdl_win);
    wl_surf = SDL_GetPointerProperty(wprops,
        SDL_PROP_WINDOW_WAYLAND_SURFACE_POINTER, NULL);
    if (!wl_surf) return -2;

    // Discover layer_shell global
    struct wl_registry *registry = wl_display_get_registry(display);
    wl_registry_add_listener(registry, &registry_listener, NULL);
    wl_display_roundtrip(display);

    if (!layer_shell) {
        wl_registry_destroy(registry);
        return -1; // compositor doesn't support layer-shell
    }

    // Create layer surface
    layer_surface = zwlr_layer_shell_v1_get_layer_surface(
        layer_shell, wl_surf, NULL,
        ZWLR_LAYER_SHELL_V1_LAYER_OVERLAY, "gamepad-osk");
    if (!layer_surface) {
        wl_registry_destroy(registry);
        return -2;
    }

    zwlr_layer_surface_v1_add_listener(layer_surface, &layer_surface_listener, NULL);

    // Configure anchors, size, margins
    uint32_t anchor = ZWLR_LAYER_SURFACE_V1_ANCHOR_LEFT |
                      ZWLR_LAYER_SURFACE_V1_ANCHOR_RIGHT;
    if (anchor_top) {
        anchor |= ZWLR_LAYER_SURFACE_V1_ANCHOR_TOP;
    } else {
        anchor |= ZWLR_LAYER_SURFACE_V1_ANCHOR_BOTTOM;
    }
    zwlr_layer_surface_v1_set_anchor(layer_surface, anchor);
    zwlr_layer_surface_v1_set_size(layer_surface, width, height);

    if (anchor_top) {
        zwlr_layer_surface_v1_set_margin(layer_surface, margin, 0, 0, 0);
    } else {
        zwlr_layer_surface_v1_set_margin(layer_surface, 0, 0, margin, 0);
    }

    zwlr_layer_surface_v1_set_keyboard_interactivity(layer_surface,
        ZWLR_LAYER_SURFACE_V1_KEYBOARD_INTERACTIVITY_NONE);
    // 0 = respect other surfaces' exclusive zones (auto-avoid panels/taskbars)
    zwlr_layer_surface_v1_set_exclusive_zone(layer_surface, 0);

    // Commit and wait for configure
    wl_surface_commit(wl_surf);
    configured = 0;
    while (!configured) {
        wl_display_roundtrip(display);
    }

    wl_registry_destroy(registry);
    return 0;
}

// Reposition the layer surface (top/bottom toggle).
static void reposition_layer_surface(int anchor_top, int margin,
    SDL_Window *sdl_win) {
    if (!layer_surface) return;

    uint32_t anchor = ZWLR_LAYER_SURFACE_V1_ANCHOR_LEFT |
                      ZWLR_LAYER_SURFACE_V1_ANCHOR_RIGHT;
    if (anchor_top) {
        anchor |= ZWLR_LAYER_SURFACE_V1_ANCHOR_TOP;
        zwlr_layer_surface_v1_set_margin(layer_surface, margin, 0, 0, 0);
    } else {
        anchor |= ZWLR_LAYER_SURFACE_V1_ANCHOR_BOTTOM;
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
        wl_display_roundtrip(display);
    }
}

// Destroy the layer surface.
static void destroy_layer_surface() {
    if (layer_surface) {
        zwlr_layer_surface_v1_destroy(layer_surface);
        layer_surface = NULL;
    }
    if (layer_shell) {
        zwlr_layer_shell_v1_destroy(layer_shell);
        layer_shell = NULL;
    }
    wl_surf = NULL;
    configured = 0;
}

// Check if layer shell was successfully attached.
static int has_layer_shell() {
    return layer_surface != NULL;
}
*/
import "C"
import (
	"log"
)

var layerShellActive bool

// createLayerShellWindow creates an SDL3 window with a roleless Wayland surface
// and attaches a wlr-layer-shell overlay role.
// Falls back to a standard SDL3 window if the compositor doesn't support layer-shell.
func createLayerShellWindow(title string, w, h int32, top bool, margin int32) (*Window, error) {
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

	anchorTop := 0
	if top {
		anchorTop = 1
	}
	ret := int(C.attach_layer_shell(
		(*C.SDL_Window)(window.ptr),
		C.int(w), C.int(h),
		C.int(anchorTop), C.int(margin),
	))

	switch ret {
	case 0:
		layerShellActive = true
		log.Printf("Layer-shell overlay attached successfully")
	case -1:
		log.Printf("Warning: compositor does not support wlr-layer-shell, using standard window")
		layerShellActive = false
	default:
		log.Printf("Warning: failed to attach layer-shell (error %d), using standard window", ret)
		layerShellActive = false
	}

	return window, nil
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

// destroyLayerSurface cleans up the layer-shell resources.
func destroyLayerSurface() {
	if layerShellActive {
		C.destroy_layer_surface()
		layerShellActive = false
	}
}

// hasLayerShell returns true if layer-shell is active.
func hasLayerShell() bool {
	return layerShellActive
}
