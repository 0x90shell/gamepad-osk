package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	udevRulePath = "/usr/lib/udev/rules.d/80-gamepad-osk.rules"
	udevRuleAlt  = "/etc/udev/rules.d/80-gamepad-osk.rules"
)

// Hardcoded canonical content - reviewed and versioned in source control.
// Kept in sync with gamepad-osk.udev but intentionally not go:embed'd
// to prevent accidental deployment of corrupted rule files.
const udevRuleContent = "# gamepad-osk: gamepad reading + virtual keyboard injection\n" +
	"KERNEL==\"event*\", SUBSYSTEM==\"input\", GROUP=\"input\", MODE=\"0660\", TAG+=\"uaccess\"\n" +
	"KERNEL==\"uinput\", SUBSYSTEM==\"misc\", OPTIONS+=\"static_node=uinput\", GROUP=\"input\", MODE=\"0660\", TAG+=\"uaccess\"\n"

const systemdServiceTemplate = `[Unit]
Description=gamepad-osk - Gamepad on-screen keyboard
After=graphical-session.target

[Service]
Type=simple
ExecStartPre=/bin/sleep 3
ExecStart=%s --daemon
Restart=on-failure
RestartSec=5

[Install]
WantedBy=graphical-session.target
`

func hasSystemd() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

// canWritePath tests if we can create/write a file at the given path.
func canWritePath(path string) bool {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".gamepad-osk-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// runSetup performs system diagnostics (install=false) or deployment (install=true).
func runSetup(install bool) int {
	if install {
		return runInstall()
	}
	return runDiagnose()
}

func runDiagnose() int {
	fmt.Printf("gamepad-osk v%s system check:\n", version)
	fmt.Println()

	if os.Geteuid() == 0 {
		fmt.Println(colorYellow("  Note: running as root - results reflect root access, not your user's."))
		fmt.Println(colorDim("  Run without sudo for an accurate check of your user's permissions."))
		fmt.Println()
	}

	issues := 0
	configIssues := 0

	// Check device access first - udev rule status depends on this
	uinputOK := false
	switch f, err := os.OpenFile("/dev/uinput", os.O_RDWR, 0); { //nolint:gosec // G304: constant path
	case err == nil:
		_ = f.Close()
		uinputOK = true
		fmt.Printf("  %-14s %s writable\n", "/dev/uinput", colorGreen("[✓]"))
	case os.IsPermission(err):
		fmt.Printf("  %-14s %s permission denied\n", "/dev/uinput", colorRed("[✗]"))
		issues++
	case os.IsNotExist(err):
		fmt.Printf("  %-14s %s not found (sudo modprobe uinput)\n", "/dev/uinput", colorRed("[✗]"))
		issues++
	default:
		fmt.Printf("  %-14s %s %v\n", "/dev/uinput", colorRed("[✗]"), err)
		issues++
	}

	inputOK := false
	matches, _ := filepath.Glob("/dev/input/event*")
	if len(matches) == 0 {
		fmt.Printf("  %-14s %s no event devices found\n", "/dev/input", colorDim("[~]"))
	} else {
		for _, dev := range matches {
			if f, err := os.OpenFile(dev, os.O_RDONLY, 0); err == nil { //nolint:gosec // G304: /dev/input path
				_ = f.Close()
				inputOK = true
				break
			}
		}
		if inputOK {
			fmt.Printf("  %-14s %s readable\n", "/dev/input", colorGreen("[✓]"))
		} else {
			fmt.Printf("  %-14s %s permission denied on all event devices\n", "/dev/input", colorRed("[✗]"))
			issues++
		}
	}

	// udev rules - only flag as a problem if device access is also failing
	udevFound := false
	for _, path := range []string{udevRulePath, udevRuleAlt} {
		if _, err := os.Stat(path); err == nil {
			fmt.Printf("  %-14s %s %s\n", "udev rules", colorGreen("[✓]"), path)
			udevFound = true
			break
		}
	}
	if !udevFound {
		if uinputOK && inputOK {
			fmt.Printf("  %-14s %s not installed (not needed - device access works)\n", "udev rules", colorDim("[~]"))
		} else {
			fmt.Printf("  %-14s %s not found\n", "udev rules", colorRed("[✗]"))
			issues++
		}
	}

	// 4. input group
	inGroup, _ := isUserInGroup("input")
	activeInSession := isGroupActiveInSession("input")
	switch {
	case inGroup && activeInSession:
		fmt.Printf("  %-14s %s member and active in session\n", "input group", colorGreen("[✓]"))
	case inGroup && !activeInSession:
		fmt.Printf("  %-14s %s member in /etc/group but NOT active in session (need re-login)\n",
			"input group", colorYellow("[!]"))
	case !inGroup:
		// Check if device access works anyway (logind ACLs)
		if f, err := os.OpenFile("/dev/uinput", os.O_RDWR, 0); err == nil { //nolint:gosec // G304: constant path
			_ = f.Close()
			fmt.Printf("  %-14s %s not a member, but device access works (logind ACL)\n",
				"input group", colorGreen("[✓]"))
		} else {
			fmt.Printf("  %-14s %s not a member\n", "input group", colorYellow("[!]"))
			fmt.Printf("  %-14s   %s\n", "", colorYellow("Fix: sudo usermod -aG input $USER"))
		}
	}

	// 5. config
	userCfg := UserConfigPath()
	if _, err := os.Stat(userCfg); err == nil {
		// Suppress log output from config parsing (unknown section warnings etc.)
		log.SetOutput(io.Discard)
		cfg := LoadConfig(userCfg)
		log.SetOutput(os.Stderr)
		problems := checkConfigFile(userCfg)
		problems = append(problems, checkConfig(cfg)...)
		if len(problems) > 0 {
			fmt.Printf("  %-14s %s %s\n", "config", colorYellow("[!]"), userCfg)
			for _, issue := range problems {
				fmt.Printf("  %-14s   %s\n", "", colorYellow(issue))
			}
			fmt.Printf("  %-14s   %s\n", "", colorDim("Edit "+userCfg+" to fix"))
			configIssues++
			issues++
		} else {
			fmt.Printf("  %-14s %s %s\n", "config", colorGreen("[✓]"), userCfg)
		}
	} else {
		fmt.Printf("  %-14s %s %s (will be created on first run)\n", "config", colorDim("[~]"), userCfg)
	}

	// 6. systemd service - check both system-wide and user-local paths
	if hasSystemd() {
		systemSvc := "/usr/lib/systemd/user/gamepad-osk.service"
		userSvc := systemdUserServicePath()
		if _, err := os.Stat(systemSvc); err == nil {
			fmt.Printf("  %-14s %s %s\n", "systemd", colorGreen("[✓]"), systemSvc)
		} else if _, err := os.Stat(userSvc); err == nil {
			fmt.Printf("  %-14s %s %s\n", "systemd", colorGreen("[✓]"), userSvc)
		} else {
			fmt.Printf("  %-14s %s user service not installed (optional)\n", "systemd", colorDim("[~]"))
		}
	} else {
		fmt.Printf("  %-14s %s not detected (skipped)\n", "systemd", colorDim("[~]"))
	}

	fmt.Println()
	if issues > 0 {
		if issues > configIssues {
			fmt.Println(colorYellow("  Permission issues found. Run: sudo gamepad-osk --setup --install"))
		}
		if configIssues > 0 {
			fmt.Println(colorYellow("  Config issues found. Edit the config file to fix, then re-run --setup to verify."))
		}
		return 1
	}
	fmt.Println(colorGreen("  All checks passed."))
	return 0
}

func runInstall() int {
	fmt.Printf("gamepad-osk v%s setup:\n", version)
	fmt.Println()

	// 1. udev rules
	if canWritePath(udevRulePath) {
		existed := false
		if _, err := os.Stat(udevRulePath); err == nil {
			existed = true
		}
		if err := os.WriteFile(udevRulePath, []byte(udevRuleContent), 0644); err != nil { //nolint:gosec // G306: udev rules are world-readable
			fmt.Printf("  %-14s %s %v\n", "udev rules", colorRed("[✗]"), err)
		} else if existed {
			fmt.Printf("  %-14s %s %s (updated)\n", "udev rules", colorGreen("[✓]"), udevRulePath)
		} else {
			fmt.Printf("  %-14s %s %s\n", "udev rules", colorGreen("[✓]"), udevRulePath)
		}

		// Reload udev
		ctx := context.Background()
		if out, err := exec.CommandContext(ctx, "udevadm", "control", "--reload-rules").CombinedOutput(); err != nil {
			fmt.Printf("  %-14s %s %s\n", "udev reload", colorRed("[✗]"), string(out))
		} else {
			fmt.Printf("  %-14s %s\n", "udev reload", colorGreen("[✓]"))
		}
		if out, err := exec.CommandContext(ctx, "udevadm", "trigger").CombinedOutput(); err != nil {
			fmt.Printf("  %-14s %s %s\n", "udev trigger", colorRed("[✗]"), string(out))
		} else {
			fmt.Printf("  %-14s %s\n", "udev trigger", colorGreen("[✓]"))
		}
	} else {
		fmt.Printf("  %-14s %s need root - run with sudo\n", "udev rules", colorDim("[~]"))
	}

	// 2. config
	userCfg := UserConfigPath()
	if _, err := os.Stat(userCfg); err == nil {
		fmt.Printf("  %-14s %s %s (already exists)\n", "config", colorDim("[~]"), userCfg)
	} else {
		dir := filepath.Dir(userCfg)
		if err := os.MkdirAll(dir, 0755); err != nil { //nolint:gosec // G301: config dir
			fmt.Printf("  %-14s %s %v\n", "config", colorRed("[✗]"), err)
		} else {
			cfg := DefaultConfig()
			f, err := os.Create(userCfg) //nolint:gosec // G304: user config path
			if err != nil {
				fmt.Printf("  %-14s %s %v\n", "config", colorRed("[✗]"), err)
			} else {
				defer func() { _ = f.Close() }()
				if err := writeINI(f, cfg); err != nil {
					fmt.Printf("  %-14s %s %v\n", "config", colorRed("[✗]"), err)
				} else {
					fmt.Printf("  %-14s %s %s\n", "config", colorGreen("[✓]"), userCfg)
				}
			}
		}
	}

	// 3. systemd service
	// Root: /usr/lib/systemd/user/ (system-wide user unit, same as AUR PKGBUILD)
	// Non-root: ~/.config/systemd/user/ (user-local)
	if !hasSystemd() {
		fmt.Printf("  %-14s %s not detected\n", "systemd", colorDim("[~]"))
	} else {
		var svcPath string
		if os.Geteuid() == 0 {
			svcPath = "/usr/lib/systemd/user/gamepad-osk.service"
		} else {
			svcPath = systemdUserServicePath()
		}
		svcDir := filepath.Dir(svcPath)
		if err := os.MkdirAll(svcDir, 0755); err != nil { //nolint:gosec // G301: systemd user dir
			fmt.Printf("  %-14s %s %v\n", "systemd", colorRed("[✗]"), err)
		} else {
			exePath := "/usr/bin/gamepad-osk"
			if exe, err := os.Executable(); err == nil {
				exePath = exe
			}
			content := fmt.Sprintf(systemdServiceTemplate, exePath)
			existed := false
			if _, statErr := os.Stat(svcPath); statErr == nil {
				existed = true
			}
			if err := os.WriteFile(svcPath, []byte(content), 0644); err != nil { //nolint:gosec // G306: service file
				fmt.Printf("  %-14s %s %v\n", "systemd", colorRed("[✗]"), err)
			} else if existed {
				fmt.Printf("  %-14s %s %s (updated)\n", "systemd", colorGreen("[✓]"), svcPath)
			} else {
				fmt.Printf("  %-14s %s %s\n", "systemd", colorGreen("[✓]"), svcPath)
			}
		}
	}

	// 4. group membership hint
	if inGroup, _ := isUserInGroup("input"); !inGroup {
		// Check if access works via ACL
		if f, err := os.OpenFile("/dev/uinput", os.O_RDWR, 0); err != nil { //nolint:gosec // G304: constant path
			fmt.Println()
			fmt.Printf("  %s\n", colorYellow("Note: add yourself to the input group:"))
			fmt.Printf("    sudo usermod -aG input $USER\n")
		} else {
			_ = f.Close()
		}
	}

	fmt.Println()
	fmt.Println(colorGreen("  Setup complete."))
	return 0
}

func systemdUserServicePath() string {
	home := os.Getenv("HOME")
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := os.UserHomeDir(); err == nil {
			home = u
		}
	}
	return filepath.Join(home, ".config", "systemd", "user", "gamepad-osk.service")
}
