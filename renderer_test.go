package main

import "testing"

func TestShadowColorFromTheme(t *testing.T) {
	light := Themes["light"]
	if got := shadowColor(light); got != light.KeyShadow {
		t.Errorf("shadowColor(light) = %v, want theme value %v", got, light.KeyShadow)
	}
}

func TestShadowColorDerived(t *testing.T) {
	for name, theme := range Themes {
		if theme.KeyShadow != (Color{}) {
			continue
		}
		bg := theme.Bg
		got := shadowColor(theme)
		want := Color{R: bg.R / 2, G: bg.G / 2, B: bg.B / 2, A: 255}
		if got != want {
			t.Errorf("theme %q: derived shadow %v, want 50%% of bg %v = %v", name, got, bg, want)
		}
		if got.A != 255 {
			t.Errorf("theme %q: derived shadow alpha = %d, want 255", name, got.A)
		}
		if got.R > bg.R || got.G > bg.G || got.B > bg.B {
			t.Errorf("theme %q: derived shadow %v is not darker than bg %v", name, got, bg)
		}
		bgBlack := bg.R == 0 && bg.G == 0 && bg.B == 0
		black := got.R == 0 && got.G == 0 && got.B == 0
		if !bgBlack && (black || got == Color{R: bg.R, G: bg.G, B: bg.B, A: 255}) {
			t.Errorf("theme %q: derived shadow %v not visibly darker than bg %v", name, got, bg)
		}
	}
}

func TestShadowOffset(t *testing.T) {
	tests := []struct {
		unit, pad, want int32
	}{
		{40, 4, 1},   // small key: at least 1 px
		{80, 4, 2},   // unit/40
		{200, 4, 3},  // capped at pad-1
		{200, 10, 5}, // unit/40 below the cap
		{80, 1, 0},   // gap too small: no shadow
		{80, 0, 0},
	}
	for _, tt := range tests {
		if got := shadowOffset(tt.unit, tt.pad); got != tt.want {
			t.Errorf("shadowOffset(%d, %d) = %d, want %d", tt.unit, tt.pad, got, tt.want)
		}
	}
}

func TestGlowThemesSetShadow(t *testing.T) {
	// A shadow derived from a black or near-black background is invisible,
	// so these themes set their own glow color.
	glow := []string{
		"neon", "cga", "zx_spectrum", "high_contrast", // border color
		"matrix", "retro", "terminal", "virtualboy", // 60% of label color
		"cyberpunk", "midnight", "sunset", // 60% of label color
		"ember", "synthwave", // 60% of accent color, raised to 2.75:1
		// Dark themes whose derived shadow barely shows
		"ayu_dark", "wine", "gotham", "navy", "teal", "copper", // accent, raised to 2.75:1
		"oxocarbon", "olive", "forest", "coral", // 60% of label color
		"gold", "nightfox", "vapor", "ice", // 60% of label color
	}
	for _, name := range glow {
		if Themes[name].KeyShadow == (Color{}) {
			t.Errorf("theme %q: KeyShadow not set", name)
		}
	}
	// Any pure black background needs one too.
	for name, theme := range Themes {
		if max(theme.Bg.R, theme.Bg.G, theme.Bg.B) <= 5 && theme.KeyShadow == (Color{}) {
			t.Errorf("theme %q has a black background but no KeyShadow", name)
		}
	}
}

func TestContrastRatio(t *testing.T) {
	black, white := c(0, 0, 0), c(255, 255, 255)
	if got := contrastRatio(black, white); got < 20.99 || got > 21.01 {
		t.Errorf("black on white = %.3f, want 21", got)
	}
	if got := contrastRatio(white, black); got < 20.99 || got > 21.01 {
		t.Errorf("white on black = %.3f, want 21 (order must not matter)", got)
	}
	if got := contrastRatio(white, white); got != 1 {
		t.Errorf("white on white = %.3f, want 1", got)
	}
}

func TestReadableText(t *testing.T) {
	black, white := c(0, 0, 0), c(255, 255, 255)
	if got := readableText(white, c(38, 139, 210)); got != white {
		t.Errorf("white on dark blue = %v, want white kept", got)
	}
	if got := readableText(white, c(255, 255, 0)); got != black {
		t.Errorf("white on yellow = %v, want black", got)
	}
	if got := readableText(c(60, 60, 60), c(20, 20, 20)); got != white {
		t.Errorf("dark grey on near-black = %v, want white", got)
	}
}

func TestHighlightTextReadable(t *testing.T) {
	// Every theme's cursor label and selected accent must reach 3:1, keep
	// white where white passes, and match each other.
	for name, theme := range Themes {
		r := &Renderer{theme: theme}
		r.setTextColors()
		if got := contrastRatio(r.cursorText, theme.HighlightBg); got < minTextContrast {
			t.Errorf("theme %q: cursor text %.2f:1", name, got)
		}
		if got := contrastRatio(r.accentSelText, theme.AccentHighlightBg); got < minTextContrast {
			t.Errorf("theme %q: selected accent text %.2f:1", name, got)
		}
		white := c(255, 255, 255)
		if contrastRatio(white, theme.HighlightBg) >= minTextContrast && r.cursorText != white {
			t.Errorf("theme %q: cursor text changed from white though white passes", name)
		}
		if contrastRatio(white, theme.AccentHighlightBg) >= minTextContrast && r.accentSelText != white {
			t.Errorf("theme %q: selected accent text changed from white though white passes", name)
		}
		// The selected accent sits right above the cursor key, so their
		// text must match when their backgrounds do.
		if theme.HighlightBg == theme.AccentHighlightBg && r.cursorText != r.accentSelText {
			t.Errorf("theme %q: cursor text %v differs from selected accent text %v", name, r.cursorText, r.accentSelText)
		}
	}
}

func TestPopupBorderPicksStrongerHighlight(t *testing.T) {
	// The popup border is whichever highlight color contrasts more with the
	// popup background, which is the gap color.
	for name, theme := range Themes {
		r := &Renderer{theme: theme}
		r.setTextColors()
		got := contrastRatio(r.popupBorder, theme.Bg)
		for _, alt := range []Color{theme.HighlightBg, theme.HighlightBorder} {
			if contrastRatio(alt, theme.Bg) > got {
				t.Errorf("theme %q: popup border %v, but %v contrasts more", name, r.popupBorder, alt)
			}
		}
	}
}
