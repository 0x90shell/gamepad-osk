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

// promptFontPath is where the Arch package installs Promptfont. Other
// locations (e.g. ~/.local/share/fonts as described in the README) are found
// by findPromptFont.
const promptFontPath = "/usr/share/fonts/TTF/promptfont.ttf"

// fontDirs lists the usual font roots. Distributions differ in the sub-folder
// layout (Arch: TTF/, Debian: truetype/<family>/, Fedora: <family>-fonts/),
// so they are searched recursively.
func fontDirs() []string {
	dirs := []string{"/usr/share/fonts", "/usr/local/share/fonts"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, ".local/share/fonts"),
			filepath.Join(home, ".fonts"))
	}
	return dirs
}

// findFontFile searches dirs recursively for one of the given file names
// (case-insensitive). Earlier names win over later ones, regardless of which
// directory they are found in.
func findFontFile(dirs []string, names ...string) string {
	want := make(map[string]int, len(names))
	for i, n := range names {
		want[strings.ToLower(n)] = i
	}
	best, bestRank := "", len(names)
	for _, root := range dirs {
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

// fcMatch asks fontconfig for the file of the best match for pattern.
// Returns "" if fc-match is missing or the result is not a usable font file.
func fcMatch(pattern string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "fc-match", "--format=%{file}", pattern).Output() //nolint:gosec // G204: fixed binary, pattern from our code
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(out))
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".ttf" && ext != ".otf" {
		return ""
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// findUIFont returns the font used for key labels: the known fonts first
// (same preference as before, now found in any sub-folder), then whatever
// fontconfig picks as the default sans-serif.
func findUIFont() string {
	if p := findFontFile(fontDirs(),
		"DejaVuSans.ttf", "LiberationSans-Regular.ttf", "FreeSans.ttf"); p != "" {
		return p
	}
	return fcMatch("sans-serif:style=Regular")
}

// findPromptFont returns the Promptfont file for controller glyphs, or "".
func findPromptFont() string {
	if _, err := os.Stat(promptFontPath); err == nil {
		return promptFontPath
	}
	if p := findFontFile(fontDirs(), "promptfont.ttf", "promptfont.otf"); p != "" {
		return p
	}
	// fc-match always returns something; only accept an actual Promptfont.
	if p := fcMatch("promptfont"); strings.Contains(strings.ToLower(filepath.Base(p)), "promptfont") {
		return p
	}
	return ""
}
