"""Pygame renderer for the on-screen keyboard."""

import pygame


class Renderer:
    def __init__(self, surface, theme, unit_size, config):
        self.surface = surface
        self.theme = theme
        self.config = config
        self.unit = unit_size
        self.pad = config.padding

        # Auto-scale font: ~32% of unit_size, or explicit config value
        font_size = config.font_size if config.font_size > 0 else max(12, int(unit_size * 0.32))
        font_small_size = max(10, font_size - 4)

        pygame.font.init()
        font_names = ["DejaVu Sans", "Liberation Sans", "FreeSans", "Arial"]
        self.font = None
        self.font_small = None
        for name in font_names:
            path = pygame.font.match_font(name)
            if path:
                self.font = pygame.font.Font(path, font_size)
                self.font_small = pygame.font.Font(path, font_small_size)
                break
        if not self.font:
            self.font = pygame.font.SysFont(None, font_size + 4)
            self.font_small = pygame.font.SysFont(None, font_size)

    def draw(self, keyboard):
        """Draw the full keyboard state."""
        self.surface.fill(self.theme.bg)

        layout = keyboard.layout
        y_offset = self.pad

        for row_idx, row in enumerate(layout):
            x_offset = self.pad
            for col_idx, key in enumerate(row):
                is_cursor = (row_idx == keyboard.cursor_row and
                             col_idx == keyboard.cursor_col)
                self._draw_key(key, x_offset, y_offset, is_cursor, keyboard)
                x_offset += int(key.width * self.unit) + self.pad
            y_offset += self.unit + self.pad

        # Draw accent popup if active
        if keyboard.accent_popup is not None:
            self._draw_accent_popup(keyboard)

        # Draw modifier indicators at the top-right
        self._draw_modifier_status(keyboard)

    def _draw_key(self, key, x, y, is_cursor, keyboard):
        w = int(key.width * self.unit)
        h = self.unit
        rect = pygame.Rect(x, y, w, h)

        # Determine colors
        if is_cursor:
            bg = self.theme.highlight_bg
            border = self.theme.highlight_border
        elif key.is_modifier and self._is_modifier_active(key, keyboard):
            bg = self.theme.modifier_active_bg
            border = self.theme.highlight_border
        elif key.is_modifier:
            bg = self.theme.modifier_bg
            border = self.theme.key_border
        elif key.label.startswith("F") and key.label[1:].isdigit():
            bg = self.theme.fn_key_bg
            border = self.theme.key_border
        else:
            bg = self.theme.key_bg
            border = self.theme.key_border

        # Draw rounded rect
        pygame.draw.rect(self.surface, bg, rect, border_radius=6)
        pygame.draw.rect(self.surface, border, rect, width=1, border_radius=6)

        # Draw label
        label = keyboard.get_display_label(key)
        text_color = self.theme.key_text
        if is_cursor:
            # Brighter text on highlight
            text_color = (255, 255, 255)

        text_surf = self.font.render(label, True, text_color)
        text_rect = text_surf.get_rect(center=rect.center)
        self.surface.blit(text_surf, text_rect)

        # Draw shift label hint (small, top-right) for non-cursor keys with shift labels
        if key.shift_label and not key.is_modifier and not is_cursor:
            shift_visible = keyboard.shift_active ^ keyboard.caps_active
            if not shift_visible:
                hint = key.shift_label
                hint_surf = self.font_small.render(
                    hint, True, self.theme.modifier_text)
                hint_rect = hint_surf.get_rect(topright=(rect.right - 4, rect.top + 2))
                self.surface.blit(hint_surf, hint_rect)

    def _is_modifier_active(self, key, keyboard):
        if key.modifier_type == "shift":
            return keyboard.shift_active
        elif key.modifier_type == "caps":
            return keyboard.caps_active
        elif key.modifier_type == "ctrl":
            return keyboard.ctrl_active
        elif key.modifier_type == "alt":
            return keyboard.alt_active
        elif key.modifier_type == "meta":
            return keyboard.meta_active
        return False

    def _draw_accent_popup(self, keyboard):
        """Draw horizontal accent selector above the current key."""
        accents, selected = keyboard.accent_popup
        row = keyboard.layout[keyboard.cursor_row]

        # Find cursor key position
        x_offset = self.pad
        for i in range(keyboard.cursor_col):
            x_offset += int(row[i].width * self.unit) + self.pad

        key_y = self.pad + keyboard.cursor_row * (self.unit + self.pad)
        popup_y = key_y - self.unit - self.pad

        # Draw popup background
        total_w = len(accents) * (self.unit + self.pad) + self.pad
        popup_rect = pygame.Rect(x_offset, popup_y, total_w, self.unit + self.pad * 2)
        pygame.draw.rect(self.surface, self.theme.accent_popup_bg, popup_rect,
                         border_radius=8)
        pygame.draw.rect(self.surface, self.theme.highlight_border, popup_rect,
                         width=2, border_radius=8)

        # Draw each accent option
        ax = x_offset + self.pad
        for i, (label, codepoint) in enumerate(accents):
            arect = pygame.Rect(ax, popup_y + self.pad, self.unit, self.unit)
            if i == selected:
                pygame.draw.rect(self.surface, self.theme.accent_highlight_bg,
                                 arect, border_radius=6)
            text = self.font.render(label, True, self.theme.accent_popup_text)
            text_rect = text.get_rect(center=arect.center)
            self.surface.blit(text, text_rect)
            ax += self.unit + self.pad

    def _draw_modifier_status(self, keyboard):
        """Draw small modifier indicators."""
        indicators = []
        if keyboard.shift_active:
            indicators.append("SHIFT")
        if keyboard.caps_active:
            indicators.append("CAPS")
        if keyboard.ctrl_active:
            indicators.append("CTRL")
        if keyboard.alt_active:
            indicators.append("ALT")
        if keyboard.meta_active:
            indicators.append("SUPER")

        if not indicators:
            return

        x = self.surface.get_width() - self.pad
        y = self.pad
        for label in indicators:
            text = self.font_small.render(label, True, self.theme.modifier_active_bg)
            text_rect = text.get_rect(topright=(x, y))
            # Background pill
            pill = text_rect.inflate(10, 4)
            pygame.draw.rect(self.surface, self.theme.modifier_active_bg, pill,
                             border_radius=4)
            text = self.font_small.render(label, True, (255, 255, 255))
            text_rect = text.get_rect(center=pill.center)
            self.surface.blit(text, text_rect)
            x = pill.left - 6


def calculate_unit_size(layout, screen_width, config):
    """Auto-scale unit_size so keyboard is ~70% of screen width.

    If config.unit_size > 0, use it as-is (explicit override).
    If 0, compute from screen resolution.
    """
    if config.unit_size > 0:
        return config.unit_size

    # Find the widest row in key-width units
    pad = config.padding
    max_units = 0
    max_keys = 0
    for row in layout:
        row_units = sum(k.width for k in row)
        if row_units > max_units:
            max_units = row_units
            max_keys = len(row)

    # Target 70% of screen width
    target_width = int(screen_width * 0.70)
    # Solve for unit: target = pad + sum(key.width * unit + pad) per key
    # ≈ (max_keys + 1) * pad + max_units * unit
    num_gaps = max_keys + 1
    unit = max(30, (target_width - num_gaps * pad) / max_units)
    return int(unit)


def calculate_window_size(layout, unit_size, padding):
    """Calculate window dimensions from layout and resolved unit_size."""
    max_row_width = 0
    for row in layout:
        row_width = padding
        for key in row:
            row_width += int(key.width * unit_size) + padding
        max_row_width = max(max_row_width, row_width)

    height = padding + len(layout) * (unit_size + padding)
    return max_row_width, height
