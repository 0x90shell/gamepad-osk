package main

import "math"

// keyRadiusFrac is the corner radius of rounded keys as a fraction of key height.
const keyRadiusFrac = 0.16

// roundRect is a rounded rectangle split into pieces on whole pixels: up to
// three plain rectangles (Rects) and four Rad x Rad corner squares (Corners:
// top-left, top-right, bottom-left, bottom-right), each filled with one
// quadrant of a circle mask. Rad 0 means a plain rectangle, no corners.
type roundRect struct {
	Rects   []FRect
	Corners [4]FRect
	Rad     int32
}

// roundRectPieces splits a rounded rectangle into pieces. Coordinates are
// rounded to whole pixels here, once, so the pieces never overlap or leave
// gaps. The radius is clamped to half the width and height; under 1 px the
// plain rectangle is returned.
func roundRectPieces(rect FRect, radius float32) roundRect {
	if radius < 1 {
		return roundRect{Rects: []FRect{rect}}
	}
	x0 := float32(math.Round(float64(rect.X)))
	y0 := float32(math.Round(float64(rect.Y)))
	w := float32(math.Round(float64(rect.W)))
	h := float32(math.Round(float64(rect.H)))
	rad := float32(min(math.Round(float64(radius)), math.Floor(float64(w)/2), math.Floor(float64(h)/2)))
	if rad < 1 {
		return roundRect{Rects: []FRect{rect}}
	}

	var p roundRect
	if band := h - 2*rad; band > 0 {
		p.Rects = append(p.Rects, FRect{X: x0, Y: y0 + rad, W: w, H: band})
	}
	if mid := w - 2*rad; mid > 0 {
		p.Rects = append(p.Rects,
			FRect{X: x0 + rad, Y: y0, W: mid, H: rad},
			FRect{X: x0 + rad, Y: y0 + h - rad, W: mid, H: rad})
	}
	p.Corners = [4]FRect{
		{X: x0, Y: y0, W: rad, H: rad},
		{X: x0 + w - rad, Y: y0, W: rad, H: rad},
		{X: x0, Y: y0 + h - rad, W: rad, H: rad},
		{X: x0 + w - rad, Y: y0 + h - rad, W: rad, H: rad},
	}
	p.Rad = int32(rad)
	return p
}

// circleMask returns the alpha coverage (0-255) of a circle of radius rad in a
// 2rad x 2rad grid, row by row, from 4x4 samples per pixel. Each quadrant is
// the corner shape for one corner of a rounded rectangle.
func circleMask(rad int32) []uint8 {
	const n = 4 // samples per pixel side
	size := int(2 * rad)
	r := float64(rad)
	mask := make([]uint8, size*size)
	for y := range size {
		for x := range size {
			in := 0
			for sy := range n {
				for sx := range n {
					dx := float64(x) + (float64(sx)+0.5)/n - r
					dy := float64(y) + (float64(sy)+0.5)/n - r
					if dx*dx+dy*dy <= r*r {
						in++
					}
				}
			}
			mask[y*size+x] = uint8(in * 255 / (n * n)) //nolint:gosec // G115: at most 255
		}
	}
	return mask
}

// fillRoundRect fills a rounded rectangle: plain fills for the straight
// parts, and a cached circle texture tinted to col for the four corners.
func (r *Renderer) fillRoundRect(rect FRect, radius float32, col Color) {
	p := roundRectPieces(rect, radius)
	setColor(r.renderer, col)
	for _, rc := range p.Rects {
		SDL3RenderFillRect(r.renderer, rc)
	}
	if p.Rad == 0 {
		return
	}
	tex := r.cornerTexture(p.Rad)
	if tex == nil {
		return
	}
	SDL3SetTextureColorAlphaMod(tex, col)
	rad := float32(p.Rad)
	for i, dst := range p.Corners {
		src := FRect{X: float32(i%2) * rad, Y: float32(i/2) * rad, W: rad, H: rad}
		SDL3RenderTexture(r.renderer, tex, &src, &dst)
	}
}

// cornerTexture returns the white circle texture for radius rad, creating it
// on first use. Its four quadrants are the four corner shapes.
func (r *Renderer) cornerTexture(rad int32) *Texture {
	if tex, ok := r.cornerTex[rad]; ok {
		return tex
	}
	mask := circleMask(rad)
	pixels := make([]byte, 0, len(mask)*4)
	for _, a := range mask {
		pixels = append(pixels, 255, 255, 255, a)
	}
	size := 2 * rad
	tex := SDL3CreateTextureRGBA(r.renderer, size, size, pixels)
	r.cornerTex[rad] = tex // nil is cached too, so a failure is not retried every frame
	return tex
}

// fillShape fills rect, with rounded corners when key_style is rounded.
func (r *Renderer) fillShape(rect FRect, radius float32, col Color) {
	if r.roundKeys {
		r.fillRoundRect(rect, radius, col)
		return
	}
	setColor(r.renderer, col)
	SDL3RenderFillRect(r.renderer, rect)
}

// fillBorderedShape fills rect in border color, then the inside, 1 px in, in
// bg. Square shapes keep the 1 px outline drawn on top of the fill.
func (r *Renderer) fillBorderedShape(rect FRect, radius float32, bg, border Color) {
	if r.roundKeys {
		r.fillRoundRect(rect, radius, border)
		r.fillRoundRect(FRect{X: rect.X + 1, Y: rect.Y + 1, W: rect.W - 2, H: rect.H - 2}, radius-1, bg)
		return
	}
	setColor(r.renderer, bg)
	SDL3RenderFillRect(r.renderer, rect)
	setColor(r.renderer, border)
	SDL3RenderRect(r.renderer, rect)
}

// keyRadius is the corner radius of a key of height h.
func keyRadius(h int32) float32 {
	return keyRadiusFrac * float32(h)
}
