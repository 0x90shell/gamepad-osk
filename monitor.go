package main

import "github.com/veandco/go-sdl2/sdl"

type MonitorRect struct {
	X, Y, W, H int32
}

// GetPrimaryMonitor returns the bounds of display 0 (usually primary).
// Falls back to the largest display if display 0 fails.
func GetPrimaryMonitor() MonitorRect {
	n, err := sdl.GetNumVideoDisplays()
	if err != nil || n == 0 {
		return MonitorRect{0, 0, 1920, 1080}
	}

	// Display 0 is typically the primary
	rect, err := sdl.GetDisplayBounds(0)
	if err == nil {
		return MonitorRect{rect.X, rect.Y, rect.W, rect.H}
	}

	// Fallback: largest display
	var best sdl.Rect
	for i := 0; i < n; i++ {
		r, err := sdl.GetDisplayBounds(i)
		if err == nil && r.W*r.H > best.W*best.H {
			best = r
		}
	}
	if best.W > 0 {
		return MonitorRect{best.X, best.Y, best.W, best.H}
	}
	return MonitorRect{0, 0, 1920, 1080}
}
