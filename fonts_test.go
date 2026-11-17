package main

import (
	"os"
	"path/filepath"
	"testing"
)

func touchFont(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // G301: test dir
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil { //nolint:gosec // G306: test file
		t.Fatal(err)
	}
	return path
}

// stubFcMatch replaces fontconfig with a fixed answer and records queries.
func stubFcMatch(t *testing.T, answer string) *[]string {
	t.Helper()
	var queries []string
	orig := fcMatch
	fcMatch = func(pattern string) string {
		queries = append(queries, pattern)
		return answer
	}
	t.Cleanup(func() { fcMatch = orig })
	return &queries
}

var testFamilies = []string{"DejaVu Sans", "Liberation Sans", "FreeSans"}

func TestFindFont_FixedPathWins(t *testing.T) {
	dir := t.TempDir()
	fixed := touchFont(t, filepath.Join(dir, "fixed", "DejaVuSans.ttf"))
	touchFont(t, filepath.Join(dir, "root", "dejavu-sans-fonts", "DejaVuSans.ttf"))
	stubFcMatch(t, "")

	got := findFontIn([]string{filepath.Join(dir, "fixed")}, []string{filepath.Join(dir, "root")}, testFamilies...)
	if got != fixed {
		t.Errorf("findFontIn = %q, want fixed path %q", got, fixed)
	}
}

func TestFindFont_FedoraLayout(t *testing.T) {
	dir := t.TempDir()
	want := touchFont(t, filepath.Join(dir, "liberation-sans-fonts", "LiberationSans-Regular.ttf"))
	stubFcMatch(t, "")

	got := findFontIn([]string{filepath.Join(dir, "missing")}, []string{dir}, testFamilies...)
	if got != want {
		t.Errorf("findFontIn = %q, want %q", got, want)
	}
}

func TestFindFont_RecursiveKeepsFamilyOrder(t *testing.T) {
	dir := t.TempDir()
	// Liberation sorts first in the walk, but DejaVu is the preferred family
	touchFont(t, filepath.Join(dir, "a-liberation", "LiberationSans-Regular.ttf"))
	want := touchFont(t, filepath.Join(dir, "z-dejavu", "DejaVuSans.ttf"))
	stubFcMatch(t, "")

	got := findFontIn(nil, []string{dir}, testFamilies...)
	if got != want {
		t.Errorf("findFontIn = %q, want preferred family %q", got, want)
	}
}

func TestFindFont_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	want := touchFont(t, filepath.Join(dir, "dejavu", "dejavusans.TTF"))
	stubFcMatch(t, "")

	if got := findFontIn(nil, []string{dir}, testFamilies...); got != want {
		t.Errorf("findFontIn = %q, want %q", got, want)
	}
}

func TestFindFont_SymlinkedRoot(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "fonts")
	touchFont(t, filepath.Join(target, "FreeSans.ttf"))
	link := filepath.Join(dir, "fonts")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	stubFcMatch(t, "")

	got := findFontIn(nil, []string{link}, testFamilies...)
	if filepath.Base(got) != "FreeSans.ttf" {
		t.Errorf("findFontIn through symlinked root = %q, want FreeSans.ttf", got)
	}
}

func TestFindFont_FontconfigFallback(t *testing.T) {
	dir := t.TempDir()
	noto := touchFont(t, filepath.Join(dir, "NotoSans-Regular.ttf"))
	queries := stubFcMatch(t, noto)

	got := findFontIn(nil, []string{filepath.Join(dir, "missing")}, testFamilies...)
	if got != noto {
		t.Errorf("findFontIn = %q, want fontconfig answer %q", got, noto)
	}
	if len(*queries) == 0 || (*queries)[0] != "DejaVu Sans" {
		t.Errorf("fontconfig queries = %v, want first query for the preferred family", *queries)
	}
}

func TestFindFont_NothingFound(t *testing.T) {
	queries := stubFcMatch(t, "")

	if got := findFontIn(nil, []string{t.TempDir()}, testFamilies...); got != "" {
		t.Errorf("findFontIn = %q, want empty", got)
	}
	last := (*queries)[len(*queries)-1]
	if last != "sans-serif" {
		t.Errorf("last fontconfig query = %q, want sans-serif", last)
	}
}

func TestFindPromptFont_UserDir(t *testing.T) {
	dir := t.TempDir()
	want := touchFont(t, filepath.Join(dir, "local", "fonts", "promptfont.ttf"))
	stubFcMatch(t, "")

	got := findPromptFontIn(filepath.Join(dir, "missing.ttf"), []string{filepath.Join(dir, "system"), filepath.Join(dir, "local", "fonts")})
	if got != want {
		t.Errorf("findPromptFontIn = %q, want %q", got, want)
	}
}

func TestFindPromptFont_FixedPathWins(t *testing.T) {
	dir := t.TempDir()
	fixed := touchFont(t, filepath.Join(dir, "TTF", "promptfont.ttf"))
	touchFont(t, filepath.Join(dir, "local", "promptfont.ttf"))
	stubFcMatch(t, "")

	if got := findPromptFontIn(fixed, []string{filepath.Join(dir, "local")}); got != fixed {
		t.Errorf("findPromptFontIn = %q, want %q", got, fixed)
	}
}

func TestFindPromptFont_RejectsFontconfigSubstitute(t *testing.T) {
	dir := t.TempDir()
	stubFcMatch(t, touchFont(t, filepath.Join(dir, "DejaVuSans.ttf")))

	if got := findPromptFontIn(filepath.Join(dir, "missing.ttf"), nil); got != "" {
		t.Errorf("findPromptFontIn = %q, want empty (fontconfig substitute is not Promptfont)", got)
	}
}

func TestFindPromptFont_FontconfigHit(t *testing.T) {
	dir := t.TempDir()
	want := touchFont(t, filepath.Join(dir, "elsewhere", "promptfont.otf"))
	stubFcMatch(t, want)

	if got := findPromptFontIn(filepath.Join(dir, "missing.ttf"), nil); got != want {
		t.Errorf("findPromptFontIn = %q, want %q", got, want)
	}
}

func TestFontRoots_XDGDataHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg/data")
	found := false
	for _, r := range fontRoots() {
		if r == "/xdg/data/fonts" {
			found = true
		}
	}
	if !found {
		t.Errorf("fontRoots() = %v, want it to include $XDG_DATA_HOME/fonts", fontRoots())
	}
}
