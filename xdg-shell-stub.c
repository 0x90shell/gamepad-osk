// Stub for xdg_popup_interface referenced by wlr-layer-shell protocol code.
// We don't use xdg_popup functionality - this provides the symbol the linker needs.
#include <wayland-client.h>

const struct wl_interface xdg_popup_interface = {
    "xdg_popup", 1, 0, NULL, 0, NULL
};
