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

// shapeMask returns the coverage (0-255) of a w x h rounded rectangle, row
// by row: 255 for the straight parts, circleMask quadrants for the corners.
func shapeMask(w, h int32, radius float32) []uint8 {
	mask := make([]uint8, w*h)
	p := roundRectPieces(FRect{W: float32(w), H: float32(h)}, radius)
	for _, rc := range p.Rects {
		for y := int32(rc.Y); y < int32(rc.Y+rc.H); y++ {
			for x := int32(rc.X); x < int32(rc.X+rc.W); x++ {
				mask[y*w+x] = 255
			}
		}
	}
	if p.Rad == 0 {
		return mask
	}
	circle := circleMask(p.Rad)
	for i, c := range p.Corners {
		ox, oy := int32(i%2)*p.Rad, int32(i/2)*p.Rad // quadrant offset in the circle
		for y := range p.Rad {
			for x := range p.Rad {
				mask[(int32(c.Y)+y)*w+int32(c.X)+x] = circle[(oy+y)*2*p.Rad+ox+x]
			}
		}
	}
	return mask
}

// shapeSpec describes one shape (key, pill, popup) drawn from a cached
// texture. It is the cache key, so equal shapes share one texture. Every
// input that changes how a shape looks must be a field here, or the cache
// keeps serving the old image. Sizes are window pixels; HiDPI drawing would
// need them scaled to device pixels.
type shapeSpec struct {
	W, H      int32
	Radius    float32
	Bg        Color
	Border    Color // drawn as the outer shape, Bg 1 px inside it
	Bordered  bool
	Shadow    Color // drawn ShadowOff px below the shape
	ShadowOff int32 // 0 = no shadow
}

// rasterShape draws the shape into RGBA bytes (straight alpha), W wide and
// H+ShadowOff high: shadow first, then border, then fill, each blended over
// the layers below.
func rasterShape(s shapeSpec) []byte {
	ih := s.H + s.ShadowOff
	px := make([]byte, s.W*ih*4)
	layer := func(mask []uint8, x0, y0, mw int32, col Color) {
		for i, m := range mask {
			if m == 0 {
				continue
			}
			x, y := x0+int32(i)%mw, y0+int32(i)/mw //nolint:gosec // G115: mask index fits in int32
			blendOver(px[(y*s.W+x)*4:], col, float64(m)/255*float64(col.A)/255)
		}
	}
	if s.ShadowOff > 0 {
		layer(shapeMask(s.W, s.H, s.Radius), 0, s.ShadowOff, s.W, s.Shadow)
	}
	if s.Bordered {
		layer(shapeMask(s.W, s.H, s.Radius), 0, 0, s.W, s.Border)
		if s.W > 2 && s.H > 2 {
			layer(shapeMask(s.W-2, s.H-2, s.Radius-1), 1, 1, s.W-2, s.Bg)
		}
	} else {
		layer(shapeMask(s.W, s.H, s.Radius), 0, 0, s.W, s.Bg)
	}
	return px
}

// blendOver blends col at coverage a (0-1) over the RGBA pixel dst, in place.
func blendOver(dst []byte, col Color, a float64) {
	da := float64(dst[3]) / 255
	oa := a + da*(1-a)
	if oa == 0 {
		return
	}
	mix := func(sc uint8, dc byte) byte {
		return byte((float64(sc)*a+float64(dc)*da*(1-a))/oa + 0.5)
	}
	dst[0] = mix(col.R, dst[0])
	dst[1] = mix(col.G, dst[1])
	dst[2] = mix(col.B, dst[2])
	dst[3] = byte(oa*255 + 0.5)
}

// maxShapeTex bounds the shape cache. A layout and theme need about 13. On
// amdgpu, VRAM grows far faster than the pixel data past a few dozen
// textures (about 100 MB at 500), so the cache is cleared and rebuilt
// instead of growing past this.
const maxShapeTex = 64

// drawShape draws a shape at (x, y) from the shape cache, rasterizing it on
// first use: one draw call per shape. If the texture cannot be created (GPU
// out of memory or lost), it falls back to plain SDL fills.
func (r *Renderer) drawShape(x, y float32, s shapeSpec) {
	tex, ok := r.shapeTex[s]
	if !ok {
		if len(r.shapeTex) >= maxShapeTex {
			r.flushShapeCache()
		}
		tex = SDL3CreateTextureRGBA(r.renderer, s.W, s.H+s.ShadowOff, rasterShape(s))
		r.shapeTex[s] = tex // nil is cached too, so a failure is not retried every frame
	}
	if tex == nil {
		r.drawShapeFallback(x, y, s)
		return
	}
	dst := FRect{X: x, Y: y, W: float32(s.W), H: float32(s.H + s.ShadowOff)}
	SDL3RenderTexture(r.renderer, tex, nil, &dst)
}

// drawShapeFallback draws s square with SDL fills: shadow strip, fill, then
// a 1 px outline. Rounded corners are lost.
func (r *Renderer) drawShapeFallback(x, y float32, s shapeSpec) {
	rect := FRect{X: x, Y: y, W: float32(s.W), H: float32(s.H)}
	if s.ShadowOff > 0 {
		setColor(r.renderer, s.Shadow)
		SDL3RenderFillRect(r.renderer, FRect{X: x, Y: y + float32(s.ShadowOff), W: rect.W, H: rect.H})
	}
	setColor(r.renderer, s.Bg)
	SDL3RenderFillRect(r.renderer, rect)
	if s.Bordered {
		setColor(r.renderer, s.Border)
		SDL3RenderRect(r.renderer, rect)
	}
}

func (r *Renderer) flushShapeCache() {
	for k, tex := range r.shapeTex {
		SDL3DestroyTexture(tex)
		delete(r.shapeTex, k)
	}
}

// shapeFor returns the cache key for a shape of rect's size. Square keys
// use radius 0.
func (r *Renderer) shapeFor(rect FRect, radius float32, bg Color, shadow bool) shapeSpec {
	if !r.roundKeys {
		radius = 0
	}
	s := shapeSpec{W: int32(rect.W), H: int32(rect.H), Radius: radius, Bg: bg}
	if shadow {
		s.ShadowOff = shadowOffset(r.unit, r.pad)
		s.Shadow = shadowColor(r.theme)
	}
	return s
}

// fillShape fills rect, with rounded corners when key_style is rounded.
func (r *Renderer) fillShape(rect FRect, radius float32, col Color) {
	r.drawShape(rect.X, rect.Y, r.shapeFor(rect, radius, col, false))
}

// fillBorderedShape draws a key-like shape: optional shadow, then bg with a
// 1 px border. The border is the outer shape and bg the same shape 1 px
// inside it, so on square shapes it matches a 1 px outline.
func (r *Renderer) fillBorderedShape(rect FRect, radius float32, bg, border Color, shadow bool) {
	s := r.shapeFor(rect, radius, bg, shadow)
	s.Border, s.Bordered = border, true
	r.drawShape(rect.X, rect.Y, s)
}

// keyRadius is the corner radius of a key of height h.
func keyRadius(h int32) float32 {
	return keyRadiusFrac * float32(h)
}
