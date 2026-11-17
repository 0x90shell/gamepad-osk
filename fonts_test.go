package main

import (
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindFontFileLayouts(t *testing.T) {
	cases := map[string]string{
		"arch":   "TTF/DejaVuSans.ttf",
		"debian": "truetype/dejavu/DejaVuSans.ttf",
		"fedora": "liberation-sans-fonts/LiberationSans-Regular.ttf",
	}
	for name, rel := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			want := filepath.Join(root, rel)
			touch(t, want)
			got := findFontFile([]string{root},
				"DejaVuSans.ttf", "LiberationSans-Regular.ttf", "FreeSans.ttf")
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestFindFontFilePreference(t *testing.T) {
	sys, user := t.TempDir(), t.TempDir()
	touch(t, filepath.Join(sys, "gnu-free/FreeSans.ttf"))
	want := filepath.Join(user, "DejaVuSans.ttf")
	touch(t, want)
	got := findFontFile([]string{sys, user}, "DejaVuSans.ttf", "FreeSans.ttf")
	if got != want {
		t.Errorf("earlier name should win across dirs: got %q, want %q", got, want)
	}
}

func TestFindFontFileCaseAndMissing(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "PromptFont.TTF")
	touch(t, want)
	if got := findFontFile([]string{root}, "promptfont.ttf"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := findFontFile([]string{root, filepath.Join(root, "nope")}, "DejaVuSans.ttf"); got != "" {
		t.Errorf("expected no match, got %q", got)
	}
}
