"""Main application — event loop orchestrating all modules."""

import select
import sys
import threading

import pygame
from .gamepad import GamepadReader
from .injector import KeyInjector
from .ipc import IPCServer
from .keyboard import KeyboardState
from .layouts import get_layout
from .renderer import Renderer, calculate_window_size, calculate_unit_size
from .themes import get_theme
from .window import create_window, _setup_window_hints, get_screen_size


class Application:
    def __init__(self, config, device_path=None):
        self.config = config
        self.device_path = device_path
        self.running = False
        self.visible = True
        self._toggle_pending = False
        self._ipc_lock = threading.Lock()

    def run(self):
        layout = get_layout()
        theme = get_theme(self.config.theme_name)

        # Auto-scale unit_size to screen resolution
        pygame.init()
        screen_w, screen_h = get_screen_size()
        unit_size = calculate_unit_size(layout, screen_w, self.config)
        pad = self.config.padding
        width, height = calculate_window_size(layout, unit_size, pad)
        pygame.quit()

        surface = create_window(width, height, self.config)
        clock = pygame.time.Clock()

        keyboard = KeyboardState(layout)
        renderer = Renderer(surface, theme, unit_size, self.config)

        # Init key injector
        try:
            injector = KeyInjector()
        except Exception as e:
            print(f"Error: cannot create UInput device: {e}", file=sys.stderr)
            print("Make sure you're in the 'input' group: sudo usermod -aG input $USER",
                  file=sys.stderr)
            pygame.quit()
            return

        # Init gamepad
        gamepad = GamepadReader(self.config)
        if not gamepad.open_device(self.device_path):
            print("Error: no gamepad found", file=sys.stderr)
            print("Specify device: gamepad-osk /dev/input/gamepad0", file=sys.stderr)
            injector.close()
            pygame.quit()
            return

        # Set non-blocking
        import fcntl
        import os
        flags = fcntl.fcntl(gamepad.fileno(), fcntl.F_GETFL)
        fcntl.fcntl(gamepad.fileno(), fcntl.F_SETFL, flags | os.O_NONBLOCK)

        # Grab if configured
        if self.config.grab:
            gamepad.grab()

        # Start IPC server
        ipc = IPCServer(self._handle_ipc_command)
        ipc.start()

        self.running = True
        try:
            self._main_loop(surface, clock, keyboard, renderer, injector, gamepad)
        finally:
            ipc.stop()
            gamepad.close()
            injector.close()
            pygame.quit()

    def _main_loop(self, surface, clock, keyboard, renderer, injector, gamepad):
        while self.running:
            # Check for toggle command from IPC
            with self._ipc_lock:
                if self._toggle_pending:
                    self._toggle_pending = False
                    self.visible = not self.visible
                    if self.visible:
                        if self.config.grab:
                            gamepad.grab()
                        # Recreate window to show it
                        pygame.display.set_mode(surface.get_size(), pygame.NOFRAME)
                        _setup_window_hints()
                    else:
                        if self.config.grab:
                            gamepad.ungrab()
                        pygame.display.iconify()

            # Process pygame events
            for event in pygame.event.get():
                if event.type == pygame.QUIT:
                    self.running = False
                    return

            # Process gamepad events (non-blocking via select)
            gp_fd = gamepad.fileno()
            if gp_fd >= 0:
                readable, _, _ = select.select([gp_fd], [], [], 0)
                if readable:
                    actions = gamepad.process_events()
                    for action in actions:
                        self._handle_action(action, keyboard, injector, gamepad)
                else:
                    # Still check repeat timers even without new events
                    actions = gamepad.process_events()
                    for action in actions:
                        self._handle_action(action, keyboard, injector, gamepad)

            # Check long-press
            if keyboard.check_long_press(self.config.long_press_ms):
                pass  # popup opened, will render next frame

            # Render
            if self.visible:
                renderer.draw(keyboard)
                pygame.display.flip()

            clock.tick(60)

    def _handle_action(self, action, keyboard, injector, gamepad):
        if action.type == "navigate":
            keyboard.navigate(action.dx, action.dy)
        elif action.type == "press":
            # A button released — if accent popup is open or long-press didn't trigger,
            # press the current key
            if keyboard.accent_popup is not None:
                keyboard.press_current(injector)
            elif not keyboard.long_press_active:
                # Normal press (long-press didn't activate)
                keyboard.press_current(injector)
            keyboard.cancel_long_press()
        elif action.type == "press_start":
            # A button pressed — start long-press timer
            keyboard.start_long_press()
        elif action.type == "backspace":
            from evdev import ecodes
            injector.press_key(ecodes.KEY_BACKSPACE)
        elif action.type == "space":
            from evdev import ecodes
            injector.press_key(ecodes.KEY_SPACE)
        elif action.type == "enter":
            from evdev import ecodes
            injector.press_key(ecodes.KEY_ENTER)
        elif action.type == "shift_on":
            keyboard.shift_active = True
        elif action.type == "shift_off":
            keyboard.shift_active = False
        elif action.type == "close":
            self.running = False
        elif action.type == "mouse_move":
            injector.move_mouse(action.dx, action.dy)

    def _handle_ipc_command(self, cmd):
        """Called from IPC thread."""
        if cmd == "toggle":
            with self._ipc_lock:
                self._toggle_pending = True
