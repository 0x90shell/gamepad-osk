package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain points HOME and every XDG directory at a temp dir before any
// test runs, so no test can read or change the real user's config or
// socket, even one that forgets t.Setenv. The AUR check() runs go test in
// the packager's own session. Tests that need other values still set them.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gamepad-osk-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, v := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME",
		"XDG_CACHE_HOME", "XDG_RUNTIME_DIR"} {
		p := filepath.Join(dir, v)
		if err := os.MkdirAll(p, 0700); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_ = os.Setenv(v, p)
	}
	for _, v := range []string{"SUDO_USER", "SUDO_UID", "SUDO_GID"} {
		_ = os.Unsetenv(v)
	}
	sockPath = initSockPath() // computed at package init, from the real XDG_RUNTIME_DIR
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// The guard itself: config and socket paths are inside the temp dir.
func TestIsolatedPaths(t *testing.T) {
	tmp := os.TempDir()
	for name, p := range map[string]string{"user config": UserConfigPath(), "socket": sockPath} {
		if rel, err := filepath.Rel(tmp, p); err != nil || strings.HasPrefix(rel, "..") {
			t.Errorf("%s path %s is not under %s", name, p, tmp)
		}
	}
}
