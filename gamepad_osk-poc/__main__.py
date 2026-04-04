"""Entry point for gamepad-osk."""

import sys

from .ipc import send_command


def main():
    args = sys.argv[1:]

    # Handle --toggle command
    if "--toggle" in args:
        if send_command("toggle"):
            sys.exit(0)
        else:
            # No running instance — start one
            print("No running instance, starting new...")
            args.remove("--toggle")

    # Device path is the first non-flag argument
    device_path = None
    for arg in args:
        if not arg.startswith("-"):
            device_path = arg
            break

    from .config import load_config
    from .app import Application

    config = load_config()
    app = Application(config, device_path)
    try:
        app.run()
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
