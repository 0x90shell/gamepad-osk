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

// Packages (AUR) own the /usr/lib copies. --setup --install as root writes
// the /etc copies, the place for files an admin adds by hand: pacman never
// conflicts with them. A file in /etc overrides the /usr/lib one.
const (
	udevRulePkg    = "/usr/lib/udev/rules.d/80-gamepad-osk.rules"
	udevRuleEtc    = "/etc/udev/rules.d/80-gamepad-osk.rules"
	serviceUnitPkg = "/usr/lib/systemd/user/gamepad-osk.service"
	serviceUnitEtc = "/etc/systemd/user/gamepad-osk.service"
)

// firstExisting returns the first path that exists, or "".
func firstExisting(paths ...string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// writeSetupFile writes content to path and prints the result line.
func writeSetupFile(label, path, content string) bool {
	existed := firstExisting(path) != ""
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil { //nolint:gosec // G301: system or user config dir
		fmt.Printf("  %-14s %s %v\n", label, colorRed("[✗]"), err)
		return false
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil { //nolint:gosec // G306: udev rules and units are world-readable
		fmt.Printf("  %-14s %s %v\n", label, colorRed("[✗]"), err)
		return false
	}
	note := ""
	if existed {
		note = " (updated)"
	}
	fmt.Printf("  %-14s %s %s%s\n", label, colorGreen("[✓]"), path, note)
	return true
}

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
// runSetup checks or installs. configPath is the --config file, or empty.
func runSetup(install bool, configPath string) int {
	if install {
		return runInstall(configPath)
	}
	return runDiagnose(configPath)
}

func runDiagnose(configPath string) int {
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
	for _, path := range []string{udevRuleEtc, udevRulePkg} {
		if _, err := os.Stat(path); err == nil {
			fmt.Printf("  %-14s %s %s\n", "udev rules", colorGreen("[✓]"), path)
			udevFound = true
		}
	}
	if firstExisting(udevRuleEtc) != "" && firstExisting(udevRulePkg) != "" {
		fmt.Printf("  %-14s   %s\n", "", colorDim("the /etc copy overrides the package's; remove it unless you edited it"))
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

	// 5. config: the file the app would read (--config, else first found)
	// Suppress log output from config parsing; the problems are listed below.
	log.SetOutput(io.Discard)
	cfg := ReadConfig(configPath)
	log.SetOutput(os.Stderr)
	if cfgPath := configSource.Path; cfgPath != "" {
		problems := checkConfigFile(cfgPath)
		problems = append(problems, checkConfig(cfg)...)
		if len(problems) > 0 {
			fmt.Printf("  %-14s %s %s\n", "config", colorYellow("[!]"), cfgPath)
			for _, issue := range problems {
				fmt.Printf("  %-14s   %s\n", "", colorYellow(issue))
			}
			fmt.Printf("  %-14s   %s\n", "", colorDim("Edit "+cfgPath+" to fix"))
			configIssues++
			issues++
		} else {
			fmt.Printf("  %-14s %s %s\n", "config", colorGreen("[✓]"), cfgPath)
		}
	} else {
		fmt.Printf("  %-14s %s %s (will be created on first run)\n", "config", colorDim("[~]"), UserConfigPath())
	}

	// 6. systemd service - check both system-wide and user-local paths
	if hasSystemd() {
		if unit := firstExisting(systemdUserServicePath(), serviceUnitEtc, serviceUnitPkg); unit != "" {
			fmt.Printf("  %-14s %s %s\n", "systemd", colorGreen("[✓]"), unit)
			if unit != serviceUnitPkg && firstExisting(serviceUnitPkg) != "" {
				fmt.Printf("  %-14s   %s\n", "", colorDim("this unit overrides the package's; remove it unless you edited it"))
			}
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

func runInstall(configPath string) int {
	fmt.Printf("gamepad-osk v%s setup:\n", version)
	fmt.Println()

	// 1. udev rules: the package's copy wins; else ours goes in /etc
	switch {
	case firstExisting(udevRulePkg) != "":
		fmt.Printf("  %-14s %s %s (installed by your package)\n", "udev rules", colorGreen("[✓]"), udevRulePkg)
	case canWritePath(udevRuleEtc):
		if writeSetupFile("udev rules", udevRuleEtc, udevRuleContent) {
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
		}
	case firstExisting(udevRuleEtc) != "":
		fmt.Printf("  %-14s %s %s (installed; run with sudo to update)\n", "udev rules", colorGreen("[✓]"), udevRuleEtc)
	default:
		fmt.Printf("  %-14s %s skipped: needs root (system file)\n", "udev rules", colorDim("[~]"))
		fmt.Printf("  %-14s   %s\n", "", colorDim("Not needed if your package installed it. Otherwise run:"))
		fmt.Printf("  %-14s   %s\n", "", colorDim("sudo gamepad-osk --setup --install"))
	}

	// 2. config (not with --config: that file is the only one used, and not
	// as root: the user's own first run creates it, owned by the user)
	userCfg := UserConfigPath()
	if geteuid() == 0 {
		fmt.Printf("  %-14s %s skipped as root: created by your user on first run\n", "config", colorDim("[~]"))
	} else if configPath != "" {
		fmt.Printf("  %-14s %s %s (--config, user config not created)\n", "config", colorDim("[~]"), configPath)
	} else if _, err := os.Stat(userCfg); err == nil {
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

	// 3. systemd service: the package's unit wins. Root writes /etc; a
	// user writes ~/.config only if no system-wide unit exists, since a
	// user unit would shadow it.
	exePath := "/usr/bin/gamepad-osk"
	if exe, err := os.Executable(); err == nil {
		exePath = exe
	}
	unit := fmt.Sprintf(systemdServiceTemplate, exePath)
	switch system := firstExisting(serviceUnitPkg, serviceUnitEtc); {
	case !hasSystemd():
		fmt.Printf("  %-14s %s not detected\n", "systemd", colorDim("[~]"))
	case system == serviceUnitPkg:
		fmt.Printf("  %-14s %s %s (installed by your package)\n", "systemd", colorGreen("[✓]"), serviceUnitPkg)
	case geteuid() == 0:
		writeSetupFile("systemd", serviceUnitEtc, unit)
	case system != "":
		fmt.Printf("  %-14s %s %s (installed system-wide)\n", "systemd", colorGreen("[✓]"), system)
	default:
		writeSetupFile("systemd", systemdUserServicePath(), unit)
	}

	// 4. group membership hint. Under sudo, device access says nothing
	// about the user's own access, so only the group counts.
	if inGroup, _ := isUserInGroup("input"); !inGroup {
		f, err := os.OpenFile("/dev/uinput", os.O_RDWR, 0) //nolint:gosec // G304: constant path
		if err == nil {
			_ = f.Close()
		}
		if err != nil || geteuid() == 0 { // works for this user via ACL otherwise
			name := realUser()
			if name == "" {
				name = "$USER"
			}
			fmt.Println()
			fmt.Printf("  %s\n", colorYellow("Note: add "+name+" to the input group:"))
			fmt.Printf("    sudo usermod -aG input %s\n", name)
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
