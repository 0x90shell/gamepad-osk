package main

import (
	"errors"
	"math"
	"strings"
	"time"
	"unsafe"
)

type texCacheKey struct {
	text  string
	font  uintptr // font pointer as identity
	color Color
}

type texCacheEntry struct {
	tex  *Texture
	w, h float32
}

type Renderer struct {
	renderer       *SDLRenderer
	theme          Theme
	unit           int32
	pad            int32
	statusH        int32 // height of modifier status bar
	font           *Font
	fontSmall      *Font
	fontGlyph      *Font
	hasPromptfont  bool // fontGlyph is Promptfont, not the label font fallback
	flashText      string
	flashGlyphText string // rendered with Promptfont, placed before flashText
	flashEnd       time.Time
	texCache       map[texCacheKey]texCacheEntry
	dirtyFrames    int                    // redraw counter for double/triple buffering
	keyShadow      bool                   // draw a drop shadow under keys and the accent popup
	roundKeys      bool                   // key_style = rounded
	shapeTex       map[shapeSpec]*Texture // rasterized key, pill and popup shapes
	cursorText     Color                  // label on HighlightBg; see readableText
	accentSelText  Color                  // selected accent on AccentHighlightBg
	popupBorder    Color                  // accent popup border; see setTextColors

	// See-through drawing for layer surfaces without a compositor alpha
	// modifier: frames go to layer, which is drawn onto the cleared window
	// with layerOpacity as alpha. 1 = draw straight to the window.
	layerOpacity   float64
	layer          *Texture
	layerW, layerH int32
}

// minTextContrast is the WCAG contrast floor for large text. Key labels
// count as large text.
const minTextContrast = 3.0

// relLuminance is the WCAG relative luminance of c, 0 (black) to 1 (white).
func relLuminance(c Color) float64 {
	ch := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

// contrastRatio is the WCAG contrast ratio of a and b, 1 to 21.
func contrastRatio(a, b Color) float64 {
	la, lb := relLuminance(a), relLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// readableText returns want if it reaches minTextContrast on bg. Otherwise
// it returns black or white, whichever contrasts more with bg.
func readableText(want, bg Color) Color {
	if contrastRatio(want, bg) >= minTextContrast {
		return want
	}
	black, white := c(0, 0, 0), c(255, 255, 255)
	if contrastRatio(black, bg) >= contrastRatio(white, bg) {
		return black
	}
	return white
}

// setTextColors picks the highlight text colors for the current theme. The
// cursor label and the selected accent follow the same rule, white unless it
// is unreadable, so the two match. The popup border uses whichever highlight
// color stands out more from the popup background (the gap color). It runs
// once per theme change, so drawing reads stored colors.
func (r *Renderer) setTextColors() {
	t := r.theme
	white := c(255, 255, 255)
	r.cursorText = readableText(white, t.HighlightBg)
	r.accentSelText = readableText(white, t.AccentHighlightBg)
	r.popupBorder = t.HighlightBg
	if contrastRatio(t.HighlightBorder, t.Bg) > contrastRatio(t.HighlightBg, t.Bg) {
		r.popupBorder = t.HighlightBorder
	}
}

// shadowColor returns the theme's key shadow color, or one derived from Bg
// (50% brightness, fully opaque) when the theme does not set it.
func shadowColor(t Theme) Color {
	if t.KeyShadow != (Color{}) {
		return t.KeyShadow
	}
	dim := func(v uint8) uint8 { return v / 2 }
	return Color{R: dim(t.Bg.R), G: dim(t.Bg.G), B: dim(t.Bg.B), A: 255}
}

// shadowOffset returns how far below a key its shadow reaches, in pixels.
// Kept below the key gap so it never touches the next row; 0 means no shadow.
func shadowOffset(unit, pad int32) int32 {
	if pad < 2 {
		return 0
	}
	return min(max(1, unit/40), pad-1)
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
	Debugf("Label font: %s", fontPath)

	font, err := TTF3OpenFont(fontPath, float32(fontSize))
	if err != nil {
		return nil, err
	}
	fontSmall, err := TTF3OpenFont(fontPath, float32(max32(10, fontSize-4)))
	if err != nil {
		return nil, err
	}

	var fontGlyph *Font
	if p := findPromptFont(); p != "" {
		var err error
		if fontGlyph, err = TTF3OpenFont(p, float32(glyphSize)); err != nil {
			Debugf("Promptfont %s failed to load (%v), showing button names", p, err)
		} else {
			Debugf("Promptfont: %s", p)
		}
	} else {
		Debugf("Promptfont not found, showing button names")
	}
	hasPromptfont := fontGlyph != nil
	if fontGlyph == nil {
		fontGlyph, _ = TTF3OpenFont(fontPath, float32(max32(10, fontSize-6)))
	}

	rend := &Renderer{
		renderer:      r,
		theme:         theme,
		unit:          unitSize,
		pad:           padding,
		statusH:       max32(20, int32(float64(unitSize)*0.4)),
		font:          font,
		fontSmall:     fontSmall,
		fontGlyph:     fontGlyph,
		hasPromptfont: hasPromptfont,
		texCache:      make(map[texCacheKey]texCacheEntry),
		shapeTex:      make(map[shapeSpec]*Texture),
		dirtyFrames:   3, // force initial draw
	}
	rend.setTextColors()
	return rend, nil
}

func (r *Renderer) Draw(kb *KeyboardState) {
	if r.dirtyFrames <= 0 {
		return
	}
	// Layer-shell: don't render until configure is received
	if layerShellMode && !IsLayerShellReady() {
		return
	}
	r.dirtyFrames--
	layered := r.beginLayer()

	t := r.theme
	setColor(r.renderer, t.Bg)
	SDL3RenderClear(r.renderer)

	// Draw modifier status bar
	pillsEnd := r.drawModifierBar(kb)

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
		r.drawAccentPopup(kb, pillsEnd)
	}

	if layered {
		r.endLayer()
	}
	SDL3RenderPresent(r.renderer)
}

// beginLayer redirects drawing into the off-screen layer when the frame is
// drawn see-through. Returns whether the layer is in use.
func (r *Renderer) beginLayer() bool {
	if r.layerOpacity <= 0 || r.layerOpacity >= 1 {
		return false
	}
	w, h := SDL3GetRenderOutputSize(r.renderer)
	if r.layer == nil || r.layerW != w || r.layerH != h {
		SDL3DestroyTexture(r.layer)
		r.layer = SDL3CreateTargetTexture(r.renderer, w, h)
		r.layerW, r.layerH = w, h
	}
	return r.layer != nil && SDL3SetRenderTarget(r.renderer, r.layer)
}

// endLayer clears the window to fully transparent and draws the layer onto
// it once, with the opacity as alpha. The layer itself is opaque (cleared to
// the theme background), so text edges blend against it and do not fringe.
func (r *Renderer) endLayer() {
	SDL3SetRenderTarget(r.renderer, nil)
	setColor(r.renderer, Color{})
	SDL3RenderClear(r.renderer)
	SDL3SetTextureAlphaMod(r.layer, uint8(math.Round(r.layerOpacity*255)))
	SDL3RenderTexture(r.renderer, r.layer, nil, nil)
}

// MarkDirty signals that the display needs updating.
// Redraws for 3 frames to cover double/triple buffering.
func (r *Renderer) MarkDirty() {
	r.dirtyFrames = 3
}

// drawModifierBar draws the status bar and returns the x where the modifier
// pills end.
func (r *Renderer) drawModifierBar(kb *KeyboardState) int32 {
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
	if kb.AltTabHeld {
		labels = append(labels, "ALT-TAB")
	}

	x := r.pad
	tc := r.theme.ModifierActiveText
	for _, label := range labels {
		r.drawPill(label, x, r.pad, r.theme.ModifierActiveBg, tc)
		tw, _ := TTF3GetStringSize(r.fontSmall, label)
		x += tw + 10 + 6
	}

	// Status flash (right-aligned, 2 seconds)
	if (r.flashText != "" || r.flashGlyphText != "") && time.Now().Before(r.flashEnd) {
		sw, _ := SDL3GetRenderOutputSize(r.renderer)
		rightEdge := float32(sw - r.pad)

		// Render text part (right-aligned)
		if r.flashText != "" {
			entry := r.cachedText(r.fontSmall, r.flashText, r.theme.KeyText)
			if entry.tex != nil {
				dst := FRect{X: rightEdge - entry.w, Y: float32(r.pad + 2), W: entry.w, H: entry.h}
				SDL3RenderTexture(r.renderer, entry.tex, nil, &dst)
				rightEdge = dst.X - 4
			}
		}

		// Render glyph part (Promptfont, left of text)
		if r.flashGlyphText != "" {
			entry := r.cachedText(r.fontGlyph, r.drawable(r.fontGlyph, r.flashGlyphText), r.theme.KeyText)
			if entry.tex != nil {
				dst := FRect{X: rightEdge - entry.w, Y: float32(r.pad + 2), W: entry.w, H: entry.h}
				SDL3RenderTexture(r.renderer, entry.tex, nil, &dst)
			}
		}
	}
	return x
}

func (r *Renderer) drawPill(label string, x, y int32, bg Color, fg Color) {
	entry := r.cachedText(r.fontSmall, label, fg)
	if entry.tex == nil {
		return
	}
	pw := int32(entry.w) + 10
	ph := int32(entry.h) + 4
	pill := FRect{X: float32(x), Y: float32(y), W: float32(pw), H: float32(ph)}
	r.fillShape(pill, float32(ph)/2, bg)
	dst := FRect{X: float32(x + 5), Y: float32(y + 2), W: entry.w, H: entry.h}
	SDL3RenderTexture(r.renderer, entry.tex, nil, &dst)
}

// cachedText returns a cached texture entry, rasterizing on first use.
func (r *Renderer) cachedText(font *Font, text string, color Color) texCacheEntry {
	if text == "" || font == nil {
		return texCacheEntry{}
	}
	key := texCacheKey{text: text, font: uintptr(unsafe.Pointer(font)), color: color} //nolint:gosec // G103: pointer used as identity key only, not dereferenced
	if entry, ok := r.texCache[key]; ok {
		return entry
	}
	surf, err := TTF3RenderTextBlended(font, text, color)
	if err != nil {
		return texCacheEntry{}
	}
	tex := SDL3CreateTextureFromSurface(r.renderer, surf)
	w := float32(SDL3SurfaceWidth(surf))
	h := float32(SDL3SurfaceHeight(surf))
	SDL3DestroySurface(surf)
	if tex == nil {
		return texCacheEntry{}
	}
	entry := texCacheEntry{tex: tex, w: w, h: h}
	r.texCache[key] = entry
	return entry
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
	if t.Borderless {
		border = bg
	}

	// Fill, border and shadow (no shadow under the highlighted or flashed key)
	shadow := r.keyShadow && !isCursor && !flashed
	r.fillBorderedShape(rect, keyRadius(h), bg, border, 1, shadow)

	// Label
	label := kb.DisplayLabel(key)
	tc := t.KeyText
	if isCursor || flashed {
		tc = r.cursorText
	} else if key.IsModifier && isModActive(key, kb) {
		tc = t.ModifierActiveText
	}
	// Use Promptfont for labels containing its codepoints (e.g. mouse speed icons)
	labelFont := r.font
	if r.fontGlyph != nil && hasPromptfontRune(label) {
		labelFont = r.fontGlyph
	}
	r.renderText(labelFont, r.drawable(labelFont, label), tc, rect, AlignCenter)

	// Shift hint (top-right)
	if key.ShiftLabel != "" && !key.IsModifier && !isCursor {
		if kb.ShiftActive == kb.CapsActive {
			hintFont := r.fontSmall
			if r.fontGlyph != nil && hasPromptfontRune(key.ShiftLabel) {
				hintFont = r.fontGlyph
			}
			r.renderText(hintFont, r.drawable(hintFont, key.ShiftLabel), t.ModifierText,
				FRect{X: float32(x), Y: float32(y), W: float32(w - 4), H: float32(h)}, AlignTopRight)
		}
	}

	// Controller glyph (bottom-right)
	if g, ok := KeyGlyphs[key.Code]; ok && r.fontGlyph != nil {
		// Without Promptfont, fontGlyph is the label font: show the button name
		glyph := g.Glyph
		if !r.hasPromptfont {
			glyph = r.drawable(r.fontGlyph, g.Text)
		}
		r.renderText(r.fontGlyph, glyph, t.GlyphColor,
			FRect{X: float32(x), Y: float32(y), W: float32(w - 3), H: float32(h - 2)}, AlignBottomRight)
	}
}

// popupDim is the alpha of the Bg layer drawn over the keyboard while the
// accent popup is open.
const popupDim = 140

// drawAccentPopup draws the popup, a pointer down to the key it belongs to,
// and its control hint in the status bar, right of minX.
func (r *Renderer) drawAccentPopup(kb *KeyboardState, minX int32) {
	accents := kb.AccentPopup.Accents
	sel := kb.AccentPopup.Selected
	row := kb.Layout[kb.CursorRow]

	xo := r.pad
	for i := range kb.CursorCol {
		xo += int32(row[i].Width*float64(r.unit)) + r.pad
	}
	ky := r.pad + r.statusH + int32(kb.CursorRow)*(r.unit+r.pad) //nolint:gosec // G115: cursor index fits in int32
	// Raise the popup to make room for the pointer
	tailH := max(6, r.unit/6)
	py := ky - r.unit - r.pad - tailH

	tw := int32(len(accents))*(r.unit+r.pad) + r.pad //nolint:gosec // G115: accent count fits in int32
	pr := FRect{X: float32(xo), Y: float32(py), W: float32(tw), H: float32(r.unit + r.pad*2)}
	keyR := keyRadius(r.unit)
	popupR := keyR + float32(r.pad)

	// Dim the keyboard so the popup reads as a menu over it, then draw the
	// popup with a thick border in a highlight color
	ow, oh := SDL3GetRenderOutputSize(r.renderer)
	dim := r.theme.Bg
	dim.A = popupDim
	setColor(r.renderer, dim)
	SDL3RenderFillRect(r.renderer, FRect{W: float32(ow), H: float32(oh)})
	bw := max(1, min(max(2, r.unit/24), r.pad))
	// Draw the cells like keys on the keyboard: key color with a key border,
	// separated by the gap color. AccentPopupBg is the key color on almost
	// every theme, so cells on it would not show.
	r.fillBorderedShape(pr, popupR, r.theme.Bg, r.popupBorder, bw, r.keyShadow)

	// Pointer: a triangle from the bottom border down to the cursor key
	cx := float32(xo) + float32(row[kb.CursorCol].Width*float64(r.unit))/2
	top := pr.Y + pr.H - float32(bw)
	setColor(r.renderer, r.popupBorder)
	for j := range tailH + bw {
		half := float32(tailH + bw - j)
		SDL3RenderFillRect(r.renderer, FRect{X: cx - half, Y: top + float32(j), W: 2 * half, H: 1})
	}
	r.drawHint(PopupHint, minX)

	ax := xo + r.pad
	for i, accent := range accents {
		ar := FRect{X: float32(ax), Y: float32(py + r.pad), W: float32(r.unit), H: float32(r.unit)}
		if i == sel {
			r.fillShape(ar, keyR, r.theme.AccentHighlightBg)
		} else {
			r.fillBorderedShape(ar, keyR, r.theme.KeyBg, r.theme.KeyBorder, 1, false)
		}
		tc := r.theme.AccentPopupText
		if i == sel {
			tc = r.accentSelText
		}
		r.renderText(r.font, accent.Label, tc, ar, AlignCenter)
		ax += r.unit + r.pad
	}
}

// drawHint draws control hints centred in the status bar, kept right of minX
// so they never cover the modifier pills.
func (r *Renderer) drawHint(items []HintItem, minX int32) {
	type part struct {
		e   texCacheEntry
		gap float32 // space before this part
	}
	tc := r.theme.KeyText
	space := float32(r.unit) / 10
	var parts []part
	for i, it := range items {
		gap := float32(0)
		if i > 0 {
			gap = space * 3
		}
		g := r.cachedText(r.fontSmall, r.drawable(r.fontSmall, it.Text), tc)
		if r.hasPromptfont {
			g = r.cachedText(r.fontGlyph, it.Glyph, tc)
		}
		parts = append(parts, part{g, gap}, part{r.cachedText(r.fontSmall, it.Label, tc), space})
	}
	var w float32
	for _, p := range parts {
		w += p.gap + p.e.w
	}
	sw, _ := SDL3GetRenderOutputSize(r.renderer)
	x := max(float32(sw)/2-w/2, float32(minX+r.pad))
	mid := float32(r.pad) + float32(r.statusH)/2
	for _, p := range parts {
		x += p.gap
		if p.e.tex != nil {
			dst := FRect{X: x, Y: mid - p.e.h/2, W: p.e.w, H: p.e.h}
			SDL3RenderTexture(r.renderer, p.e.tex, nil, &dst)
		}
		x += p.e.w
	}
}

type TextAlign int

const (
	AlignCenter TextAlign = iota
	AlignTopRight
	AlignBottomRight
)

func (r *Renderer) renderText(font *Font, text string, color Color, rect FRect, align TextAlign) {
	entry := r.cachedText(font, text, color)
	if entry.tex == nil {
		return
	}

	var dst FRect
	switch align {
	case AlignCenter:
		dst = FRect{
			X: rect.X + (rect.W-entry.w)/2,
			Y: rect.Y + (rect.H-entry.h)/2,
			W: entry.w, H: entry.h,
		}
	case AlignTopRight:
		dst = FRect{
			X: rect.X + rect.W - entry.w,
			Y: rect.Y + 2,
			W: entry.w, H: entry.h,
		}
	case AlignBottomRight:
		dst = FRect{
			X: rect.X + rect.W - entry.w,
			Y: rect.Y + rect.H - entry.h,
			W: entry.w, H: entry.h,
		}
	}
	SDL3RenderTexture(r.renderer, entry.tex, nil, &dst)
}

func (r *Renderer) SetTheme(t Theme) {
	r.theme = t
	r.setTextColors()
	r.flushTexCache()
	r.Flash(t.Name)
}

func (r *Renderer) flushTexCache() {
	for k, entry := range r.texCache {
		if entry.tex != nil {
			SDL3DestroyTexture(entry.tex)
		}
		delete(r.texCache, k)
	}
	r.flushShapeCache()
}

func (r *Renderer) Flash(text string) {
	r.flashText = text
	r.flashGlyphText = ""
	r.flashEnd = time.Now().Add(2 * time.Second)
	r.MarkDirty()
}

// FlashGlyph renders a Promptfont glyph followed by text in the normal font.
func (r *Renderer) FlashGlyph(glyph string, text string) {
	r.flashGlyphText = glyph
	r.flashText = text
	r.flashEnd = time.Now().Add(2 * time.Second)
	r.MarkDirty()
}

func (r *Renderer) Destroy() {
	r.flushTexCache()
	SDL3DestroyTexture(r.layer)
	r.layer = nil
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

// asciiFallback replaces symbols that the label font may lack. Without
// Promptfont, fontGlyph is the label font, and fonts such as Liberation Sans
// or Noto Sans have none of these. "" means draw nothing.
var asciiFallback = map[string]string{
	"\u25c0\u25b6": "< >", // popup hint: d-pad left and right
	"\u27F9":       "M+",  // mouse speed up (Shift+Up)
	"\u27FE":       "M-",  // mouse speed down (Shift+Down)
	"\u2699":       "",    // Theme key gear; the key says Theme anyway
}

// drawable returns s if font has every character in it, else its ASCII
// fallback, so a missing glyph never shows as an empty box.
func (r *Renderer) drawable(font *Font, s string) string {
	if font == nil {
		return s
	}
	return fallbackText(s, func(c rune) bool { return TTF3FontHasGlyph(font, c) })
}

func fallbackText(s string, has func(rune) bool) string {
	for _, c := range s {
		if c != ' ' && !has(c) {
			if a, ok := asciiFallback[s]; ok {
				return a
			}
			return s
		}
	}
	return s
}

// hasPromptfontRune returns true if the string contains codepoints from
// Promptfont's mapped range (U+2600-U+27FF).
func hasPromptfontRune(s string) bool {
	for _, r := range s {
		if r >= 0x2600 && r <= 0x27FF {
			return true
		}
	}
	return false
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}
