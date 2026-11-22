package main

type MonitorRect struct {
	X, Y, W, H int32
}

// intersectRect returns the overlap of two rectangles.
// If they don't overlap, returns a (falls back to monitor bounds).
func intersectRect(a, b MonitorRect) MonitorRect {
	x1 := max32(a.X, b.X)
	y1 := max32(a.Y, b.Y)
	x2 := min32(a.X+a.W, b.X+b.W)
	y2 := min32(a.Y+a.H, b.Y+b.H)
	if x2 > x1 && y2 > y1 {
		return MonitorRect{X: x1, Y: y1, W: x2 - x1, H: y2 - y1}
	}
	return a
}

func min32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

// The SDL display calls, variables so tests can fake the monitors.
var (
	sdlDisplays       = SDL3GetDisplays
	sdlPrimaryDisplay = SDL3GetPrimaryDisplay
	sdlDisplayBounds  = SDL3GetDisplayBounds
)

func GetPrimaryMonitor() MonitorRect {
	displays := sdlDisplays()
	if len(displays) == 0 {
		return MonitorRect{0, 0, 1920, 1080}
	}
	primary := sdlPrimaryDisplay()
	if primary != 0 {
		if r, ok := sdlDisplayBounds(primary); ok {
			return MonitorRect(r)
		}
	}
	if r, ok := sdlDisplayBounds(displays[0]); ok {
		return MonitorRect(r)
	}
	var bestArea int32
	var best MonitorRect
	for _, id := range displays {
		if r, ok := sdlDisplayBounds(id); ok {
			area := r.W * r.H
			if area > bestArea {
				bestArea = area
				best = MonitorRect(r)
			}
		}
	}
	if bestArea > 0 {
		return best
	}
	return MonitorRect{0, 0, 1920, 1080}
}
