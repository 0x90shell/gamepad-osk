"""Gamepad input processing via evdev with adaptive navigation speed."""

import time
from dataclasses import dataclass
from evdev import InputDevice, ecodes, list_devices


@dataclass
class NavAxis:
    """Tracks a single navigation axis with adaptive repeat speed."""
    direction: int = 0
    held_since: float = 0.0
    last_move: float = 0.0

    def get_repeat_interval(self):
        """Returns repeat interval in seconds. Ramps from 300ms to 80ms over 1s."""
        if self.held_since == 0:
            return 0.3
        elapsed = time.monotonic() - self.held_since
        t = min(elapsed / 1.0, 1.0)
        return 0.3 + (0.08 - 0.3) * t


@dataclass
class Action:
    type: str  # navigate, press, backspace, space, enter, shift_on, shift_off, close, mouse_move
    dx: int = 0
    dy: int = 0


class GamepadReader:
    """Reads evdev gamepad events and produces semantic actions."""

    def __init__(self, config):
        self.config = config
        self.device = None
        self.grabbed = False

        self.nav_x = NavAxis()
        self.nav_y = NavAxis()
        self.deadzone = config.deadzone

        # Axis ranges (will be set from device info)
        self.axis_max = 32767
        self.trigger_max = 1023

        # Mouse state (left stick)
        self.mouse_x = 0.0
        self.mouse_y = 0.0

        # A button hold tracking for long-press
        self.a_held = False

        # Trigger edge detection
        self.lt_active = False
        self.rt_active = False

    def open_device(self, device_path=None):
        """Open the gamepad device."""
        path = device_path or self.config.device
        if path:
            self.device = InputDevice(path)
        else:
            self.device = self._auto_detect()

        if self.device:
            # Get axis ranges
            caps = self.device.capabilities(absinfo=True)
            abs_caps = caps.get(ecodes.EV_ABS, [])
            for code, absinfo in abs_caps:
                if code == ecodes.ABS_X:
                    self.axis_max = absinfo.max or 32767
                elif code == ecodes.ABS_Z:
                    self.trigger_max = absinfo.max or 1023

        return self.device is not None

    def _auto_detect(self):
        """Find a gamepad device by capabilities."""
        for path in list_devices():
            try:
                dev = InputDevice(path)
                caps = dev.capabilities()
                has_abs = ecodes.EV_ABS in caps
                has_btn = ecodes.EV_KEY in caps
                if has_abs and has_btn:
                    abs_codes = [c if isinstance(c, int) else c[0]
                                 for c in caps[ecodes.EV_ABS]]
                    key_codes = caps[ecodes.EV_KEY]
                    if (ecodes.ABS_X in abs_codes and
                            ecodes.BTN_SOUTH in key_codes):
                        print(f"Auto-detected gamepad: {dev.name} ({dev.path})")
                        return dev
                dev.close()
            except (PermissionError, OSError):
                continue
        return None

    def grab(self):
        """Grab the device (exclusive access)."""
        if self.device and not self.grabbed:
            try:
                self.device.grab()
                self.grabbed = True
            except OSError as e:
                print(f"Warning: could not grab device: {e}")

    def ungrab(self):
        """Release the device grab."""
        if self.device and self.grabbed:
            try:
                self.device.ungrab()
                self.grabbed = False
            except OSError:
                pass

    def fileno(self):
        """Return fd for select()."""
        if self.device:
            return self.device.fd
        return -1

    def process_events(self):
        """Read pending events and return list of Actions."""
        if not self.device:
            return []

        actions = []
        try:
            for event in self.device.read():
                action = self._handle_event(event)
                if action:
                    actions.append(action)
        except BlockingIOError:
            pass

        # Check adaptive repeat for navigation
        now = time.monotonic()
        nav_action = self._check_nav_repeat(now)
        if nav_action:
            actions.append(nav_action)

        # Mouse movement from left stick
        if self.config.mouse_enabled and (abs(self.mouse_x) > 0.01 or abs(self.mouse_y) > 0.01):
            sens = self.config.mouse_sensitivity
            actions.append(Action("mouse_move",
                                  dx=int(self.mouse_x * sens),
                                  dy=int(self.mouse_y * sens)))

        return actions

    def _handle_event(self, event):
        if event.type == ecodes.EV_KEY:
            return self._handle_button(event.code, event.value)
        elif event.type == ecodes.EV_ABS:
            return self._handle_axis(event.code, event.value)
        return None

    def _handle_button(self, code, value):
        # value: 1=press, 0=release
        if code == ecodes.BTN_SOUTH:  # A
            if value == 1:
                self.a_held = True
                return Action("press_start")
            else:
                self.a_held = False
                return Action("press")
        elif code == ecodes.BTN_EAST:  # B
            if value == 1:
                return Action("close")
        elif code == ecodes.BTN_WEST:  # X
            if value == 1:
                return Action("backspace")
        elif code == ecodes.BTN_NORTH:  # Y
            if value == 1:
                return Action("space")
        return None

    def _handle_axis(self, code, value):
        now = time.monotonic()

        # Right stick - keyboard navigation
        if code == ecodes.ABS_RX:
            normalized = value / self.axis_max
            direction = self._apply_deadzone(normalized)
            return self._update_nav(self.nav_x, direction, now, dx=True)
        elif code == ecodes.ABS_RY:
            normalized = value / self.axis_max
            direction = self._apply_deadzone(normalized)
            return self._update_nav(self.nav_y, direction, now, dx=False)

        # D-pad - keyboard navigation (digital)
        elif code == ecodes.ABS_HAT0X:
            return self._update_nav(self.nav_x, value, now, dx=True)
        elif code == ecodes.ABS_HAT0Y:
            return self._update_nav(self.nav_y, value, now, dx=False)

        # Left stick - mouse
        elif code == ecodes.ABS_X:
            normalized = value / self.axis_max
            if abs(normalized) < self.deadzone:
                self.mouse_x = 0.0
            else:
                self.mouse_x = normalized
        elif code == ecodes.ABS_Y:
            normalized = value / self.axis_max
            if abs(normalized) < self.deadzone:
                self.mouse_y = 0.0
            else:
                self.mouse_y = normalized

        # Triggers (edge detection — only fire on transition)
        elif code == ecodes.ABS_Z:  # LT
            threshold = self.trigger_max * 0.3
            active = value > threshold
            if active != self.lt_active:
                self.lt_active = active
                return Action("shift_on" if active else "shift_off")
        elif code == ecodes.ABS_RZ:  # RT
            threshold = self.trigger_max * 0.3
            active = value > threshold
            if active and not self.rt_active:
                self.rt_active = True
                return Action("enter")
            elif not active:
                self.rt_active = False

        return None

    def _apply_deadzone(self, normalized):
        """Convert normalized analog value to -1, 0, or 1."""
        if normalized > self.deadzone:
            return 1
        elif normalized < -self.deadzone:
            return -1
        return 0

    def _update_nav(self, nav, direction, now, dx=True):
        """Update a nav axis and return immediate action if direction changed."""
        if direction != nav.direction:
            nav.direction = direction
            if direction != 0:
                nav.held_since = now
                nav.last_move = now
                if dx:
                    return Action("navigate", dx=direction, dy=0)
                else:
                    return Action("navigate", dx=0, dy=direction)
            else:
                nav.held_since = 0.0
        return None

    def _check_nav_repeat(self, now):
        """Check if held navigation should fire a repeat."""
        for nav, is_x in [(self.nav_x, True), (self.nav_y, False)]:
            if nav.direction != 0:
                interval = nav.get_repeat_interval()
                if now - nav.last_move >= interval:
                    nav.last_move = now
                    if is_x:
                        return Action("navigate", dx=nav.direction, dy=0)
                    else:
                        return Action("navigate", dx=0, dy=nav.direction)
        return None

    def close(self):
        self.ungrab()
        if self.device:
            try:
                self.device.close()
            except Exception:
                pass
