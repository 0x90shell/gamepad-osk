package main

import (
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
)

// isTTY returns true if stdout is a terminal (not piped or redirected).
// Both fmt.Printf (stdout) and log.Print (stderr) display on the same terminal
// in normal use, so checking stdout covers both.
func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func colorRed(msg string) string {
	if isTTY() {
		return "\033[1;31m" + msg + "\033[0m" // bold red
	}
	return msg
}

func colorYellow(msg string) string {
	if isTTY() {
		return "\033[1;33m" + msg + "\033[0m" // bold yellow
	}
	return msg
}

func colorGreen(msg string) string {
	if isTTY() {
		return "\033[1;32m" + msg + "\033[0m" // bold green
	}
	return msg
}

func colorDim(msg string) string {
	if isTTY() {
		return "\033[2m" + msg + "\033[0m" // dim/faint
	}
	return msg
}

// logPermissionFix logs actionable fix steps for input permission errors.
// Used by both injector (uinput) and gamepad (evdev) error paths.
func logPermissionFix() {
	if inGroup, _ := isUserInGroup("input"); inGroup {
		log.Print(colorYellow("  You are in the 'input' group but it may not be active in this session."))
		log.Print(colorYellow("  Check: run 'id' and verify 'input' appears in the output"))
		log.Print(colorYellow("  Fix: fully log out of your desktop session and log back in"))
	} else {
		log.Print(colorYellow("  Fix: sudo usermod -aG input $USER (then fully log out and back in)"))
	}
	log.Print(colorDim("  Tip: run 'gamepad-osk --setup' to diagnose and fix permission issues"))
}

// isUserInGroup checks if the current user is a member of the named group
// by reading /etc/group (covers persistent membership, not just active session).
func isUserInGroup(groupName string) (bool, error) {
	data, err := os.ReadFile("/etc/group")
	if err != nil {
		return false, err
	}
	username := os.Getenv("USER")
	if username == "" {
		return false, nil
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		parts := strings.SplitN(line, ":", 4)
		if len(parts) == 4 && parts[0] == groupName {
			for member := range strings.SplitSeq(parts[3], ",") {
				if member == username {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// isGroupActiveInSession checks if the named group is active in the current
// process credentials (not just /etc/group membership).
func isGroupActiveInSession(groupName string) bool {
	gids, err := os.Getgroups()
	if err != nil {
		return false
	}
	targetGID := groupGID(groupName)
	if targetGID < 0 {
		return false
	}
	return slices.Contains(gids, targetGID)
}

// groupGID returns the GID for a group name, or -1 if not found.
func groupGID(groupName string) int {
	data, err := os.ReadFile("/etc/group")
	if err != nil {
		return -1
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		parts := strings.SplitN(line, ":", 4)
		if len(parts) >= 3 && parts[0] == groupName {
			gid, err := strconv.Atoi(parts[2])
			if err == nil {
				return gid
			}
		}
	}
	return -1
}
