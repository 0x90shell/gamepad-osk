package main

import "testing"

func TestGlyphsFollowEveryRemap(t *testing.T) {
	for _, swap := range []string{"false", "true", "auto"} {
		table := buttonTable(swap == "true")
		cfg := DefaultConfig()
		cfg.Gamepad.SwapXY = swap
		b := &cfg.Gamepad.Buttons
		b.Backspace, b.Space, b.Enter, b.Shift = "lb", "rb", "select", "y"
		cfg.Gamepad.MouseStick = "left"
		g := BuildKeyGlyphs(cfg.Gamepad)
		want := map[int]string{KEY_BACKSPACE: "lb", KEY_SPACE: "rb", KEY_ENTER: "select",
			KEY_LEFTSHIFT: "y", KEY_RIGHTSHIFT: "y", KEY_CAPSLOCK: "r3"}
		for code, name := range want {
			if g[code].Glyph != table[name].Glyph || g[code].Text != upper(name) {
				t.Errorf("swap=%s key %d = %+v, want %s", swap, code, g[code], name)
			}
		}
	}
}

// The glyph names the button, so swap_xy must not change it.
func TestGlyphIgnoresSwap(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gamepad.SwapXY = "false"
	off := BuildKeyGlyphs(cfg.Gamepad)
	cfg.Gamepad.SwapXY = "true"
	on := BuildKeyGlyphs(cfg.Gamepad)
	for _, k := range []int{KEY_BACKSPACE, KEY_SPACE} {
		if off[k] != on[k] {
			t.Errorf("key %d: swap off %+v, on %+v", k, off[k], on[k])
		}
	}
}

// A clash makes ValidateConfig fall back to the default buttons; the glyphs must follow.
func TestGlyphsAfterClashFallback(t *testing.T) {
	def := BuildKeyGlyphs(DefaultConfig().Gamepad)
	cfg := DefaultConfig()
	cfg.Gamepad.Buttons.Enter, cfg.Gamepad.Buttons.LeftClick = "rb", "rb"
	ValidateConfig(&cfg)
	g := BuildKeyGlyphs(cfg.Gamepad)
	if g[KEY_ENTER] != def[KEY_ENTER] {
		t.Errorf("enter after clash = %+v, want default %+v", g[KEY_ENTER], def[KEY_ENTER])
	}
}

func TestPopupHintAllButtons(t *testing.T) {
	table := buttonTable(false)
	for _, pair := range [][2]string{{"lb", "rb"}, {"rt", "select"}, {"x", "a"}} {
		cfg := DefaultConfig()
		cfg.Gamepad.Buttons.Press, cfg.Gamepad.Buttons.Close = pair[0], pair[1]
		h := BuildPopupHint(cfg.Gamepad)
		if len(h) != 3 || h[1].Glyph != table[pair[0]].Glyph || h[2].Glyph != table[pair[1]].Glyph {
			t.Errorf("press=%s close=%s: hint %+v", pair[0], pair[1], h)
		}
	}
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}
