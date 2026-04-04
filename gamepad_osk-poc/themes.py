"""Color theme definitions for the on-screen keyboard."""

from dataclasses import dataclass


@dataclass(frozen=True)
class Theme:
    name: str
    bg: tuple
    key_bg: tuple
    key_bg_pressed: tuple
    key_border: tuple
    key_text: tuple
    highlight_bg: tuple
    highlight_border: tuple
    modifier_bg: tuple
    modifier_active_bg: tuple
    modifier_text: tuple
    fn_key_bg: tuple
    accent_popup_bg: tuple
    accent_popup_text: tuple
    accent_highlight_bg: tuple


THEMES = {
    "dark": Theme(
        name="dark",
        bg=(30, 30, 35),
        key_bg=(55, 55, 65),
        key_bg_pressed=(80, 80, 95),
        key_border=(70, 70, 80),
        key_text=(220, 220, 230),
        highlight_bg=(60, 110, 180),
        highlight_border=(80, 140, 220),
        modifier_bg=(50, 50, 60),
        modifier_active_bg=(90, 70, 130),
        modifier_text=(180, 180, 200),
        fn_key_bg=(45, 45, 55),
        accent_popup_bg=(50, 50, 60),
        accent_popup_text=(220, 220, 230),
        accent_highlight_bg=(60, 110, 180),
    ),
    "steam_green": Theme(
        name="steam_green",
        bg=(22, 32, 22),
        key_bg=(35, 55, 35),
        key_bg_pressed=(50, 80, 50),
        key_border=(45, 70, 45),
        key_text=(200, 220, 200),
        highlight_bg=(56, 150, 60),
        highlight_border=(76, 185, 80),
        modifier_bg=(30, 48, 30),
        modifier_active_bg=(40, 120, 45),
        modifier_text=(170, 200, 170),
        fn_key_bg=(28, 44, 28),
        accent_popup_bg=(32, 50, 32),
        accent_popup_text=(200, 220, 200),
        accent_highlight_bg=(56, 150, 60),
    ),
    "candy": Theme(
        name="candy",
        bg=(40, 18, 55),
        key_bg=(70, 35, 90),
        key_bg_pressed=(100, 55, 125),
        key_border=(85, 45, 110),
        key_text=(240, 210, 250),
        highlight_bg=(210, 80, 140),
        highlight_border=(240, 110, 170),
        modifier_bg=(60, 28, 78),
        modifier_active_bg=(180, 60, 120),
        modifier_text=(220, 190, 230),
        fn_key_bg=(55, 25, 72),
        accent_popup_bg=(65, 30, 85),
        accent_popup_text=(240, 210, 250),
        accent_highlight_bg=(210, 80, 140),
    ),
    "ocean": Theme(
        name="ocean",
        bg=(15, 25, 45),
        key_bg=(25, 45, 75),
        key_bg_pressed=(35, 65, 105),
        key_border=(35, 55, 90),
        key_text=(190, 215, 240),
        highlight_bg=(30, 120, 190),
        highlight_border=(50, 150, 220),
        modifier_bg=(20, 38, 65),
        modifier_active_bg=(25, 100, 170),
        modifier_text=(170, 200, 230),
        fn_key_bg=(20, 35, 60),
        accent_popup_bg=(22, 40, 70),
        accent_popup_text=(190, 215, 240),
        accent_highlight_bg=(30, 120, 190),
    ),
    "solarized": Theme(
        name="solarized",
        bg=(0, 43, 54),
        key_bg=(7, 54, 66),
        key_bg_pressed=(88, 110, 117),
        key_border=(0, 54, 66),
        key_text=(147, 161, 161),
        highlight_bg=(38, 139, 210),
        highlight_border=(42, 161, 152),
        modifier_bg=(0, 43, 54),
        modifier_active_bg=(133, 153, 0),
        modifier_text=(131, 148, 150),
        fn_key_bg=(0, 38, 48),
        accent_popup_bg=(7, 54, 66),
        accent_popup_text=(147, 161, 161),
        accent_highlight_bg=(38, 139, 210),
    ),
    "high_contrast": Theme(
        name="high_contrast",
        bg=(0, 0, 0),
        key_bg=(20, 20, 20),
        key_bg_pressed=(60, 60, 60),
        key_border=(200, 200, 200),
        key_text=(255, 255, 255),
        highlight_bg=(255, 255, 0),
        highlight_border=(255, 255, 255),
        modifier_bg=(10, 10, 10),
        modifier_active_bg=(200, 0, 0),
        modifier_text=(255, 255, 255),
        fn_key_bg=(15, 15, 15),
        accent_popup_bg=(20, 20, 20),
        accent_popup_text=(255, 255, 255),
        accent_highlight_bg=(255, 255, 0),
    ),
}


def get_theme(name):
    return THEMES.get(name, THEMES["dark"])
