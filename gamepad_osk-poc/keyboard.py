"""Keyboard state machine — cursor position, layer switching, key resolution."""

import time
from evdev import ecodes


class KeyboardState:
    def __init__(self, layout):
        self.layout = layout
        self.cursor_row = 2  # start on QWERTY row
        self.cursor_col = 1  # skip Tab, land on 'q'
        self.shift_active = False
        self.caps_active = False
        self.ctrl_active = False
        self.alt_active = False
        self.meta_active = False

        # Long-press for accents
        self.long_press_start = 0.0
        self.long_press_active = False
        self.accent_popup = None  # (accents_list, selected_index) or None

        # Track cursor x-position in pixel units for vertical nav
        self._target_x = None

    def navigate(self, dx, dy):
        """Move cursor by (dx, dy) grid steps."""
        if self.accent_popup is not None:
            # Navigate within accent popup
            accents, idx = self.accent_popup
            new_idx = idx + dx
            if 0 <= new_idx < len(accents):
                self.accent_popup = (accents, new_idx)
            return

        if dy != 0:
            # Save current pixel x-position for vertical alignment
            if self._target_x is None:
                self._target_x = self._key_center_x(self.cursor_row, self.cursor_col)
            new_row = (self.cursor_row + dy) % len(self.layout)
            self.cursor_row = new_row
            # Find the key whose center is closest to target_x
            self.cursor_col = self._find_closest_col(new_row, self._target_x)
        elif dx != 0:
            row = self.layout[self.cursor_row]
            new_col = self.cursor_col + dx
            if new_col < 0:
                new_col = len(row) - 1
            elif new_col >= len(row):
                new_col = 0
            self.cursor_col = new_col
            self._target_x = None  # reset on horizontal movement

    def _key_center_x(self, row_idx, col_idx):
        """Get the center x position (in key-width units) of a key."""
        row = self.layout[row_idx]
        x = 0.0
        for i, key in enumerate(row):
            if i == col_idx:
                return x + key.width / 2.0
            x += key.width
        return x

    def _find_closest_col(self, row_idx, target_x):
        """Find the column in row whose center is closest to target_x."""
        row = self.layout[row_idx]
        best_col = 0
        best_dist = float("inf")
        x = 0.0
        for i, key in enumerate(row):
            center = x + key.width / 2.0
            dist = abs(center - target_x)
            if dist < best_dist:
                best_dist = dist
                best_col = i
            x += key.width
        return best_col

    def get_current_key(self):
        row = self.layout[self.cursor_row]
        return row[min(self.cursor_col, len(row) - 1)]

    def get_display_label(self, key):
        """Get the label to display based on current layer."""
        if (self.shift_active ^ self.caps_active) and key.shift_label:
            return key.shift_label
        return key.label

    def press_current(self, injector):
        """Press the current key, handling modifiers and injection."""
        # If accent popup is open, select the accent
        if self.accent_popup is not None:
            accents, idx = self.accent_popup
            label, codepoint = accents[idx]
            injector.type_unicode(codepoint)
            self.close_accent_popup()
            return

        key = self.get_current_key()

        if key.is_modifier:
            self._toggle_modifier(key)
            return

        # Build modifier set
        mods = set()
        if self.shift_active:
            mods.add(ecodes.KEY_LEFTSHIFT)
        if self.ctrl_active:
            mods.add(ecodes.KEY_LEFTCTRL)
        if self.alt_active:
            mods.add(ecodes.KEY_LEFTALT)
        if self.meta_active:
            mods.add(ecodes.KEY_LEFTMETA)

        injector.press_key(key.code, mods)

        # Auto-release shift after one keypress (unless caps is on)
        if self.shift_active and not self.caps_active:
            self.shift_active = False
        # Auto-release ctrl/alt/meta after one keypress
        self.ctrl_active = False
        self.alt_active = False
        self.meta_active = False

    def start_long_press(self):
        key = self.get_current_key()
        if key.accents:
            self.long_press_start = time.monotonic()
            self.long_press_active = True

    def cancel_long_press(self):
        self.long_press_active = False
        self.long_press_start = 0.0

    def check_long_press(self, long_press_ms):
        """Check if long-press threshold reached. Returns True if popup opened."""
        if not self.long_press_active:
            return False
        elapsed = (time.monotonic() - self.long_press_start) * 1000
        if elapsed >= long_press_ms:
            key = self.get_current_key()
            if key.accents:
                self.accent_popup = (key.accents, 0)
                self.long_press_active = False
                return True
        return False

    def close_accent_popup(self):
        self.accent_popup = None
        self.long_press_active = False

    def _toggle_modifier(self, key):
        if key.modifier_type == "shift":
            self.shift_active = not self.shift_active
        elif key.modifier_type == "caps":
            self.caps_active = not self.caps_active
        elif key.modifier_type == "ctrl":
            self.ctrl_active = not self.ctrl_active
        elif key.modifier_type == "alt":
            self.alt_active = not self.alt_active
        elif key.modifier_type == "meta":
            self.meta_active = not self.meta_active
