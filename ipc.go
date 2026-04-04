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
	os.Remove(sockPath)
	l, err := net.Listen("unix", sockPath)
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
		conn.Close()
	}
}

func (s *IPCServer) Stop() {
	s.running = false
	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(sockPath)
}

func IPCSend(cmd string) bool {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return false
	}
	defer conn.Close()
	conn.Write([]byte(cmd))
	return true
}
