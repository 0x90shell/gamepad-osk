package main

import (
	"math"
	"strings"
	"time"
)

type KeyboardState struct {
	Layout          [][]KeyDef
	CursorRow       int
	CursorCol       int
	ShiftActive     bool
	ShiftHeld       bool // true when held via gamepad trigger (not auto-released)
	CapsActive      bool
	CtrlActive      bool
	AltActive       bool
	MetaActive      bool
	LongPressStart  time.Time
	LongPressActive bool
	AccentPopup     *AccentPopupState
	SkipPress       bool // the popup was cancelled; the next press release types nothing
	targetX         float64
	targetXSet      bool

	// Alt+Tab cycling: Alt held at OS level across multiple Tab presses
	AltTabHeld    bool
	AltTabLastTab time.Time // last Tab press, for auto-release timeout

	// Visual flash for shortcut keys
	FlashCode  int
	FlashUntil time.Time

	// Callbacks for the Theme key
	OnThemeCycle        func()
	OnThemeCycleReverse func()

	// Callbacks for live mouse sensitivity adjustment (Shift+↑/↓)
	OnSensitivityUp   func()
	OnSensitivityDown func()

	// OnNotice shows a short message in the status bar
	OnNotice func(msg string)
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
	if kb.Shifted(key) && key.ShiftLabel != "" {
		return key.ShiftLabel
	}
	return key.Label
}

// isLetter: the a to z keys, the only keys Caps changes.
func isLetter(key KeyDef) bool {
	return len(key.Label) == 1 && key.Label[0] >= 'a' && key.Label[0] <= 'z'
}

// Shifted reports whether key gets its Shift meaning. Caps changes letters
// only, as on a PC keyboard (Caps and Shift together give lowercase); every
// other key follows Shift alone.
func (kb *KeyboardState) Shifted(key KeyDef) bool {
	if isLetter(key) {
		return kb.ShiftActive != kb.CapsActive
	}
	return kb.ShiftActive
}

// AccentText is the popup accent as shown and typed: a capital with Caps on
// (Shift is taken as the popup trigger). Letters with no single capital keep
// their form (ß).
func (kb *KeyboardState) AccentText(a AccentDef) (string, int) {
	if !kb.CapsActive {
		return a.Label, a.Codepoint
	}
	up := strings.ToUpper(a.Label)
	return up, int([]rune(up)[0])
}

func (kb *KeyboardState) PressCurrent(inj *Injector) {
	if kb.AccentPopup != nil {
		_, cp := kb.AccentText(kb.AccentPopup.Accents[kb.AccentPopup.Selected])
		if inj != nil {
			inj.TypeUnicode(cp)
		}
		kb.CloseAccentPopup()
		if !kb.ShiftHeld { // a tapped Shift opened the popup: the accent uses it up
			kb.ShiftActive = false
		}
		kb.clearArmed()
		return
	}

	key := kb.CurrentKey()
	if key.IsModifier {
		kb.toggleModifier(key)
		return
	}

	// Paste/Copy button - Paste normally, Copy when shifted
	if key.Label == "Paste" {
		if kb.ShiftActive {
			if inj != nil {
				inj.PressKey(KEY_C, []int{KEY_LEFTCTRL})
			}
			if !kb.ShiftHeld { // a held trigger keeps Shift on
				kb.ShiftActive = false
			}
		} else if inj != nil {
			inj.PressKey(KEY_V, []int{KEY_LEFTCTRL})
		}
		kb.clearArmed()
		return
	}

	// Theme key - cycle theme (shift = reverse)
	if key.Label == themeKeyLabel {
		if kb.ShiftActive {
			if kb.OnThemeCycleReverse != nil {
				kb.OnThemeCycleReverse()
			}
			if !kb.ShiftHeld { // a held trigger keeps Shift on
				kb.ShiftActive = false
			}
		} else if kb.OnThemeCycle != nil {
			kb.OnThemeCycle()
		}
		kb.clearArmed()
		return
	}

	shiftOn := kb.Shifted(key)

	// AltTab cycling: hold Alt across multiple Tab presses
	if key.Label == "AltTab" && !shiftOn {
		if !kb.AltTabHeld {
			if inj != nil {
				inj.HoldKey(KEY_LEFTALT)
			}
			kb.AltTabHeld = true
		}
		kb.AltTabLastTab = time.Now()
		if inj != nil {
			inj.PressKey(KEY_TAB, nil)
		}
		kb.clearArmed()
		return
	}

	// Release held Alt if pressing any other key
	kb.ReleaseAltTab(inj)

	// Shift+↑/↓: adjust mouse sensitivity instead of sending key
	if shiftOn && (key.Code == KEY_UP || key.Code == KEY_DOWN) {
		if key.Code == KEY_UP && kb.OnSensitivityUp != nil {
			kb.OnSensitivityUp()
		} else if key.Code == KEY_DOWN && kb.OnSensitivityDown != nil {
			kb.OnSensitivityDown()
		}
		if !kb.ShiftHeld { // a held trigger keeps Shift on
			kb.ShiftActive = false
		}
		kb.clearArmed()
		return
	}

	// Shortcut row: default sends the key or its combo, Shift sends the F-key.
	// Home, End, PgUp, PgDn and the F-keys take the armed modifiers (Ctrl+Home,
	// Alt+F4); the fixed shortcuts and the media keys ignore them.
	if len(key.Combo) > 0 || key.ShiftCode != 0 {
		code, mods := key.Code, key.Combo
		if shiftOn && key.ShiftCode != 0 {
			code, mods = key.ShiftCode, kb.armedMods()
		} else if len(key.Combo) == 0 && code != KEY_PLAYPAUSE && code != KEY_MUTE {
			mods = kb.armedMods()
		}
		if !kb.blockRisky(code) && inj != nil {
			inj.PressKey(code, mods)
		}
		if !kb.ShiftHeld { // a held trigger keeps Shift on
			kb.ShiftActive = false
		}
		kb.clearArmed()
		return
	}

	code := key.Code

	var mods []int
	// No Shift on Esc and Delete: Shift+Delete deletes past the trash in file managers.
	if shiftOn && code != KEY_ESC && code != KEY_DELETE {
		mods = append(mods, KEY_LEFTSHIFT)
	}
	mods = append(mods, kb.armedMods()...)

	if !kb.blockRisky(code) && inj != nil {
		inj.PressKey(code, mods)
	}

	if kb.ShiftActive && !kb.ShiftHeld {
		kb.ShiftActive = false
	}
	kb.clearArmed()
}

// riskyCombos are the keys that, with Ctrl+Alt, can leave a couch user stuck
// with no keyboard in reach. They are not sent (blockRisky).
var riskyCombos = map[int]bool{
	KEY_ESC:       true, // KDE kill-window mode: the next click kills a window
	KEY_BACKSPACE: true, // X11 with zap on: ends the session
	// F1 to F12: switch to a text console, where this keyboard cannot show
	KEY_F1: true, KEY_F2: true, KEY_F3: true, KEY_F4: true, KEY_F5: true, KEY_F6: true,
	KEY_F7: true, KEY_F8: true, KEY_F9: true, KEY_F10: true, KEY_F11: true, KEY_F12: true,
}

// blockRisky reports whether Ctrl+Alt+code is a risky combo, and says so in
// the status bar. The caller then sends nothing; the armed modifiers are
// used up as for any key.
func (kb *KeyboardState) blockRisky(code int) bool {
	if !riskyCombos[code] || !kb.CtrlActive || !kb.AltActive {
		return false
	}
	if kb.OnNotice != nil {
		kb.OnNotice("Blocked risky combo")
	}
	return true
}

// armedMods is the armed Ctrl, Alt and Super, in that order.
func (kb *KeyboardState) armedMods() []int {
	var mods []int
	if kb.CtrlActive {
		mods = append(mods, KEY_LEFTCTRL)
	}
	if kb.AltActive {
		mods = append(mods, KEY_LEFTALT)
	}
	if kb.MetaActive {
		mods = append(mods, KEY_LEFTMETA)
	}
	return mods
}

// clearArmed ends the armed Ctrl, Alt and Super: any key press but a
// modifier uses them up, so they never carry over to a later key.
func (kb *KeyboardState) clearArmed() {
	kb.CtrlActive = false
	kb.AltActive = false
	kb.MetaActive = false
}

// CheckAltTabTimeout auto-releases Alt if the cycling has been idle too long.
// Returns true if Alt was released.
func (kb *KeyboardState) CheckAltTabTimeout(inj *Injector) bool {
	if kb.AltTabHeld && time.Since(kb.AltTabLastTab) >= 3*time.Second {
		kb.ReleaseAltTab(inj)
		return true
	}
	return false
}

// ReleaseAltTab releases held Alt from Alt+Tab cycling if active.
func (kb *KeyboardState) ReleaseAltTab(inj *Injector) {
	if !kb.AltTabHeld {
		return
	}
	if inj != nil {
		inj.ReleaseKey(KEY_LEFTALT)
	}
	kb.AltTabHeld = false
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

// CancelAccentPopup closes the popup without typing. The press button is
// still held, so its release must not type the plain key either.
func (kb *KeyboardState) CancelAccentPopup() {
	kb.CloseAccentPopup()
	kb.SkipPress = true
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
