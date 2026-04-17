package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"syscall"
)

func initSockPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "gamepad-osk.sock")
	}
	// Under sudo, XDG_RUNTIME_DIR is unset but the real user's dir exists
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil {
			dir := "/run/user/" + u.Uid
			if info, err := os.Stat(dir); err == nil && info.IsDir() { //nolint:gosec // G703: trusted /run/user path from user.Lookup
				return filepath.Join(dir, "gamepad-osk.sock")
			}
		}
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("gamepad-osk-%d.sock", os.Getuid()))
}

var sockPath = initSockPath()

type IPCServer struct {
	listener  net.Listener
	onCommand func(string)
	running   bool
}

func NewIPCServer(onCommand func(string)) *IPCServer {
	return &IPCServer{onCommand: onCommand}
}

func (s *IPCServer) Start() error {
	Debugf("IPC socket: %s (XDG_RUNTIME_DIR=%s)", sockPath, os.Getenv("XDG_RUNTIME_DIR"))
	l, err := net.Listen("unix", sockPath) //nolint:noctx // Unix socket, no context needed
	if err != nil {
		if isAddrInUse(err) {
			if IPCSend("ping") {
				// Live instance owns this socket
				return fmt.Errorf("another instance is listening on %s", sockPath)
			}
			// Can't connect - could be stale (crash) or permission mismatch (sudo).
			// Only remove if we own the file or are root.
			if canRemoveSocket(sockPath) {
				Debugf("Removed stale socket %s", sockPath)
				_ = os.Remove(sockPath)
				l, err = net.Listen("unix", sockPath) //nolint:noctx // Unix socket, no context needed
			} else {
				return fmt.Errorf("socket %s exists but is not connectable (owned by another user?)", sockPath)
			}
		}
		if err != nil {
			return err
		}
	}
	s.listener = l
	s.running = true
	log.Printf("IPC: listening on %s", sockPath)
	go s.listen()
	return nil
}

func (s *IPCServer) listen() {
	for s.running {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.running {
				continue
			}
			return
		}
		buf := make([]byte, 256)
		n, _ := conn.Read(buf)
		if n > 0 {
			cmd := string(buf[:n])
			s.onCommand(cmd)
		}
		_ = conn.Close()
	}
}

func (s *IPCServer) Stop() {
	s.running = false
	if s.listener != nil {
		_ = s.listener.Close()
	}
	_ = os.Remove(sockPath)
}

func IPCSend(cmd string) bool {
	conn, err := net.Dial("unix", sockPath) //nolint:noctx // Unix socket IPC, no context needed
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	_, _ = conn.Write([]byte(cmd))
	return true
}

// socketOwnedByOther returns true if the socket file exists and is owned by
// a different user. Root is excluded (root can always connect).
func socketOwnedByOther(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	uid := os.Getuid()
	if uid == 0 {
		return false // root can connect to anything
	}
	return uint32(uid) != stat.Uid //nolint:gosec // G115: uid fits in uint32
}

// canRemoveSocket returns true if the current process can safely remove the socket.
// Only remove if we own the file or are root - don't nuke another user's live socket
// just because we can't connect (permission mismatch from sudo).
func canRemoveSocket(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	uid := os.Getuid()
	return uid == 0 || uint32(uid) == stat.Uid //nolint:gosec // G115: uid fits in uint32
}

func isAddrInUse(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			return errors.Is(sysErr.Err, syscall.EADDRINUSE)
		}
	}
	return false
}
