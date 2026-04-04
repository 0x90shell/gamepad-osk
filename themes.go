package main

import "github.com/veandco/go-sdl2/sdl"

type Theme struct {
	Name               string
	Bg                 sdl.Color
	KeyBg              sdl.Color
	KeyBgPressed       sdl.Color
	KeyBorder          sdl.Color
	KeyText            sdl.Color
	HighlightBg        sdl.Color
	HighlightBorder    sdl.Color
	ModifierBg         sdl.Color
	ModifierActiveBg   sdl.Color
	ModifierActiveText sdl.Color // pill text color (dark on light bg, white on dark bg)
	ModifierText       sdl.Color
	FnKeyBg            sdl.Color
	AccentPopupBg      sdl.Color
	AccentPopupText    sdl.Color
	AccentHighlightBg  sdl.Color
	GlyphColor         sdl.Color
}

func c(r, g, b uint8) sdl.Color { return sdl.Color{R: r, G: g, B: b, A: 255} }

var w = c(255, 255, 255) // white pill text
var d = c(20, 20, 20)    // dark pill text

var Themes = map[string]Theme{
	// --- Core themes ---
	"dark": {Name: "dark", Bg: c(30, 30, 35), KeyBg: c(55, 55, 65), KeyBgPressed: c(80, 80, 95),
		KeyBorder: c(70, 70, 80), KeyText: c(220, 220, 230), HighlightBg: c(60, 110, 180),
		HighlightBorder: c(80, 140, 220), ModifierBg: c(50, 50, 60),
		ModifierActiveBg: c(90, 70, 130), ModifierActiveText: w, ModifierText: c(180, 180, 200),
		FnKeyBg: c(45, 45, 55), AccentPopupBg: c(50, 50, 60),
		AccentPopupText: c(220, 220, 230), AccentHighlightBg: c(60, 110, 180), GlyphColor: c(120, 120, 140)},
	"steam_green": {Name: "steam_green", Bg: c(22, 32, 22), KeyBg: c(35, 55, 35), KeyBgPressed: c(50, 80, 50),
		KeyBorder: c(45, 70, 45), KeyText: c(200, 220, 200), HighlightBg: c(56, 150, 60),
		HighlightBorder: c(76, 185, 80), ModifierBg: c(30, 48, 30),
		ModifierActiveBg: c(40, 120, 45), ModifierActiveText: w, ModifierText: c(170, 200, 170),
		FnKeyBg: c(28, 44, 28), AccentPopupBg: c(32, 50, 32),
		AccentPopupText: c(200, 220, 200), AccentHighlightBg: c(56, 150, 60), GlyphColor: c(100, 160, 100)},
	"high_contrast": {Name: "high_contrast", Bg: c(0, 0, 0), KeyBg: c(20, 20, 20), KeyBgPressed: c(60, 60, 60),
		KeyBorder: c(200, 200, 200), KeyText: c(255, 255, 255), HighlightBg: c(255, 255, 0),
		HighlightBorder: c(255, 255, 255), ModifierBg: c(10, 10, 10),
		ModifierActiveBg: c(200, 0, 0), ModifierActiveText: w, ModifierText: c(255, 255, 255),
		FnKeyBg: c(15, 15, 15), AccentPopupBg: c(20, 20, 20),
		AccentPopupText: c(255, 255, 255), AccentHighlightBg: c(255, 255, 0), GlyphColor: c(180, 180, 0)},
	"terminal": {Name: "terminal", Bg: c(0, 0, 0), KeyBg: c(10, 18, 10), KeyBgPressed: c(20, 40, 20),
		KeyBorder: c(0, 50, 0), KeyText: c(0, 204, 0), HighlightBg: c(0, 80, 0),
		HighlightBorder: c(0, 120, 0), ModifierBg: c(8, 14, 8),
		ModifierActiveBg: c(0, 160, 0), ModifierActiveText: d, ModifierText: c(0, 140, 0),
		FnKeyBg: c(6, 12, 6), AccentPopupBg: c(10, 18, 10),
		AccentPopupText: c(0, 204, 0), AccentHighlightBg: c(0, 80, 0), GlyphColor: c(0, 100, 0)},

	// --- Popular editor/terminal themes ---
	"nord": {Name: "nord", Bg: c(46, 52, 64), KeyBg: c(59, 66, 82), KeyBgPressed: c(76, 86, 106),
		KeyBorder: c(67, 76, 94), KeyText: c(216, 222, 233), HighlightBg: c(94, 129, 172),
		HighlightBorder: c(129, 161, 193), ModifierBg: c(59, 66, 82),
		ModifierActiveBg: c(136, 192, 208), ModifierActiveText: d, ModifierText: c(180, 190, 210),
		FnKeyBg: c(52, 60, 74), AccentPopupBg: c(59, 66, 82),
		AccentPopupText: c(216, 222, 233), AccentHighlightBg: c(94, 129, 172), GlyphColor: c(76, 86, 106)},
	"dracula": {Name: "dracula", Bg: c(40, 42, 54), KeyBg: c(68, 71, 90), KeyBgPressed: c(98, 114, 164),
		KeyBorder: c(80, 83, 105), KeyText: c(248, 248, 242), HighlightBg: c(189, 147, 249),
		HighlightBorder: c(255, 121, 198), ModifierBg: c(55, 57, 72),
		ModifierActiveBg: c(255, 85, 85), ModifierActiveText: w, ModifierText: c(200, 200, 220),
		FnKeyBg: c(50, 52, 66), AccentPopupBg: c(68, 71, 90),
		AccentPopupText: c(248, 248, 242), AccentHighlightBg: c(189, 147, 249), GlyphColor: c(98, 114, 164)},
	"gruvbox": {Name: "gruvbox", Bg: c(40, 40, 40), KeyBg: c(60, 56, 54), KeyBgPressed: c(80, 73, 69),
		KeyBorder: c(80, 73, 69), KeyText: c(235, 219, 178), HighlightBg: c(215, 153, 33),
		HighlightBorder: c(250, 189, 47), ModifierBg: c(50, 48, 47),
		ModifierActiveBg: c(184, 187, 38), ModifierActiveText: d, ModifierText: c(189, 174, 147),
		FnKeyBg: c(50, 48, 47), AccentPopupBg: c(60, 56, 54),
		AccentPopupText: c(235, 219, 178), AccentHighlightBg: c(215, 153, 33), GlyphColor: c(124, 111, 100)},
	"catppuccin": {Name: "catppuccin", Bg: c(30, 30, 46), KeyBg: c(49, 50, 68), KeyBgPressed: c(69, 71, 90),
		KeyBorder: c(58, 59, 78), KeyText: c(205, 214, 244), HighlightBg: c(137, 180, 250),
		HighlightBorder: c(180, 190, 254), ModifierBg: c(43, 44, 60),
		ModifierActiveBg: c(245, 194, 231), ModifierActiveText: d, ModifierText: c(166, 173, 200),
		FnKeyBg: c(36, 37, 52), AccentPopupBg: c(49, 50, 68),
		AccentPopupText: c(205, 214, 244), AccentHighlightBg: c(137, 180, 250), GlyphColor: c(88, 91, 112)},
	"solarized": {Name: "solarized", Bg: c(0, 43, 54), KeyBg: c(7, 54, 66), KeyBgPressed: c(88, 110, 117),
		KeyBorder: c(0, 54, 66), KeyText: c(147, 161, 161), HighlightBg: c(38, 139, 210),
		HighlightBorder: c(42, 161, 152), ModifierBg: c(0, 43, 54),
		ModifierActiveBg: c(133, 153, 0), ModifierActiveText: d, ModifierText: c(131, 148, 150),
		FnKeyBg: c(0, 38, 48), AccentPopupBg: c(7, 54, 66),
		AccentPopupText: c(147, 161, 161), AccentHighlightBg: c(38, 139, 210), GlyphColor: c(88, 110, 117)},
	"monokai": {Name: "monokai", Bg: c(39, 40, 34), KeyBg: c(55, 56, 48), KeyBgPressed: c(75, 76, 66),
		KeyBorder: c(65, 66, 56), KeyText: c(248, 248, 242), HighlightBg: c(166, 226, 46),
		HighlightBorder: c(190, 240, 80), ModifierBg: c(45, 46, 38),
		ModifierActiveBg: c(249, 38, 114), ModifierActiveText: w, ModifierText: c(200, 200, 190),
		FnKeyBg: c(42, 43, 36), AccentPopupBg: c(55, 56, 48),
		AccentPopupText: c(248, 248, 242), AccentHighlightBg: c(166, 226, 46), GlyphColor: c(117, 113, 94)},
	"onedark": {Name: "onedark", Bg: c(40, 44, 52), KeyBg: c(55, 60, 72), KeyBgPressed: c(75, 82, 99),
		KeyBorder: c(63, 68, 82), KeyText: c(171, 178, 191), HighlightBg: c(97, 175, 239),
		HighlightBorder: c(130, 195, 250), ModifierBg: c(48, 52, 63),
		ModifierActiveBg: c(224, 108, 117), ModifierActiveText: w, ModifierText: c(150, 158, 172),
		FnKeyBg: c(44, 48, 58), AccentPopupBg: c(55, 60, 72),
		AccentPopupText: c(171, 178, 191), AccentHighlightBg: c(97, 175, 239), GlyphColor: c(92, 99, 112)},
	"tokyo_night": {Name: "tokyo_night", Bg: c(26, 27, 38), KeyBg: c(41, 46, 66), KeyBgPressed: c(59, 66, 97),
		KeyBorder: c(50, 56, 80), KeyText: c(169, 177, 214), HighlightBg: c(122, 162, 247),
		HighlightBorder: c(158, 186, 255), ModifierBg: c(33, 37, 53),
		ModifierActiveBg: c(187, 154, 247), ModifierActiveText: d, ModifierText: c(140, 150, 190),
		FnKeyBg: c(30, 32, 45), AccentPopupBg: c(41, 46, 66),
		AccentPopupText: c(169, 177, 214), AccentHighlightBg: c(122, 162, 247), GlyphColor: c(68, 75, 106)},
	"everforest": {Name: "everforest", Bg: c(47, 53, 47), KeyBg: c(63, 72, 60), KeyBgPressed: c(80, 92, 76),
		KeyBorder: c(70, 80, 66), KeyText: c(211, 198, 170), HighlightBg: c(143, 191, 115),
		HighlightBorder: c(167, 210, 140), ModifierBg: c(53, 61, 52),
		ModifierActiveBg: c(219, 188, 127), ModifierActiveText: d, ModifierText: c(180, 170, 150),
		FnKeyBg: c(50, 57, 49), AccentPopupBg: c(63, 72, 60),
		AccentPopupText: c(211, 198, 170), AccentHighlightBg: c(143, 191, 115), GlyphColor: c(90, 102, 86)},
	"kanagawa": {Name: "kanagawa", Bg: c(31, 31, 40), KeyBg: c(43, 43, 56), KeyBgPressed: c(60, 60, 80),
		KeyBorder: c(54, 54, 70), KeyText: c(220, 215, 186), HighlightBg: c(127, 180, 202),
		HighlightBorder: c(160, 200, 220), ModifierBg: c(36, 36, 48),
		ModifierActiveBg: c(195, 64, 67), ModifierActiveText: w, ModifierText: c(180, 176, 156),
		FnKeyBg: c(34, 34, 44), AccentPopupBg: c(43, 43, 56),
		AccentPopupText: c(220, 215, 186), AccentHighlightBg: c(127, 180, 202), GlyphColor: c(84, 84, 109)},
	"gotham": {Name: "gotham", Bg: c(10, 15, 20), KeyBg: c(17, 27, 33), KeyBgPressed: c(30, 45, 55),
		KeyBorder: c(20, 35, 42), KeyText: c(152, 209, 206), HighlightBg: c(38, 139, 210),
		HighlightBorder: c(50, 160, 230), ModifierBg: c(13, 21, 26),
		ModifierActiveBg: c(195, 132, 24), ModifierActiveText: d, ModifierText: c(120, 170, 168),
		FnKeyBg: c(12, 18, 23), AccentPopupBg: c(17, 27, 33),
		AccentPopupText: c(152, 209, 206), AccentHighlightBg: c(38, 139, 210), GlyphColor: c(40, 62, 72)},
	"horizon": {Name: "horizon", Bg: c(28, 30, 38), KeyBg: c(45, 47, 58), KeyBgPressed: c(65, 68, 82),
		KeyBorder: c(53, 56, 68), KeyText: c(205, 200, 192), HighlightBg: c(233, 86, 120),
		HighlightBorder: c(250, 110, 145), ModifierBg: c(35, 38, 48),
		ModifierActiveBg: c(250, 180, 80), ModifierActiveText: d, ModifierText: c(175, 172, 165),
		FnKeyBg: c(32, 34, 43), AccentPopupBg: c(45, 47, 58),
		AccentPopupText: c(205, 200, 192), AccentHighlightBg: c(233, 86, 120), GlyphColor: c(90, 93, 108)},

	// --- Fun/retro themes ---
	"candy": {Name: "candy", Bg: c(40, 18, 55), KeyBg: c(70, 35, 90), KeyBgPressed: c(100, 55, 125),
		KeyBorder: c(85, 45, 110), KeyText: c(240, 210, 250), HighlightBg: c(210, 80, 140),
		HighlightBorder: c(240, 110, 170), ModifierBg: c(60, 28, 78),
		ModifierActiveBg: c(180, 60, 120), ModifierActiveText: w, ModifierText: c(220, 190, 230),
		FnKeyBg: c(55, 25, 72), AccentPopupBg: c(65, 30, 85),
		AccentPopupText: c(240, 210, 250), AccentHighlightBg: c(210, 80, 140), GlyphColor: c(180, 120, 160)},
	"ocean": {Name: "ocean", Bg: c(15, 25, 45), KeyBg: c(25, 45, 75), KeyBgPressed: c(35, 65, 105),
		KeyBorder: c(35, 55, 90), KeyText: c(190, 215, 240), HighlightBg: c(30, 120, 190),
		HighlightBorder: c(50, 150, 220), ModifierBg: c(20, 38, 65),
		ModifierActiveBg: c(25, 100, 170), ModifierActiveText: w, ModifierText: c(170, 200, 230),
		FnKeyBg: c(20, 35, 60), AccentPopupBg: c(22, 40, 70),
		AccentPopupText: c(190, 215, 240), AccentHighlightBg: c(30, 120, 190), GlyphColor: c(100, 150, 200)},
	"retro": {Name: "retro", Bg: c(0, 0, 0), KeyBg: c(18, 12, 0), KeyBgPressed: c(40, 28, 0),
		KeyBorder: c(50, 35, 0), KeyText: c(255, 176, 0), HighlightBg: c(80, 56, 0),
		HighlightBorder: c(120, 84, 0), ModifierBg: c(14, 10, 0),
		ModifierActiveBg: c(200, 140, 0), ModifierActiveText: d, ModifierText: c(180, 126, 0),
		FnKeyBg: c(12, 8, 0), AccentPopupBg: c(18, 12, 0),
		AccentPopupText: c(255, 176, 0), AccentHighlightBg: c(80, 56, 0), GlyphColor: c(100, 70, 0)},
	"synthwave": {Name: "synthwave", Bg: c(13, 2, 33), KeyBg: c(30, 10, 60), KeyBgPressed: c(50, 20, 90),
		KeyBorder: c(45, 15, 80), KeyText: c(230, 180, 255), HighlightBg: c(255, 0, 110),
		HighlightBorder: c(255, 50, 150), ModifierBg: c(22, 6, 48),
		ModifierActiveBg: c(0, 255, 255), ModifierActiveText: d, ModifierText: c(180, 140, 220),
		FnKeyBg: c(20, 5, 42), AccentPopupBg: c(30, 10, 60),
		AccentPopupText: c(230, 180, 255), AccentHighlightBg: c(255, 0, 110), GlyphColor: c(120, 40, 180)},
	"neon": {Name: "neon", Bg: c(5, 5, 5), KeyBg: c(15, 15, 15), KeyBgPressed: c(30, 30, 30),
		KeyBorder: c(0, 255, 128), KeyText: c(255, 255, 255), HighlightBg: c(255, 0, 255),
		HighlightBorder: c(0, 255, 255), ModifierBg: c(10, 10, 10),
		ModifierActiveBg: c(0, 255, 0), ModifierActiveText: d, ModifierText: c(200, 200, 200),
		FnKeyBg: c(12, 12, 12), AccentPopupBg: c(15, 15, 15),
		AccentPopupText: c(255, 255, 255), AccentHighlightBg: c(255, 0, 255), GlyphColor: c(0, 180, 90)},

	// --- New batch ---
	"rose_pine": {Name: "rose_pine", Bg: c(25, 23, 36), KeyBg: c(38, 35, 53), KeyBgPressed: c(57, 53, 82),
		KeyBorder: c(46, 42, 66), KeyText: c(224, 222, 244), HighlightBg: c(196, 167, 231),
		HighlightBorder: c(235, 188, 186), ModifierBg: c(30, 28, 44),
		ModifierActiveBg: c(235, 111, 146), ModifierActiveText: d, ModifierText: c(190, 186, 220),
		FnKeyBg: c(28, 26, 40), AccentPopupBg: c(38, 35, 53),
		AccentPopupText: c(224, 222, 244), AccentHighlightBg: c(196, 167, 231), GlyphColor: c(80, 75, 110)},
	"ayu_dark": {Name: "ayu_dark", Bg: c(10, 14, 20), KeyBg: c(22, 28, 38), KeyBgPressed: c(38, 46, 60),
		KeyBorder: c(30, 37, 50), KeyText: c(203, 204, 198), HighlightBg: c(255, 180, 84),
		HighlightBorder: c(255, 200, 120), ModifierBg: c(16, 21, 30),
		ModifierActiveBg: c(57, 186, 230), ModifierActiveText: d, ModifierText: c(160, 162, 158),
		FnKeyBg: c(13, 17, 25), AccentPopupBg: c(22, 28, 38),
		AccentPopupText: c(203, 204, 198), AccentHighlightBg: c(255, 180, 84), GlyphColor: c(60, 70, 90)},
	"material": {Name: "material", Bg: c(38, 50, 56), KeyBg: c(55, 71, 79), KeyBgPressed: c(78, 93, 100),
		KeyBorder: c(66, 82, 90), KeyText: c(236, 239, 241), HighlightBg: c(0, 150, 136),
		HighlightBorder: c(0, 188, 170), ModifierBg: c(45, 60, 67),
		ModifierActiveBg: c(255, 82, 82), ModifierActiveText: w, ModifierText: c(200, 210, 215),
		FnKeyBg: c(42, 55, 62), AccentPopupBg: c(55, 71, 79),
		AccentPopupText: c(236, 239, 241), AccentHighlightBg: c(0, 150, 136), GlyphColor: c(96, 120, 130)},
	"cobalt": {Name: "cobalt", Bg: c(0, 25, 60), KeyBg: c(10, 40, 85), KeyBgPressed: c(20, 60, 120),
		KeyBorder: c(15, 50, 100), KeyText: c(230, 240, 255), HighlightBg: c(60, 140, 255),
		HighlightBorder: c(100, 170, 255), ModifierBg: c(5, 32, 72),
		ModifierActiveBg: c(255, 200, 0), ModifierActiveText: d, ModifierText: c(180, 200, 230),
		FnKeyBg: c(3, 28, 65), AccentPopupBg: c(10, 40, 85),
		AccentPopupText: c(230, 240, 255), AccentHighlightBg: c(60, 140, 255), GlyphColor: c(40, 80, 140)},
	"midnight": {Name: "midnight", Bg: c(12, 5, 20), KeyBg: c(25, 12, 40), KeyBgPressed: c(42, 22, 65),
		KeyBorder: c(35, 16, 55), KeyText: c(210, 190, 240), HighlightBg: c(120, 60, 200),
		HighlightBorder: c(160, 90, 240), ModifierBg: c(18, 8, 30),
		ModifierActiveBg: c(100, 50, 180), ModifierActiveText: w, ModifierText: c(170, 150, 200),
		FnKeyBg: c(15, 7, 25), AccentPopupBg: c(25, 12, 40),
		AccentPopupText: c(210, 190, 240), AccentHighlightBg: c(120, 60, 200), GlyphColor: c(70, 35, 120)},
	"ember": {Name: "ember", Bg: c(15, 5, 0), KeyBg: c(35, 12, 5), KeyBgPressed: c(60, 22, 10),
		KeyBorder: c(50, 18, 8), KeyText: c(255, 200, 150), HighlightBg: c(200, 60, 10),
		HighlightBorder: c(240, 100, 30), ModifierBg: c(25, 8, 2),
		ModifierActiveBg: c(255, 120, 0), ModifierActiveText: d, ModifierText: c(200, 150, 100),
		FnKeyBg: c(20, 7, 1), AccentPopupBg: c(35, 12, 5),
		AccentPopupText: c(255, 200, 150), AccentHighlightBg: c(200, 60, 10), GlyphColor: c(120, 45, 10)},
	"ice": {Name: "ice", Bg: c(15, 20, 30), KeyBg: c(30, 40, 55), KeyBgPressed: c(50, 65, 85),
		KeyBorder: c(40, 52, 70), KeyText: c(200, 220, 245), HighlightBg: c(100, 180, 255),
		HighlightBorder: c(150, 210, 255), ModifierBg: c(22, 30, 42),
		ModifierActiveBg: c(170, 220, 255), ModifierActiveText: d, ModifierText: c(160, 185, 215),
		FnKeyBg: c(18, 25, 36), AccentPopupBg: c(30, 40, 55),
		AccentPopupText: c(200, 220, 245), AccentHighlightBg: c(100, 180, 255), GlyphColor: c(70, 95, 130)},
	"forest": {Name: "forest", Bg: c(10, 18, 10), KeyBg: c(20, 35, 18), KeyBgPressed: c(35, 55, 30),
		KeyBorder: c(28, 45, 24), KeyText: c(180, 210, 170), HighlightBg: c(50, 120, 40),
		HighlightBorder: c(70, 150, 55), ModifierBg: c(15, 26, 14),
		ModifierActiveBg: c(80, 160, 60), ModifierActiveText: d, ModifierText: c(140, 175, 130),
		FnKeyBg: c(12, 22, 12), AccentPopupBg: c(20, 35, 18),
		AccentPopupText: c(180, 210, 170), AccentHighlightBg: c(50, 120, 40), GlyphColor: c(40, 70, 35)},
	"slate": {Name: "slate", Bg: c(30, 34, 38), KeyBg: c(48, 53, 58), KeyBgPressed: c(68, 74, 80),
		KeyBorder: c(58, 63, 69), KeyText: c(200, 205, 210), HighlightBg: c(90, 100, 115),
		HighlightBorder: c(120, 130, 145), ModifierBg: c(38, 42, 47),
		ModifierActiveBg: c(130, 140, 155), ModifierActiveText: d, ModifierText: c(170, 175, 180),
		FnKeyBg: c(34, 38, 42), AccentPopupBg: c(48, 53, 58),
		AccentPopupText: c(200, 205, 210), AccentHighlightBg: c(90, 100, 115), GlyphColor: c(80, 88, 96)},
}

func GetTheme(name string) Theme {
	if t, ok := Themes[name]; ok {
		return t
	}
	return Themes["dark"]
}
