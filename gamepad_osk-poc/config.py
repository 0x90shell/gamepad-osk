"""Configuration loading with defaults."""

import os
import sys

if sys.version_info >= (3, 11):
    import tomllib
else:
    try:
        import tomli as tomllib
    except ImportError:
        tomllib = None


DEFAULTS = {
    "theme": {"name": "dark"},
    "window": {"bottom_margin": 20, "opacity": 0.95},
    "keys": {"unit_size": 0, "padding": 4, "font_size": 0},  # 0 = auto-scale
    "gamepad": {
        "device": "",
        "grab": True,
        "deadzone": 0.25,
        "long_press_ms": 500,
    },
    "mouse": {"enabled": True, "sensitivity": 8},
}

CONFIG_PATHS = [
    os.path.expanduser("~/.config/gamepad-osk/config.toml"),
    os.path.join(os.path.dirname(os.path.dirname(__file__)), "config.toml"),
]


def _deep_merge(base, override):
    """Merge override into base, returning new dict."""
    result = dict(base)
    for k, v in override.items():
        if k in result and isinstance(result[k], dict) and isinstance(v, dict):
            result[k] = _deep_merge(result[k], v)
        else:
            result[k] = v
    return result


class Config:
    """Flat attribute access to nested config dict."""

    def __init__(self, data):
        self._data = data

    @property
    def theme_name(self):
        return self._data["theme"]["name"]

    @property
    def bottom_margin(self):
        return self._data["window"]["bottom_margin"]

    @property
    def opacity(self):
        return self._data["window"]["opacity"]

    @property
    def unit_size(self):
        return self._data["keys"]["unit_size"]

    @property
    def padding(self):
        return self._data["keys"]["padding"]

    @property
    def font_size(self):
        return self._data["keys"]["font_size"]

    @property
    def device(self):
        return self._data["gamepad"]["device"]

    @property
    def grab(self):
        return self._data["gamepad"]["grab"]

    @property
    def deadzone(self):
        return self._data["gamepad"]["deadzone"]

    @property
    def long_press_ms(self):
        return self._data["gamepad"]["long_press_ms"]

    @property
    def mouse_enabled(self):
        return self._data["mouse"]["enabled"]

    @property
    def mouse_sensitivity(self):
        return self._data["mouse"]["sensitivity"]


def load_config():
    """Load config from file, merged with defaults."""
    data = dict(DEFAULTS)

    for path in CONFIG_PATHS:
        if os.path.isfile(path):
            if tomllib is None:
                print(f"Warning: cannot parse {path} (no tomllib)", file=sys.stderr)
                break
            with open(path, "rb") as f:
                user = tomllib.load(f)
            data = _deep_merge(data, user)
            break

    return Config(data)
