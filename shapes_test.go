package main

import (
	"math"
	"testing"
)

// rasterize paints spans into a pixel grid of coverage counts and alpha sums.
// Fails the test if a span is not on whole pixels or a pixel is painted twice.
func rasterize(t *testing.T, spans []span) (alpha map[[2]int]float64) {
	t.Helper()
	alpha = make(map[[2]int]float64)
	for _, s := range spans {
		r := s.Rect
		for _, v := range []float32{r.X, r.Y, r.W, r.H} {
			if v != float32(math.Round(float64(v))) {
				t.Fatalf("span %+v not on whole pixels", r)
			}
		}
		for y := int(r.Y); y < int(r.Y+r.H); y++ {
			for x := int(r.X); x < int(r.X+r.W); x++ {
				p := [2]int{x, y}
				if _, ok := alpha[p]; ok {
					t.Fatalf("pixel %v painted twice", p)
				}
				alpha[p] = s.Alpha
			}
		}
	}
	return alpha
}

func TestRoundRectSmallRadiusIsPlainRect(t *testing.T) {
	rect := FRect{X: 10.5, Y: 20, W: 50, H: 30}
	for _, radius := range []float32{0, 0.4, 0.99} {
		spans := roundRectSpans(rect, radius)
		if len(spans) != 1 || spans[0].Rect != rect || spans[0].Alpha != 1 {
			t.Errorf("radius %v: got %+v, want the plain rect", radius, spans)
		}
	}
}

func TestRoundRectRadiusClamped(t *testing.T) {
	// Radius larger than half the height: clamped to a full pill end
	alpha := rasterize(t, roundRectSpans(FRect{X: 0, Y: 0, W: 60, H: 20}, 100))
	for p := range alpha {
		if p[0] < 0 || p[0] >= 60 || p[1] < 0 || p[1] >= 20 {
			t.Fatalf("pixel %v outside the rect", p)
		}
	}
	// Middle row reaches both ends; corner pixel is empty
	for x := range 60 {
		if _, ok := alpha[[2]int{x, 10}]; !ok {
			t.Errorf("middle row pixel %d not painted", x)
		}
	}
	if _, ok := alpha[[2]int{0, 0}]; ok {
		t.Error("corner pixel painted on a fully round end")
	}
}

func TestRoundRectOddHeightPill(t *testing.T) {
	// H=21 with radius H/2: rounding must not make the middle band negative
	for _, s := range roundRectSpans(FRect{X: 0, Y: 0, W: 80, H: 21}, 10.5) {
		if s.Rect.W <= 0 || s.Rect.H <= 0 {
			t.Fatalf("empty or negative span %+v", s.Rect)
		}
	}
	alpha := rasterize(t, roundRectSpans(FRect{X: 0, Y: 0, W: 80, H: 21}, 10.5))
	if alpha[[2]int{40, 10}] != 1 {
		t.Error("centre pixel not solid")
	}
}

func TestRoundRectCoverage(t *testing.T) {
	cases := []struct {
		rect   FRect
		radius float32
	}{
		{FRect{X: 0, Y: 0, W: 80, H: 80}, 12.8},
		{FRect{X: 10.4, Y: 20.6, W: 50.5, H: 30.2}, 5},
		{FRect{X: 3.5, Y: 7.49, W: 140, H: 61}, 9.76},
		{FRect{X: 0, Y: 0, W: 24, H: 24}, 3.84},
		{FRect{X: 0, Y: 0, W: 7, H: 5}, 2.5},
	}
	for _, tc := range cases {
		spans := roundRectSpans(tc.rect, tc.radius)
		alpha := rasterize(t, spans)

		x0 := int(math.Round(float64(tc.rect.X)))
		y0 := int(math.Round(float64(tc.rect.Y)))
		w := int(math.Round(float64(tc.rect.W)))
		h := int(math.Round(float64(tc.rect.H)))
		rad := min(int(math.Round(float64(tc.radius))), w/2, h/2)

		// Nothing outside the rounded bounds
		for p := range alpha {
			if p[0] < x0 || p[0] >= x0+w || p[1] < y0 || p[1] >= y0+h {
				t.Errorf("%+v: pixel %v outside rect", tc.rect, p)
			}
		}
		// Every row is one run with no gaps; the solid cross is fully covered
		for y := y0; y < y0+h; y++ {
			first, last, n := -1, -1, 0
			for x := x0; x < x0+w; x++ {
				if _, ok := alpha[[2]int{x, y}]; ok {
					if first < 0 {
						first = x
					}
					last = x
					n++
				}
				inCross := (x >= x0+rad && x < x0+w-rad) || (y >= y0+rad && y < y0+h-rad)
				if inCross && alpha[[2]int{x, y}] != 1 {
					t.Errorf("%+v: pixel (%d,%d) in the solid cross not solid", tc.rect, x, y)
				}
			}
			if n > 0 && last-first+1 != n {
				t.Errorf("%+v: row %d has a gap", tc.rect, y)
			}
		}
		// Left-right and top-bottom symmetric
		for p, a := range alpha {
			mx := [2]int{2*x0 + w - 1 - p[0], p[1]}
			my := [2]int{p[0], 2*y0 + h - 1 - p[1]}
			if alpha[mx] != a || alpha[my] != a {
				t.Errorf("%+v: pixel %v not symmetric", tc.rect, p)
				break
			}
		}
		// Painted area close to the true rounded-rect area
		var area float64
		for _, a := range alpha {
			area += a
		}
		want := float64(w*h) - (4-math.Pi)*float64(rad*rad)
		if math.Abs(area-want) > 0.05*want+2 {
			t.Errorf("%+v: area %.1f, want about %.1f", tc.rect, area, want)
		}
	}
}
