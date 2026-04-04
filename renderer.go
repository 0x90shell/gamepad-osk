package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

const promptFontPath = "/usr/share/fonts/TTF/promptfont.ttf"

type Renderer struct {
	renderer       *sdl.Renderer
	theme          Theme
	unit           int32
	pad            int32
	statusH        int32 // height of modifier status bar
	font           *ttf.Font
	fontSmall      *ttf.Font
	fontGlyph      *ttf.Font
	flashText string
	flashEnd  time.Time
}

func NewRenderer(r *sdl.Renderer, theme Theme, unitSize, padding int32) (*Renderer, error) {
	fontSize := max32(12, int32(float64(unitSize)*0.32))
	glyphSize := max32(12, int32(float64(unitSize)*0.28))

	if err := ttf.Init(); err != nil {
		return nil, err
	}

	fontPath := findFont("DejaVu Sans", "Liberation Sans", "FreeSans")
	if fontPath == "" {
		return nil, fmt.Errorf("no suitable font found")
	}

	font, err := ttf.OpenFont(fontPath, int(fontSize))
	if err != nil {
		return nil, err
	}
	fontSmall, err := ttf.OpenFont(fontPath, int(max32(10, fontSize-4)))
	if err != nil {
		return nil, err
	}

	var fontGlyph *ttf.Font
	if _, err := os.Stat(promptFontPath); err == nil {
		fontGlyph, _ = ttf.OpenFont(promptFontPath, int(glyphSize))
	}
	if fontGlyph == nil {
		fontGlyph, _ = ttf.OpenFont(fontPath, int(max32(10, fontSize-6)))
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
	r.renderer.Clear()

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

	r.renderer.Present()
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
		surf, err := r.fontSmall.RenderUTF8Blended(label, tc)
		if err == nil {
			x += surf.W + 10 + 6
			surf.Free()
		}
	}

	// Status flash (right-aligned, 2 seconds)
	if r.flashText != "" && time.Now().Before(r.flashEnd) {
		sw, _, _ := r.renderer.GetOutputSize()
		surf, err := r.fontSmall.RenderUTF8Blended(r.flashText, r.theme.KeyText)
		if err == nil {
			tex, _ := r.renderer.CreateTextureFromSurface(surf)
			if tex != nil {
				dst := sdl.Rect{X: sw - surf.W - r.pad, Y: r.pad + 2, W: surf.W, H: surf.H}
				r.renderer.Copy(tex, nil, &dst)
				tex.Destroy()
			}
			surf.Free()
		}
	}
}

func (r *Renderer) drawPill(label string, x, y int32, bg sdl.Color, fg sdl.Color) {
	surf, err := r.fontSmall.RenderUTF8Blended(label, fg)
	if err != nil {
		return
	}
	defer surf.Free()
	pw := surf.W + 10
	ph := surf.H + 4
	pill := sdl.Rect{X: x, Y: y, W: pw, H: ph}
	setColor(r.renderer, bg)
	r.renderer.FillRect(&pill)
	tex, _ := r.renderer.CreateTextureFromSurface(surf)
	if tex != nil {
		dst := sdl.Rect{X: x + 5, Y: y + 2, W: surf.W, H: surf.H}
		r.renderer.Copy(tex, nil, &dst)
		tex.Destroy()
	}
}

func (r *Renderer) drawKey(key KeyDef, x, y int32, isCursor bool, kb *KeyboardState) {
	w := int32(key.Width * float64(r.unit))
	h := r.unit
	rect := sdl.Rect{X: x, Y: y, W: w, H: h}
	t := r.theme

	flashed := kb.IsFlashed(key)
	var bg, border sdl.Color
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
	r.renderer.FillRect(&rect)
	// Border
	setColor(r.renderer, border)
	r.renderer.DrawRect(&rect)

	// Label
	label := kb.DisplayLabel(key)
	tc := t.KeyText
	if isCursor || flashed {
		tc = sdl.Color{R: 255, G: 255, B: 255, A: 255}
	} else if key.IsModifier && isModActive(key, kb) {
		tc = t.ModifierActiveText
	}
	r.renderText(r.font, label, tc, rect, AlignCenter)

	// Shift hint (top-right)
	if key.ShiftLabel != "" && !key.IsModifier && !isCursor {
		if kb.ShiftActive == kb.CapsActive {
			r.renderText(r.fontSmall, key.ShiftLabel, t.ModifierText,
				sdl.Rect{X: x, Y: y, W: w - 4, H: h}, AlignTopRight)
		}
	}

	// Controller glyph (bottom-right)
	if glyph, ok := KeyGlyphs[key.Code]; ok && r.fontGlyph != nil {
		r.renderText(r.fontGlyph, glyph, t.GlyphColor,
			sdl.Rect{X: x, Y: y, W: w - 3, H: h - 2}, AlignBottomRight)
	}
}

func (r *Renderer) drawAccentPopup(kb *KeyboardState) {
	accents := kb.AccentPopup.Accents
	sel := kb.AccentPopup.Selected
	row := kb.Layout[kb.CursorRow]

	xo := r.pad
	for i := 0; i < kb.CursorCol; i++ {
		xo += int32(row[i].Width*float64(r.unit)) + r.pad
	}
	ky := r.pad + r.statusH + int32(kb.CursorRow)*(r.unit+r.pad)
	py := ky - r.unit - r.pad

	tw := int32(len(accents))*(r.unit+r.pad) + r.pad
	pr := sdl.Rect{X: xo, Y: py, W: tw, H: r.unit + r.pad*2}
	setColor(r.renderer, r.theme.AccentPopupBg)
	r.renderer.FillRect(&pr)
	setColor(r.renderer, r.theme.HighlightBorder)
	r.renderer.DrawRect(&pr)

	ax := xo + r.pad
	for i, accent := range accents {
		ar := sdl.Rect{X: ax, Y: py + r.pad, W: r.unit, H: r.unit}
		if i == sel {
			setColor(r.renderer, r.theme.AccentHighlightBg)
			r.renderer.FillRect(&ar)
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

func (r *Renderer) renderText(font *ttf.Font, text string, color sdl.Color, rect sdl.Rect, align TextAlign) {
	if text == "" || font == nil {
		return
	}
	surf, err := font.RenderUTF8Blended(text, color)
	if err != nil {
		return
	}
	defer surf.Free()

	tex, err := r.renderer.CreateTextureFromSurface(surf)
	if err != nil {
		return
	}
	defer tex.Destroy()

	var dst sdl.Rect
	switch align {
	case AlignCenter:
		dst = sdl.Rect{
			X: rect.X + (rect.W-surf.W)/2,
			Y: rect.Y + (rect.H-surf.H)/2,
			W: surf.W, H: surf.H,
		}
	case AlignTopRight:
		dst = sdl.Rect{
			X: rect.X + rect.W - surf.W,
			Y: rect.Y + 2,
			W: surf.W, H: surf.H,
		}
	case AlignBottomRight:
		dst = sdl.Rect{
			X: rect.X + rect.W - surf.W,
			Y: rect.Y + rect.H - surf.H,
			W: surf.W, H: surf.H,
		}
	}
	r.renderer.Copy(tex, nil, &dst)
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
		r.font.Close()
	}
	if r.fontSmall != nil {
		r.fontSmall.Close()
	}
	if r.fontGlyph != nil {
		r.fontGlyph.Close()
	}
}

func CalcUnitSize(layout [][]KeyDef, screenWidth int32, cfg Config) int32 {
	if cfg.Keys.UnitSize > 0 {
		return int32(cfg.Keys.UnitSize)
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
	h := pad + statusH + int32(len(layout))*(unit+pad)
	return maxW, h
}

func setColor(r *sdl.Renderer, c sdl.Color) {
	r.SetDrawColor(c.R, c.G, c.B, c.A)
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
