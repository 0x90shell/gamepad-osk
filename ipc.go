package main

import (
	"net"
	"os"
	"path/filepath"
)

var sockPath = filepath.Join(os.Getenv("HOME"), ".cache", "gamepad-osk.sock")

type IPCServer struct {
	listener  net.Listener
	onCommand func(string)
	running   bool
}

func NewIPCServer(onCommand func(string)) *IPCServer {
	return &IPCServer{onCommand: onCommand}
}

func (s *IPCServer) Start() error {
	_ = os.Remove(sockPath)
	l, err := net.Listen("unix", sockPath) //nolint:noctx // Unix socket, no context needed
	if err != nil {
		return err
	}
	s.listener = l
	s.running = true
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
