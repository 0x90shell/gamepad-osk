package main

import (
	"os"
	"strings"
	"testing"
)

func TestHasSystemd(_ *testing.T) {
	// Should not panic, just returns a bool
	_ = hasSystemd()
}

func TestIsGroupActiveInSession(_ *testing.T) {
	// Should not panic; result depends on current user's groups
	_ = isGroupActiveInSession("input")
	_ = isGroupActiveInSession("nonexistent-group-12345")
}

func TestGroupGID(t *testing.T) {
	// root group should always exist with GID 0
	gid := groupGID("root")
	if gid != 0 {
		t.Errorf("groupGID(\"root\") = %d, want 0", gid)
	}
	// Nonexistent group returns -1
	gid = groupGID("nonexistent-group-12345")
	if gid != -1 {
		t.Errorf("groupGID(\"nonexistent\") = %d, want -1", gid)
	}
}

func TestUdevRuleContent_Valid(t *testing.T) {
	if !strings.HasPrefix(udevRuleContent, "#") {
		t.Error("udev rule content should start with a comment")
	}
	if !strings.Contains(udevRuleContent, "KERNEL==\"event*\"") {
		t.Error("udev rule should contain event* rule")
	}
	if !strings.Contains(udevRuleContent, "KERNEL==\"uinput\"") {
		t.Error("udev rule should contain uinput rule")
	}
	if !strings.Contains(udevRuleContent, "GROUP=\"input\"") {
		t.Error("udev rule should set GROUP to input")
	}
}

func TestSystemdServiceContent_Valid(t *testing.T) {
	if !strings.Contains(systemdServiceTemplate, "[Unit]") {
		t.Error("service template should contain [Unit]")
	}
	if !strings.Contains(systemdServiceTemplate, "[Service]") {
		t.Error("service template should contain [Service]")
	}
	if !strings.Contains(systemdServiceTemplate, "[Install]") {
		t.Error("service template should contain [Install]")
	}
	if !strings.Contains(systemdServiceTemplate, "%s") { //nolint:govet // not a format string
		t.Errorf("service template should contain %s placeholder for binary path", "%%s")
	}
}

func TestSetupDiagnose_NoPanic(_ *testing.T) {
	// Smoke test: runDiagnose must not panic.
	// Redirect stdout to suppress diagnostic output during tests.
	old := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = old }()
	_ = runDiagnose()
}

func TestIsUserInGroup(t *testing.T) {
	// root group always exists
	_, err := isUserInGroup("root")
	if err != nil {
		t.Errorf("isUserInGroup(\"root\") error: %v", err)
	}
	// Nonexistent group should return false, no error
	inGroup, err := isUserInGroup("nonexistent-group-12345")
	if err != nil {
		t.Errorf("isUserInGroup(\"nonexistent\") error: %v", err)
	}
	if inGroup {
		t.Error("should not be in nonexistent group")
	}
}
