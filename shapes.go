package main

import "math"

// span is one filled piece of a shape: a rectangle drawn at Alpha times the
// color's alpha. Solid pieces have Alpha 1, anti-aliased edge pixels less.
type span struct {
	Rect  FRect
	Alpha float64
}

// roundRectSpans splits a rounded rectangle into filled pieces: a middle band,
// then one pixel row at a time for the corners, each with an anti-aliased
// edge pixel. Coordinates are rounded to whole pixels here, once, so the
// pieces never overlap or leave gaps. The radius is clamped to half the width
// and height; under 1 px the plain rectangle is returned.
func roundRectSpans(rect FRect, radius float32) []span {
	if radius < 1 {
		return []span{{Rect: rect, Alpha: 1}}
	}
	x0 := math.Round(float64(rect.X))
	y0 := math.Round(float64(rect.Y))
	w := math.Round(float64(rect.W))
	h := math.Round(float64(rect.H))
	rad := min(math.Round(float64(radius)), math.Floor(w/2), math.Floor(h/2))
	if rad < 1 {
		return []span{{Rect: rect, Alpha: 1}}
	}

	fr := func(x, y, w, h float64) FRect {
		return FRect{X: float32(x), Y: float32(y), W: float32(w), H: float32(h)}
	}
	var spans []span
	if band := h - 2*rad; band > 0 {
		spans = append(spans, span{Rect: fr(x0, y0+rad, w, band), Alpha: 1})
	}
	for i := range int(rad) {
		// Distance of this pixel row's centre from the corner circle centre
		dy := rad - (float64(i) + 0.5)
		inset := rad - math.Sqrt(max(0, rad*rad-dy*dy))
		full := math.Ceil(inset)
		frac := full - inset // coverage of the partial edge pixel
		for _, y := range []float64{y0 + float64(i), y0 + h - 1 - float64(i)} {
			if solid := w - 2*full; solid > 0 {
				spans = append(spans, span{Rect: fr(x0+full, y, solid, 1), Alpha: 1})
			}
			if frac > 0.02 && full >= 1 {
				spans = append(spans,
					span{Rect: fr(x0+full-1, y, 1, 1), Alpha: frac},
					span{Rect: fr(x0+w-full, y, 1, 1), Alpha: frac})
			}
		}
	}
	return spans
}
