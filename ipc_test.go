package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// useTempSocket points the IPC socket at a temp dir for one test.
func useTempSocket(t *testing.T) {
	t.Helper()
	old := sockPath
	sockPath = filepath.Join(t.TempDir(), "gamepad-osk.sock")
	t.Cleanup(func() { sockPath = old })
}

func startTestServer(t *testing.T) (*IPCServer, chan string) {
	t.Helper()
	got := make(chan string, 4)
	s := NewIPCServer(func(cmd string) { got <- cmd })
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(s.Stop)
	return s, got
}

func waitCommand(t *testing.T, got chan string, want string, within time.Duration) {
	t.Helper()
	select {
	case cmd := <-got:
		if cmd != want {
			t.Errorf("command %q, want %q", cmd, want)
		}
	case <-time.After(within):
		t.Errorf("no %q within %v", want, within)
	}
}

func TestIPCRoundTrip(t *testing.T) {
	useTempSocket(t)
	_, got := startTestServer(t)
	if !IPCSend("toggle") {
		t.Fatal("IPCSend failed with a server running")
	}
	waitCommand(t, got, "toggle", time.Second)
}

// A second instance that loses the start race must fail with
// errInstanceRunning, and its Stop must leave the first one's socket.
func TestIPCSecondInstance(t *testing.T) {
	useTempSocket(t)
	_, got := startTestServer(t)
	loser := NewIPCServer(func(string) {})
	if err := loser.Start(); !errors.Is(err, errInstanceRunning) {
		t.Fatalf("second Start: %v, want errInstanceRunning", err)
	}
	loser.Stop()
	if _, err := os.Stat(sockPath); err != nil {
		t.Fatalf("the loser's Stop removed the socket: %v", err)
	}
	waitCommand(t, got, "ping", time.Second) // the loser's live-instance check
	if !IPCSend("toggle") {
		t.Fatal("IPCSend failed after the loser stopped")
	}
	waitCommand(t, got, "toggle", time.Second)
}

// A client that connects and sends nothing delays later commands by at
// most the read deadline (1 s), instead of blocking them.
func TestIPCSilentClient(t *testing.T) {
	useTempSocket(t)
	_, got := startTestServer(t)
	silent, err := net.Dial("unix", sockPath) //nolint:noctx // Unix socket, as in IPCSend
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = silent.Close() }()
	time.Sleep(50 * time.Millisecond) // the server is now reading from it
	if !IPCSend("toggle") {
		t.Fatal("IPCSend failed")
	}
	waitCommand(t, got, "toggle", 2*time.Second)
}

func TestIPCStopRemovesOwnSocket(t *testing.T) {
	useTempSocket(t)
	s, _ := startTestServer(t)
	s.Stop()
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Errorf("socket still there after Stop: %v", err)
	}
	if IPCSend("ping") {
		t.Error("IPCSend succeeded with no server")
	}
}
