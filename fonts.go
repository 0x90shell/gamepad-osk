package main

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const promptFontPath = "/usr/share/fonts/TTF/promptfont.ttf"

// Known locations checked first, in order. Cheap stat calls, no directory walk.
var fixedFontDirs = []string{
	"/usr/share/fonts/TTF",                 // Arch
	"/usr/share/fonts/truetype/dejavu",     // Debian
	"/usr/share/fonts/truetype/liberation", // Debian
	"/usr/share/fonts/truetype/freefont",   // Debian
	"/usr/share/fonts/truetype",
	"/usr/share/fonts",
}

var fontFiles = map[string][]string{
	"DejaVu Sans":     {"DejaVuSans.ttf"},
	"Liberation Sans": {"LiberationSans-Regular.ttf"},
	"FreeSans":        {"FreeSans.ttf"},
}

// fontRoots are searched recursively when the fixed paths miss. Covers
// layouts like Fedora's /usr/share/fonts/<package>/ and per-user installs.
func fontRoots() []string {
	roots := []string{"/usr/share/fonts", "/usr/local/share/fonts"}
	dataHome := os.Getenv("XDG_DATA_HOME")
	home, err := os.UserHomeDir()
	if dataHome == "" && err == nil {
		dataHome = filepath.Join(home, ".local/share")
	}
	if dataHome != "" {
		roots = append(roots, filepath.Join(dataHome, "fonts"))
	}
	if err == nil {
		roots = append(roots, filepath.Join(home, ".fonts"))
	}
	return roots
}

// fcMatch asks fontconfig for the file of the best match for pattern.
// Returns "" if fc-match is missing or the result is not a usable font file.
// A var so tests can stub it.
var fcMatch = func(pattern string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "fc-match", "--format=%{file}", pattern).Output() //nolint:gosec // G204: fixed binary, pattern from our code
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(out))
	if !isFontFile(path) {
		return ""
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

func isFontFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttf", ".otf":
		return true
	}
	return false
}

// searchFontFile walks roots recursively for the given file names
// (case-insensitive). Earlier names win over later ones; earlier roots
// win over later roots for the same name.
func searchFontFile(roots []string, names ...string) string {
	want := make(map[string]int, len(names))
	for i, n := range names {
		want[strings.ToLower(n)] = i
	}
	best, bestRank := "", len(names)
	for _, root := range roots {
		// WalkDir does not descend into a symlinked root, and
		// ~/.local/share/fonts is often a symlink into a dotfiles repo.
		if r, err := filepath.EvalSymlinks(root); err == nil {
			root = r
		} else {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil //nolint:nilerr // unreadable entries are skipped
			}
			if rank, ok := want[strings.ToLower(d.Name())]; ok && rank < bestRank {
				best, bestRank = path, rank
			}
			return nil
		})
		if bestRank == 0 {
			break
		}
	}
	return best
}

// findFont returns the first available font for the given family names.
// Order: fixed paths, recursive search of the font roots, then fontconfig.
func findFont(names ...string) string {
	return findFontIn(fixedFontDirs, fontRoots(), names...)
}

func findFontIn(fixed, roots []string, names ...string) string {
	var files []string
	for _, name := range names {
		files = append(files, fontFiles[name]...)
	}

	for _, name := range names {
		for _, dir := range fixed {
			for _, f := range fontFiles[name] {
				path := filepath.Join(dir, f)
				if _, err := os.Stat(path); err == nil {
					return path
				}
			}
		}
	}

	if p := searchFontFile(roots, files...); p != "" {
		return p
	}

	// fontconfig always returns its closest match, which may be a different
	// family. Any sans font beats exiting with "no suitable font found".
	for _, name := range names {
		if p := fcMatch(name); p != "" {
			return p
		}
	}
	return fcMatch("sans-serif")
}

// findPromptFont returns the Promptfont file for controller glyphs, or "".
func findPromptFont() string {
	return findPromptFontIn(promptFontPath, fontRoots())
}

func findPromptFontIn(fixed string, roots []string) string {
	if _, err := os.Stat(fixed); err == nil {
		return fixed
	}
	if p := searchFontFile(roots, "promptfont.ttf", "promptfont.otf"); p != "" {
		return p
	}
	// fc-match falls back to an unrelated font when Promptfont is missing,
	// so only accept a result that is actually Promptfont.
	if p := fcMatch("promptfont"); strings.Contains(strings.ToLower(filepath.Base(p)), "promptfont") {
		return p
	}
	return ""
}
