"""SDL2/pygame window management — borderless, always-on-top, positioned."""

import os
import ctypes
import ctypes.util

import pygame


def create_window(width, height, config):
    """Create a borderless, always-on-top window at bottom-center of screen."""
    os.environ.setdefault("SDL_VIDEO_ALLOW_SCREENSAVER", "1")

    pygame.init()

    # Get screen dimensions for positioning
    info = pygame.display.Info()
    screen_w = info.current_w
    screen_h = info.current_h

    x = (screen_w - width) // 2
    y = screen_h - height - config.bottom_margin

    # Position hint (X11 only, Wayland ignores this)
    os.environ["SDL_VIDEO_WINDOW_POS"] = f"{x},{y}"

    surface = pygame.display.set_mode((width, height), pygame.NOFRAME)
    pygame.display.set_caption("gamepad-osk")

    # Set always-on-top and prevent focus stealing
    _setup_window_hints()

    return surface


def _setup_window_hints():
    """Set X11/SDL2 hints for overlay behavior."""
    # Try to use SDL2 directly for always-on-top
    sdl2_name = ctypes.util.find_library("SDL2-2.0") or "libSDL2-2.0.so.0"
    try:
        sdl2 = ctypes.CDLL(sdl2_name)
    except OSError:
        return

    # SDL_SetHint for window type
    SDL_SetHint = sdl2.SDL_SetHint
    SDL_SetHint.argtypes = [ctypes.c_char_p, ctypes.c_char_p]
    SDL_SetHint.restype = ctypes.c_int

    # Get the SDL window pointer from pygame
    try:
        sdl_window = pygame.display.get_wm_info().get("window")
    except Exception:
        sdl_window = None

    if sdl_window is None:
        return

    # Try X11 approach for focus prevention and always-on-top
    x11_name = ctypes.util.find_library("X11")
    if x11_name:
        _setup_x11_hints(sdl_window, x11_name)


def _setup_x11_hints(window_id, x11_name):
    """Set X11 window properties for overlay behavior."""
    try:
        x11 = ctypes.CDLL(x11_name)
    except OSError:
        return

    try:
        display_name = os.environ.get("DISPLAY")
        if not display_name:
            return  # Wayland without XWayland

        display = x11.XOpenDisplay(display_name.encode() if display_name else None)
        if not display:
            return

        # Intern atoms
        def intern(name):
            return x11.XInternAtom(display, name.encode(), False)

        wm_state = intern("_NET_WM_STATE")
        above = intern("_NET_WM_STATE_ABOVE")
        skip_taskbar = intern("_NET_WM_STATE_SKIP_TASKBAR")
        skip_pager = intern("_NET_WM_STATE_SKIP_PAGER")

        # Set _NET_WM_STATE: above, skip_taskbar, skip_pager
        atoms = (ctypes.c_long * 3)(above, skip_taskbar, skip_pager)
        x11.XChangeProperty(
            display, window_id, wm_state,
            intern("ATOM"), 32, 0,  # PropModeReplace=0
            ctypes.cast(atoms, ctypes.c_char_p), 3,
        )

        # Set WM_HINTS to not take input focus
        # The WM_HINTS struct has: flags(long), input(bool), ...
        # Flag bit 0 = InputHint; input=False means don't give us focus
        wm_hints_atom = intern("WM_HINTS")
        hints = (ctypes.c_long * 9)()
        hints[0] = 1  # InputHint flag
        hints[1] = 0  # input = False
        x11.XChangeProperty(
            display, window_id, wm_hints_atom,
            wm_hints_atom, 32, 0,
            ctypes.cast(hints, ctypes.c_char_p), 9,
        )

        x11.XFlush(display)
        x11.XCloseDisplay(display)

    except Exception:
        pass  # best-effort


def get_screen_size():
    """Return current screen dimensions."""
    info = pygame.display.Info()
    return info.current_w, info.current_h
