package main

import (
	"math"
	"time"
)

type KeyboardState struct {
	Layout          [][]KeyDef
	CursorRow       int
	CursorCol       int
	ShiftActive     bool
	CapsActive      bool
	CtrlActive      bool
	AltActive       bool
	MetaActive      bool
	LongPressStart  time.Time
	LongPressActive bool
	AccentPopup     *AccentPopupState
	targetX         float64
	targetXSet      bool

	// Visual flash for shortcut keys
	FlashCode  int
	FlashUntil time.Time

	// Callbacks for Cfg key
	OnThemeCycle        func()
	OnThemeCycleReverse func()
}

type AccentPopupState struct {
	Accents  []AccentDef
	Selected int
}

func NewKeyboardState(layout [][]KeyDef) *KeyboardState {
	return &KeyboardState{
		Layout:    layout,
		CursorRow: 2,
		CursorCol: 1, // land on 'q'
	}
}

func (kb *KeyboardState) Navigate(dx, dy int) {
	if kb.AccentPopup != nil {
		newIdx := kb.AccentPopup.Selected + dx
		if newIdx >= 0 && newIdx < len(kb.AccentPopup.Accents) {
			kb.AccentPopup.Selected = newIdx
		}
		return
	}
	if dy != 0 {
		if !kb.targetXSet {
			kb.targetX = kb.keyCenterX(kb.CursorRow, kb.CursorCol)
			kb.targetXSet = true
		}
		kb.CursorRow = (kb.CursorRow + dy + len(kb.Layout)) % len(kb.Layout)
		kb.CursorCol = kb.findClosestCol(kb.CursorRow, kb.targetX)
	} else if dx != 0 {
		row := kb.Layout[kb.CursorRow]
		kb.CursorCol = (kb.CursorCol + dx + len(row)) % len(row)
		kb.targetXSet = false
	}
}

func (kb *KeyboardState) keyCenterX(rowIdx, colIdx int) float64 {
	x := 0.0
	for i, key := range kb.Layout[rowIdx] {
		if i == colIdx {
			return x + key.Width/2.0
		}
		x += key.Width
	}
	return x
}

func (kb *KeyboardState) findClosestCol(rowIdx int, targetX float64) int {
	bestCol := 0
	bestDist := math.Inf(1)
	x := 0.0
	for i, key := range kb.Layout[rowIdx] {
		center := x + key.Width/2.0
		dist := math.Abs(center - targetX)
		if dist < bestDist {
			bestDist = dist
			bestCol = i
		}
		x += key.Width
	}
	return bestCol
}

func (kb *KeyboardState) CurrentKey() KeyDef {
	row := kb.Layout[kb.CursorRow]
	col := kb.CursorCol
	if col >= len(row) {
		col = len(row) - 1
	}
	return row[col]
}

func (kb *KeyboardState) DisplayLabel(key KeyDef) string {
	if (kb.ShiftActive != kb.CapsActive) && key.ShiftLabel != "" {
		return key.ShiftLabel
	}
	return key.Label
}

func (kb *KeyboardState) PressCurrent(inj *Injector) {
	if kb.AccentPopup != nil {
		accent := kb.AccentPopup.Accents[kb.AccentPopup.Selected]
		if inj != nil {
			inj.TypeUnicode(accent.Codepoint)
		}
		kb.CloseAccentPopup()
		return
	}

	key := kb.CurrentKey()
	if key.IsModifier {
		kb.toggleModifier(key)
		return
	}

	// Paste/Copy button - Paste normally, Copy when shifted
	if key.Label == "Paste" {
		shiftOn := kb.ShiftActive != kb.CapsActive
		if shiftOn {
			if inj != nil {
				inj.PressKey(KEY_C, []int{KEY_LEFTCTRL})
			}
			if kb.ShiftActive && !kb.CapsActive {
				kb.ShiftActive = false
			}
		} else if inj != nil {
			inj.PressKey(KEY_V, []int{KEY_LEFTCTRL})
		}
		return
	}

	// Cfg button - cycle theme (shift = reverse)
	if key.Label == "Cfg" {
		shiftOn := kb.ShiftActive != kb.CapsActive
		if shiftOn {
			if kb.OnThemeCycleReverse != nil {
				kb.OnThemeCycleReverse()
			}
			if kb.ShiftActive && !kb.CapsActive {
				kb.ShiftActive = false
			}
		} else if kb.OnThemeCycle != nil {
			kb.OnThemeCycle()
		}
		return
	}

	shiftOn := kb.ShiftActive != kb.CapsActive

	// Combo keys (shortcut row): default sends combo, shift sends ShiftCode
	if len(key.Combo) > 0 || key.ShiftCode != 0 {
		if inj != nil {
			if shiftOn && key.ShiftCode != 0 {
				inj.PressKey(key.ShiftCode, nil)
			} else {
				inj.PressKey(key.Code, key.Combo)
			}
		}
		if kb.ShiftActive && !kb.CapsActive {
			kb.ShiftActive = false
		}
		kb.CtrlActive = false
		kb.AltActive = false
		kb.MetaActive = false
		return
	}

	code := key.Code

	var mods []int
	if shiftOn && code != KEY_UP && code != KEY_DOWN && code != KEY_LEFT && code != KEY_RIGHT &&
		code != KEY_ESC && code != KEY_DELETE {
		mods = append(mods, KEY_LEFTSHIFT)
	}
	if kb.CtrlActive {
		mods = append(mods, KEY_LEFTCTRL)
	}
	if kb.AltActive {
		mods = append(mods, KEY_LEFTALT)
	}
	if kb.MetaActive {
		mods = append(mods, KEY_LEFTMETA)
	}

	if inj != nil {
		inj.PressKey(code, mods)
	}

	if kb.ShiftActive && !kb.CapsActive {
		kb.ShiftActive = false
	}
	kb.CtrlActive = false
	kb.AltActive = false
	kb.MetaActive = false
}

func (kb *KeyboardState) StartLongPress() {
	key := kb.CurrentKey()
	if len(key.Accents) > 0 {
		kb.LongPressStart = time.Now()
		kb.LongPressActive = true
	}
}

func (kb *KeyboardState) CancelLongPress() {
	kb.LongPressActive = false
}

func (kb *KeyboardState) CheckLongPress(thresholdMs int) bool {
	if !kb.LongPressActive {
		return false
	}
	if time.Since(kb.LongPressStart).Milliseconds() >= int64(thresholdMs) {
		key := kb.CurrentKey()
		if len(key.Accents) > 0 {
			kb.AccentPopup = &AccentPopupState{Accents: key.Accents, Selected: 0}
			kb.LongPressActive = false
			return true
		}
	}
	return false
}

func (kb *KeyboardState) CloseAccentPopup() {
	kb.AccentPopup = nil
	kb.LongPressActive = false
}

func (kb *KeyboardState) FlashKey(code int) {
	kb.FlashCode = code
	kb.FlashUntil = time.Now().Add(150 * time.Millisecond)
}

func (kb *KeyboardState) IsFlashed(key KeyDef) bool {
	if kb.FlashCode != 0 && key.Code == kb.FlashCode {
		if time.Now().Before(kb.FlashUntil) {
			return true
		}
		kb.FlashCode = 0
	}
	return false
}

func (kb *KeyboardState) toggleModifier(key KeyDef) {
	switch key.ModifierType {
	case "shift":
		kb.ShiftActive = !kb.ShiftActive
	case "caps":
		kb.CapsActive = !kb.CapsActive
	case "ctrl":
		kb.CtrlActive = !kb.CtrlActive
	case "alt":
		kb.AltActive = !kb.AltActive
	case "meta":
		kb.MetaActive = !kb.MetaActive
	}
}
