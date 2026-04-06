package main

import "log"

// ButtonInfo maps a config button name to evdev code and Promptfont glyph.
type ButtonInfo struct {
	EvdevBtn  uint16 // for EV_KEY buttons (BTN_*)
	EvdevAxis uint16 // for EV_ABS axes (ABS_Z, ABS_RZ) — 0 if not axis-based
	IsAxis    bool
	Glyph     string // Promptfont Unicode character
}

// buttonTable returns the mapping from button name to evdev info.
// swap_xy swaps evdev codes for "x" and "y" (Xbox 360 Linux driver quirk).
func isSwapXY(cfg GamepadConfig) bool {
	return cfg.SwapXY == "true"
}

func buttonTable(swapXY bool) map[string]ButtonInfo {
	xBtn := uint16(BTN_WEST)
	yBtn := uint16(BTN_NORTH)
	if swapXY {
		xBtn, yBtn = yBtn, xBtn
	}
	return map[string]ButtonInfo{
		"a":      {EvdevBtn: BTN_SOUTH, Glyph: "\u21a7"},  // face bottom
		"b":      {EvdevBtn: BTN_EAST, Glyph: "\u21a6"},   // face right
		"x":      {EvdevBtn: xBtn, Glyph: "\u21a4"},       // face left
		"y":      {EvdevBtn: yBtn, Glyph: "\u21a5"},       // face top
		"lb":     {EvdevBtn: BTN_TL, Glyph: "\u2198"},     // left shoulder
		"rb":     {EvdevBtn: BTN_TR, Glyph: "\u2199"},     // right shoulder
		"l3":     {EvdevBtn: BTN_THUMBL, Glyph: "\u21ba"}, // left stick click
		"r3":     {EvdevBtn: BTN_THUMBR, Glyph: "\u21bb"}, // right stick click
		"start":  {EvdevBtn: 0x13b, Glyph: "\u21f8"},      // BTN_START
		"select": {EvdevBtn: 0x13a, Glyph: "\u21f7"},      // BTN_SELECT
		"lt":     {EvdevBtn: BTN_TL2, EvdevAxis: ABS_Z, IsAxis: true, Glyph: "\u2196"},  // left trigger (axis on Xbox/DS, button on Switch)
		"rt":     {EvdevBtn: BTN_TR2, EvdevAxis: ABS_RZ, IsAxis: true, Glyph: "\u2197"}, // right trigger (axis on Xbox/DS, button on Switch)
	}
}

// ActionMap maps evdev codes to actions, built from config at startup.
type ActionMap struct {
	BtnPress     map[uint16]ActionType // EV_KEY press actions
	BtnRelease   map[uint16]ActionType // EV_KEY release actions (for hold buttons like mouse)
	AxisActions  map[uint16]ActionType // ABS axis actions (triggers)
	AxisRelease  map[uint16]ActionType // ABS axis release actions (for repeat stop)
}

func BuildActionMap(cfg GamepadConfig) ActionMap {
	table := buttonTable(isSwapXY(cfg))
	press := make(map[uint16]ActionType)
	release := make(map[uint16]ActionType)
	axes := make(map[uint16]ActionType)
	axesRelease := make(map[uint16]ActionType)

	setBtn := func(buttonName string, action ActionType) {
		info, ok := table[buttonName]
		if !ok {
			log.Printf("Warning: unknown button %q in config", buttonName)
			return
		}
		if info.IsAxis {
			axes[info.EvdevAxis] = action
			// Dual-map: also register as button for controllers that send digital triggers (Switch Pro)
			if info.EvdevBtn != 0 {
				press[info.EvdevBtn] = action
			}
		} else {
			press[info.EvdevBtn] = action
		}
	}

	b := cfg.Buttons
	setBtn(b.Press, ActionPress)    // special: handled as press_start/press
	setBtn(b.Close, ActionClose)
	setBtn(b.Backspace, ActionBackspace)
	setBtn(b.Space, ActionSpace)
	setBtn(b.Shift, ActionShiftOn)
	setBtn(b.Enter, ActionEnter)
	setBtn(b.PositionToggle, ActionPositionToggle)

	// Auto-assign stick clicks: mouse stick click = left click, nav stick click = caps
	if cfg.MouseStick == "left" {
		press[BTN_THUMBL] = ActionLeftClick
		release[BTN_THUMBL] = ActionLeftClickRelease
		press[BTN_THUMBR] = ActionCapsToggle
	} else {
		press[BTN_THUMBR] = ActionLeftClick
		release[BTN_THUMBR] = ActionLeftClickRelease
		press[BTN_THUMBL] = ActionCapsToggle
	}

	// Repeatable keys: need release to stop key repeat (buttons and axes)
	for _, pair := range []struct{ name string; action ActionType }{
		{b.Backspace, ActionBackspaceRelease},
		{b.Space, ActionSpaceRelease},
		{b.Enter, ActionEnterRelease},
	} {
		if info, ok := table[pair.name]; ok {
			if info.IsAxis {
				axesRelease[info.EvdevAxis] = pair.action
				// Dual-map: also register button release for digital triggers (Switch Pro)
				if info.EvdevBtn != 0 {
					release[info.EvdevBtn] = pair.action
				}
			} else {
				release[info.EvdevBtn] = pair.action
			}
		}
	}

	// Mouse clicks: need both press and release for hold/drag
	if info, ok := table[b.LeftClick]; ok && !info.IsAxis {
		press[info.EvdevBtn] = ActionLeftClick
		release[info.EvdevBtn] = ActionLeftClickRelease
	}
	if info, ok := table[b.RightClick]; ok && !info.IsAxis {
		press[info.EvdevBtn] = ActionRightClick
		release[info.EvdevBtn] = ActionRightClickRelease
	}

	return ActionMap{BtnPress: press, BtnRelease: release, AxisActions: axes, AxisRelease: axesRelease}
}

// BuildKeyGlyphs maps keyboard evdev codes to Promptfont glyphs
// based on which controller button triggers that action.
func BuildKeyGlyphs(cfg GamepadConfig) map[int]string {
	table := buttonTable(isSwapXY(cfg))
	b := cfg.Buttons
	glyphs := make(map[int]string)

	mappings := []struct {
		keyCode    int
		buttonName string
	}{
		{KEY_BACKSPACE, b.Backspace},
		{KEY_SPACE, b.Space},
		{KEY_ENTER, b.Enter},
		{KEY_LEFTSHIFT, b.Shift},
		{KEY_RIGHTSHIFT, b.Shift},
		{KEY_CAPSLOCK, func() string { if cfg.MouseStick == "left" { return "r3" }; return "l3" }()},
	}

	for _, m := range mappings {
		if info, ok := table[m.buttonName]; ok {
			glyphs[m.keyCode] = info.Glyph
		}
	}

	// Cfg key always gets the gear icon
	glyphs[0] = "\u2699"

	return glyphs
}
