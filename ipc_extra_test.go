package main

import (
	"net"
	"os"
	"testing"
	"time"
)

// staleSocket leaves a socket file with no listener behind, as a killed
// process does: closing a Go listener normally unlinks the file, so turn that off.
func staleSocket(t *testing.T) {
	t.Helper()
	l, err := net.Listen("unix", sockPath) //nolint:noctx // Unix socket, no context needed
	if err != nil {
		t.Fatal(err)
	}
	ul, ok := l.(*net.UnixListener)
	if !ok {
		t.Fatalf("listener is %T, not a unix listener", l)
	}
	ul.SetUnlinkOnClose(false)
	_ = ul.Close()
	if _, err := os.Stat(sockPath); err != nil {
		t.Fatalf("socket file not left behind: %v", err)
	}
	if IPCSend("ping") {
		t.Fatal("a stale socket accepted a connection")
	}
}

// U42: after a crash the socket file is still there. Start removes it, listens,
// and commands work. Without this every restart after kill -9 would run with no IPC.
func TestIPCStartReplacesStaleSocket(t *testing.T) {
	useTempSocket(t)
	staleSocket(t)
	_, got := startTestServer(t) // fails the test if Start returns an error
	if !IPCSend("toggle") {
		t.Fatal("no server answering after Start replaced the stale socket")
	}
	waitCommand(t, got, "toggle", time.Second)
}

// U43: canRemoveSocket is true for our own file, false for a missing path and
// for a file another user owns. /etc/passwd is root-owned on any Linux host;
// skip when the tests run as root (root can remove anything).
func TestCanRemoveSocket(t *testing.T) {
	useTempSocket(t)
	if canRemoveSocket(sockPath) {
		t.Error("canRemoveSocket is true for a path that does not exist")
	}
	staleSocket(t)
	if !canRemoveSocket(sockPath) {
		t.Error("canRemoveSocket is false for a socket file we own")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root: every file can be removed")
	}
	if canRemoveSocket("/etc/passwd") {
		t.Error("canRemoveSocket is true for a root-owned file")
	}
}

// U44: socketOwnedByOther is the mirror: false for our file or a missing path,
// true for another user's file (main.go uses it to refuse a start under sudo).
func TestSocketOwnedByOther(t *testing.T) {
	useTempSocket(t)
	if socketOwnedByOther(sockPath) {
		t.Error("true for a path that does not exist")
	}
	staleSocket(t)
	if socketOwnedByOther(sockPath) {
		t.Error("true for a socket we own")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root: socketOwnedByOther is always false")
	}
	if !socketOwnedByOther("/etc/passwd") {
		t.Error("false for a root-owned file")
	}
}
