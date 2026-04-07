package main

import (
	"errors"
	"os"
	"strings"
	"time"
)

const promptFontPath = "/usr/share/fonts/TTF/promptfont.ttf"

type Renderer struct {
	renderer       *SDLRenderer
	theme          Theme
	unit           int32
	pad            int32
	statusH        int32 // height of modifier status bar
	font           *Font
	fontSmall      *Font
	fontGlyph      *Font
	flashText string
	flashEnd  time.Time
}

func NewRenderer(r *SDLRenderer, theme Theme, unitSize, padding int32) (*Renderer, error) {
	fontSize := max32(12, int32(float64(unitSize)*0.32))
	glyphSize := max32(12, int32(float64(unitSize)*0.28))

	if err := TTF3Init(); err != nil {
		return nil, err
	}

	fontPath := findFont("DejaVu Sans", "Liberation Sans", "FreeSans")
	if fontPath == "" {
		return nil, errors.New("no suitable font found")
	}

	font, err := TTF3OpenFont(fontPath, float32(fontSize))
	if err != nil {
		return nil, err
	}
	fontSmall, err := TTF3OpenFont(fontPath, float32(max32(10, fontSize-4)))
	if err != nil {
		return nil, err
	}

	var fontGlyph *Font
	if _, err := os.Stat(promptFontPath); err == nil {
		fontGlyph, _ = TTF3OpenFont(promptFontPath, float32(glyphSize))
	}
	if fontGlyph == nil {
		fontGlyph, _ = TTF3OpenFont(fontPath, float32(max32(10, fontSize-6)))
	}

	return &Renderer{
		renderer:  r,
		theme:     theme,
		unit:      unitSize,
		pad:       padding,
		statusH:   max32(20, int32(float64(unitSize)*0.4)),
		font:      font,
		fontSmall: fontSmall,
		fontGlyph: fontGlyph,
	}, nil
}

func (r *Renderer) Draw(kb *KeyboardState) {
	t := r.theme
	setColor(r.renderer, t.Bg)
	SDL3RenderClear(r.renderer)

	// Draw modifier status bar
	r.drawModifierBar(kb)

	// Draw keyboard rows
	y := r.pad + r.statusH
	for ri, row := range kb.Layout {
		x := r.pad
		for ci, key := range row {
			isCursor := ri == kb.CursorRow && ci == kb.CursorCol
			r.drawKey(key, x, y, isCursor, kb)
			x += int32(key.Width*float64(r.unit)) + r.pad
		}
		y += r.unit + r.pad
	}

	// Accent popup
	if kb.AccentPopup != nil {
		r.drawAccentPopup(kb)
	}

	SDL3RenderPresent(r.renderer)
}

func (r *Renderer) drawModifierBar(kb *KeyboardState) {
	// Modifier pills (left-aligned)
	var labels []string
	if kb.ShiftActive {
		labels = append(labels, "SHIFT")
	}
	if kb.CapsActive {
		labels = append(labels, "CAPS")
	}
	if kb.CtrlActive {
		labels = append(labels, "CTRL")
	}
	if kb.AltActive {
		labels = append(labels, "ALT")
	}
	if kb.MetaActive {
		labels = append(labels, "SUPER")
	}

	x := r.pad
	tc := r.theme.ModifierActiveText
	for _, label := range labels {
		r.drawPill(label, x, r.pad, r.theme.ModifierActiveBg, tc)
		tw, _ := TTF3GetStringSize(r.fontSmall, label)
		x += tw + 10 + 6
	}

	// Status flash (right-aligned, 2 seconds)
	if r.flashText != "" && time.Now().Before(r.flashEnd) {
		sw, _ := SDL3GetRenderOutputSize(r.renderer)
		surf, err := TTF3RenderTextBlended(r.fontSmall, r.flashText, r.theme.KeyText)
		if err == nil {
			tex := SDL3CreateTextureFromSurface(r.renderer, surf)
			if tex != nil {
				surfW := SDL3SurfaceWidth(surf)
				surfH := SDL3SurfaceHeight(surf)
				dst := FRect{X: float32(sw - surfW - r.pad), Y: float32(r.pad + 2), W: float32(surfW), H: float32(surfH)}
				SDL3RenderTexture(r.renderer, tex, nil, &dst)
				SDL3DestroyTexture(tex)
			}
			SDL3DestroySurface(surf)
		}
	}
}

func (r *Renderer) drawPill(label string, x, y int32, bg Color, fg Color) {
	surf, err := TTF3RenderTextBlended(r.fontSmall, label, fg)
	if err != nil {
		return
	}
	surfW := SDL3SurfaceWidth(surf)
	surfH := SDL3SurfaceHeight(surf)
	defer SDL3DestroySurface(surf)
	pw := surfW + 10
	ph := surfH + 4
	pill := FRect{X: float32(x), Y: float32(y), W: float32(pw), H: float32(ph)}
	setColor(r.renderer, bg)
	SDL3RenderFillRect(r.renderer, pill)
	tex := SDL3CreateTextureFromSurface(r.renderer, surf)
	if tex != nil {
		dst := FRect{X: float32(x + 5), Y: float32(y + 2), W: float32(surfW), H: float32(surfH)}
		SDL3RenderTexture(r.renderer, tex, nil, &dst)
		SDL3DestroyTexture(tex)
	}
}

func (r *Renderer) drawKey(key KeyDef, x, y int32, isCursor bool, kb *KeyboardState) {
	w := int32(key.Width * float64(r.unit))
	h := r.unit
	rect := FRect{X: float32(x), Y: float32(y), W: float32(w), H: float32(h)}
	t := r.theme

	flashed := kb.IsFlashed(key)
	var bg, border Color
	switch {
	case isCursor || flashed:
		bg, border = t.HighlightBg, t.HighlightBorder
	case key.IsModifier && isModActive(key, kb):
		bg, border = t.ModifierActiveBg, t.HighlightBorder
	case key.IsModifier:
		bg, border = t.ModifierBg, t.KeyBorder
	case strings.HasPrefix(key.Label, "F") && len(key.Label) > 1 && key.Label[1] >= '0' && key.Label[1] <= '9':
		bg, border = t.FnKeyBg, t.KeyBorder
	default:
		bg, border = t.KeyBg, t.KeyBorder
	}

	// Fill
	setColor(r.renderer, bg)
	SDL3RenderFillRect(r.renderer, rect)
	// Border
	setColor(r.renderer, border)
	SDL3RenderRect(r.renderer, rect)

	// Label
	label := kb.DisplayLabel(key)
	tc := t.KeyText
	if isCursor || flashed {
		tc = Color{R: 255, G: 255, B: 255, A: 255}
	} else if key.IsModifier && isModActive(key, kb) {
		tc = t.ModifierActiveText
	}
	r.renderText(r.font, label, tc, rect, AlignCenter)

	// Shift hint (top-right)
	if key.ShiftLabel != "" && !key.IsModifier && !isCursor {
		if kb.ShiftActive == kb.CapsActive {
			r.renderText(r.fontSmall, key.ShiftLabel, t.ModifierText,
				FRect{X: float32(x), Y: float32(y), W: float32(w - 4), H: float32(h)}, AlignTopRight)
		}
	}

	// Controller glyph (bottom-right)
	if glyph, ok := KeyGlyphs[key.Code]; ok && r.fontGlyph != nil {
		r.renderText(r.fontGlyph, glyph, t.GlyphColor,
			FRect{X: float32(x), Y: float32(y), W: float32(w - 3), H: float32(h - 2)}, AlignBottomRight)
	}
}

func (r *Renderer) drawAccentPopup(kb *KeyboardState) {
	accents := kb.AccentPopup.Accents
	sel := kb.AccentPopup.Selected
	row := kb.Layout[kb.CursorRow]

	xo := r.pad
	for i := range kb.CursorCol {
		xo += int32(row[i].Width*float64(r.unit)) + r.pad
	}
	ky := r.pad + r.statusH + int32(kb.CursorRow)*(r.unit+r.pad) //nolint:gosec // G115: cursor index fits in int32
	py := ky - r.unit - r.pad

	tw := int32(len(accents))*(r.unit+r.pad) + r.pad //nolint:gosec // G115: accent count fits in int32
	pr := FRect{X: float32(xo), Y: float32(py), W: float32(tw), H: float32(r.unit + r.pad*2)}
	setColor(r.renderer, r.theme.AccentPopupBg)
	SDL3RenderFillRect(r.renderer, pr)
	setColor(r.renderer, r.theme.HighlightBorder)
	SDL3RenderRect(r.renderer, pr)

	ax := xo + r.pad
	for i, accent := range accents {
		ar := FRect{X: float32(ax), Y: float32(py + r.pad), W: float32(r.unit), H: float32(r.unit)}
		if i == sel {
			setColor(r.renderer, r.theme.AccentHighlightBg)
			SDL3RenderFillRect(r.renderer, ar)
		}
		r.renderText(r.font, accent.Label, r.theme.AccentPopupText, ar, AlignCenter)
		ax += r.unit + r.pad
	}
}

type TextAlign int

const (
	AlignCenter TextAlign = iota
	AlignTopRight
	AlignBottomRight
)

func (r *Renderer) renderText(font *Font, text string, color Color, rect FRect, align TextAlign) {
	if text == "" || font == nil {
		return
	}
	surf, err := TTF3RenderTextBlended(font, text, color)
	if err != nil {
		return
	}
	defer SDL3DestroySurface(surf)

	tex := SDL3CreateTextureFromSurface(r.renderer, surf)
	if tex == nil {
		return
	}
	defer SDL3DestroyTexture(tex)

	surfW := float32(SDL3SurfaceWidth(surf))
	surfH := float32(SDL3SurfaceHeight(surf))

	var dst FRect
	switch align {
	case AlignCenter:
		dst = FRect{
			X: rect.X + (rect.W-surfW)/2,
			Y: rect.Y + (rect.H-surfH)/2,
			W: surfW, H: surfH,
		}
	case AlignTopRight:
		dst = FRect{
			X: rect.X + rect.W - surfW,
			Y: rect.Y + 2,
			W: surfW, H: surfH,
		}
	case AlignBottomRight:
		dst = FRect{
			X: rect.X + rect.W - surfW,
			Y: rect.Y + rect.H - surfH,
			W: surfW, H: surfH,
		}
	}
	SDL3RenderTexture(r.renderer, tex, nil, &dst)
}

func (r *Renderer) SetTheme(t Theme) {
	r.theme = t
	r.Flash(t.Name)
}

func (r *Renderer) Flash(text string) {
	r.flashText = text
	r.flashEnd = time.Now().Add(2 * time.Second)
}

func (r *Renderer) Destroy() {
	if r.font != nil {
		TTF3CloseFont(r.font)
	}
	if r.fontSmall != nil {
		TTF3CloseFont(r.fontSmall)
	}
	if r.fontGlyph != nil {
		TTF3CloseFont(r.fontGlyph)
	}
}

func CalcUnitSize(layout [][]KeyDef, screenWidth int32, cfg Config) int32 {
	if cfg.Keys.UnitSize > 0 {
		return int32(cfg.Keys.UnitSize) //nolint:gosec // G115: unit size fits in int32
	}
	var maxUnits float64
	var maxKeys int
	for _, row := range layout {
		ru := 0.0
		for _, k := range row {
			ru += k.Width
		}
		if ru > maxUnits {
			maxUnits = ru
			maxKeys = len(row)
		}
	}
	scale := float64(cfg.Keys.Scale) / 100.0
	if scale <= 0 {
		scale = 0.70
	}
	target := float64(screenWidth) * scale
	pad := float64(cfg.Keys.Padding)
	unit := (target - float64(maxKeys+1)*pad) / maxUnits
	if unit < 30 {
		unit = 30
	}
	return int32(unit)
}

func CalcWindowSize(layout [][]KeyDef, unit, pad, statusH int32) (int32, int32) {
	var maxW int32
	for _, row := range layout {
		rw := pad
		for _, key := range row {
			rw += int32(key.Width*float64(unit)) + pad
		}
		if rw > maxW {
			maxW = rw
		}
	}
	h := pad + statusH + int32(len(layout))*(unit+pad) //nolint:gosec // G115: layout row count fits in int32
	return maxW, h
}

func setColor(r *SDLRenderer, c Color) {
	SDL3SetRenderDrawColor(r, c)
}

func isModActive(key KeyDef, kb *KeyboardState) bool {
	switch key.ModifierType {
	case "shift":
		return kb.ShiftActive
	case "caps":
		return kb.CapsActive
	case "ctrl":
		return kb.CtrlActive
	case "alt":
		return kb.AltActive
	case "meta":
		return kb.MetaActive
	}
	return false
}

func findFont(names ...string) string {
	// Common font directories on Linux
	dirs := []string{
		"/usr/share/fonts/TTF",
		"/usr/share/fonts/truetype/dejavu",
		"/usr/share/fonts/truetype/liberation",
		"/usr/share/fonts/truetype/freefont",
		"/usr/share/fonts/truetype",
		"/usr/share/fonts",
	}
	filePatterns := map[string][]string{
		"DejaVu Sans":    {"DejaVuSans.ttf"},
		"Liberation Sans": {"LiberationSans-Regular.ttf"},
		"FreeSans":        {"FreeSans.ttf"},
	}

	for _, name := range names {
		patterns := filePatterns[name]
		for _, dir := range dirs {
			for _, pat := range patterns {
				path := dir + "/" + pat
				if _, err := os.Stat(path); err == nil {
					return path
				}
			}
		}
	}
	return ""
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}
