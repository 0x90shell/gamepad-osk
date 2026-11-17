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
