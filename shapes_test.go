package main

import (
	"math"
	"testing"
)

// paintRoundRect paints the pieces into a pixel grid of alpha values (0-1),
// using circleMask for the corners. Fails the test if a piece is not on whole
// pixels or a pixel is painted twice.
func paintRoundRect(t *testing.T, p roundRect) map[[2]int]float64 {
	t.Helper()
	alpha := make(map[[2]int]float64)
	paint := func(r FRect, a func(x, y int) float64) {
		for _, v := range []float32{r.X, r.Y, r.W, r.H} {
			if v != float32(math.Round(float64(v))) {
				t.Fatalf("piece %+v not on whole pixels", r)
			}
		}
		for y := int(r.Y); y < int(r.Y+r.H); y++ {
			for x := int(r.X); x < int(r.X+r.W); x++ {
				px := [2]int{x, y}
				if _, ok := alpha[px]; ok {
					t.Fatalf("pixel %v painted twice", px)
				}
				alpha[px] = a(x-int(r.X), y-int(r.Y))
			}
		}
	}
	for _, r := range p.Rects {
		paint(r, func(int, int) float64 { return 1 })
	}
	if p.Rad == 0 {
		return alpha
	}
	rad := int(p.Rad)
	mask := circleMask(p.Rad)
	for i, c := range p.Corners {
		ox, oy := (i%2)*rad, (i/2)*rad // quadrant offset in the mask
		paint(c, func(x, y int) float64 {
			return float64(mask[(oy+y)*2*rad+ox+x]) / 255
		})
	}
	return alpha
}

func TestRoundRectSmallRadiusIsPlainRect(t *testing.T) {
	rect := FRect{X: 10.5, Y: 20, W: 50, H: 30}
	for _, radius := range []float32{0, 0.4, 0.99} {
		p := roundRectPieces(rect, radius)
		if p.Rad != 0 || len(p.Rects) != 1 || p.Rects[0] != rect {
			t.Errorf("radius %v: got %+v, want the plain rect", radius, p)
		}
	}
}

func TestRoundRectRadiusClamped(t *testing.T) {
	// Radius larger than half the height: clamped to a full pill end
	p := roundRectPieces(FRect{X: 0, Y: 0, W: 60, H: 20}, 100)
	if p.Rad != 10 {
		t.Fatalf("rad = %d, want 10", p.Rad)
	}
	alpha := paintRoundRect(t, p)
	if len(alpha) != 60*20 {
		t.Errorf("painted %d pixels, want the full 60x20 box", len(alpha))
	}
	if alpha[[2]int{0, 0}] != 0 {
		t.Error("corner pixel painted on a fully round end")
	}
	if alpha[[2]int{30, 10}] != 1 {
		t.Error("centre pixel not solid")
	}
}

func TestRoundRectOddHeightPill(t *testing.T) {
	// H=21 with radius H/2: rounding must not make the middle band negative
	p := roundRectPieces(FRect{X: 0, Y: 0, W: 80, H: 21}, 10.5)
	if p.Rad != 10 {
		t.Fatalf("rad = %d, want 10", p.Rad)
	}
	for _, r := range p.Rects {
		if r.W <= 0 || r.H <= 0 {
			t.Fatalf("empty or negative piece %+v", r)
		}
	}
	if len(paintRoundRect(t, p)) != 80*21 {
		t.Error("pieces do not cover the box")
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
		{FRect{X: 0, Y: 0, W: 20, H: 20}, 10}, // circle: no plain middle pieces
	}
	for _, tc := range cases {
		p := roundRectPieces(tc.rect, tc.radius)
		alpha := paintRoundRect(t, p)

		x0 := int(math.Round(float64(tc.rect.X)))
		y0 := int(math.Round(float64(tc.rect.Y)))
		w := int(math.Round(float64(tc.rect.W)))
		h := int(math.Round(float64(tc.rect.H)))
		rad := min(int(math.Round(float64(tc.radius))), w/2, h/2)
		if int(p.Rad) != rad {
			t.Errorf("%+v: rad = %d, want %d", tc.rect, p.Rad, rad)
		}

		// Pieces tile the bounding box exactly
		if len(alpha) != w*h {
			t.Errorf("%+v: painted %d pixels, want %d", tc.rect, len(alpha), w*h)
		}
		for px := range alpha {
			if px[0] < x0 || px[0] >= x0+w || px[1] < y0 || px[1] >= y0+h {
				t.Errorf("%+v: pixel %v outside rect", tc.rect, px)
			}
		}
		// The solid cross is fully opaque
		for y := y0; y < y0+h; y++ {
			for x := x0; x < x0+w; x++ {
				inCross := (x >= x0+rad && x < x0+w-rad) || (y >= y0+rad && y < y0+h-rad)
				if inCross && alpha[[2]int{x, y}] != 1 {
					t.Errorf("%+v: pixel (%d,%d) in the solid cross not solid", tc.rect, x, y)
				}
			}
		}
		// Left-right and top-bottom symmetric
		for px, a := range alpha {
			mx := [2]int{2*x0 + w - 1 - px[0], px[1]}
			my := [2]int{px[0], 2*y0 + h - 1 - px[1]}
			if alpha[mx] != a || alpha[my] != a {
				t.Errorf("%+v: pixel %v not symmetric", tc.rect, px)
				break
			}
		}
		// Painted area close to the true rounded-rect area
		var area float64
		for _, a := range alpha {
			area += a
		}
		want := float64(w*h) - (4-math.Pi)*float64(rad*rad)
		if math.Abs(area-want) > 0.02*want+2 {
			t.Errorf("%+v: area %.1f, want about %.1f", tc.rect, area, want)
		}
	}
}

func TestCircleMask(t *testing.T) {
	for _, rad := range []int32{1, 2, 5, 13, 40} {
		mask := circleMask(rad)
		size := int(2 * rad)
		if len(mask) != size*size {
			t.Fatalf("rad %d: mask length %d, want %d", rad, len(mask), size*size)
		}
		// Tiny circles cover their corner and centre pixels only in part
		c := int(rad)
		if rad >= 5 && mask[0] != 0 {
			t.Errorf("rad %d: outer corner alpha %d, want 0", rad, mask[0])
		}
		if rad >= 5 && mask[c*size+c] != 255 {
			t.Errorf("rad %d: centre alpha %d, want 255", rad, mask[c*size+c])
		}
		var sum float64
		for y := range size {
			for x := range size {
				a := mask[y*size+x]
				if a != mask[y*size+size-1-x] || a != mask[(size-1-y)*size+x] {
					t.Fatalf("rad %d: mask not symmetric at (%d,%d)", rad, x, y)
				}
				sum += float64(a) / 255
			}
		}
		want := math.Pi * float64(rad*rad)
		if math.Abs(sum-want) > 0.05*want+1 {
			t.Errorf("rad %d: mask area %.1f, want about %.1f", rad, sum, want)
		}
	}
}
