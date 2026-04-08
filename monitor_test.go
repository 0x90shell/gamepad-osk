package main

import "testing"

func TestIntersectRect_Overlap(t *testing.T) {
	// Monitor at 0,0 1920x1080, workarea at 0,0 1920x1040 (40px bottom panel)
	mon := MonitorRect{0, 0, 1920, 1080}
	wa := MonitorRect{0, 0, 1920, 1040}
	got := intersectRect(mon, wa)
	if got != wa {
		t.Errorf("bottom panel: got %+v, want %+v", got, wa)
	}
}

func TestIntersectRect_TopPanel(t *testing.T) {
	mon := MonitorRect{0, 0, 1920, 1080}
	wa := MonitorRect{0, 36, 1920, 1044} // 36px top panel
	got := intersectRect(mon, wa)
	if got != wa {
		t.Errorf("top panel: got %+v, want %+v", got, wa)
	}
}

func TestIntersectRect_MultiMonitor(t *testing.T) {
	// _NET_WORKAREA spans all monitors: 6400x1440
	// Primary is at 1280,0 2560x1440 with 40px panel
	mon := MonitorRect{1280, 0, 2560, 1440}
	wa := MonitorRect{0, 0, 6400, 1400} // combined workarea, 40px panel
	got := intersectRect(mon, wa)
	want := MonitorRect{1280, 0, 2560, 1400}
	if got != want {
		t.Errorf("multi-monitor: got %+v, want %+v", got, want)
	}
}

func TestIntersectRect_NoOverlap(t *testing.T) {
	// No overlap should fall back to first rect (monitor bounds)
	a := MonitorRect{0, 0, 1920, 1080}
	b := MonitorRect{5000, 5000, 100, 100}
	got := intersectRect(a, b)
	if got != a {
		t.Errorf("no overlap: got %+v, want fallback to %+v", got, a)
	}
}

func TestIntersectRect_SecondMonitorOffset(t *testing.T) {
	// Second monitor to the right, workarea accounts for left panel
	mon := MonitorRect{1920, 0, 2560, 1440}
	wa := MonitorRect{48, 0, 4432, 1440} // 48px left panel on first monitor
	got := intersectRect(mon, wa)
	want := MonitorRect{1920, 0, 2560, 1440} // second monitor unaffected
	if got != want {
		t.Errorf("second monitor: got %+v, want %+v", got, want)
	}
}
