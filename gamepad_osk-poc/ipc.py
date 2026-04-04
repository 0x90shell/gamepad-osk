"""Unix socket IPC for toggle and other commands."""

import os
import socket
import threading


SOCK_PATH = os.path.expanduser("~/.cache/gamepad-osk.sock")


class IPCServer:
    """Listens for commands on a Unix socket."""

    def __init__(self, on_command):
        """on_command(cmd: str) will be called from the listener thread."""
        self.on_command = on_command
        self.sock = None
        self.thread = None
        self.running = False

    def start(self):
        # Clean up stale socket
        if os.path.exists(SOCK_PATH):
            try:
                os.unlink(SOCK_PATH)
            except OSError:
                pass

        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.bind(SOCK_PATH)
        self.sock.listen(1)
        self.sock.settimeout(1.0)
        self.running = True

        self.thread = threading.Thread(target=self._listen, daemon=True)
        self.thread.start()

    def _listen(self):
        while self.running:
            try:
                conn, _ = self.sock.accept()
                data = conn.recv(256).decode("utf-8").strip()
                if data:
                    self.on_command(data)
                conn.close()
            except socket.timeout:
                continue
            except OSError:
                break

    def stop(self):
        self.running = False
        if self.sock:
            try:
                self.sock.close()
            except Exception:
                pass
        if os.path.exists(SOCK_PATH):
            try:
                os.unlink(SOCK_PATH)
            except OSError:
                pass


def send_command(cmd):
    """Send a command to the running instance. Returns True on success."""
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    try:
        sock.connect(SOCK_PATH)
        sock.sendall(cmd.encode("utf-8"))
        sock.close()
        return True
    except (ConnectionRefusedError, FileNotFoundError):
        return False
    finally:
        sock.close()
