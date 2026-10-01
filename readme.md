# rootpls

A way for agents to ask to run things as root.

`rootpls` shows a confirmation dialog with the agent's reason, a risk level, the exact command, and the working directory. If you approve, it hands off to polkit (`pkexec`), which asks for your password. The dialog is built with [Gio](https://gioui.org).

```bash
rootpls -r "Install ripgrep the user asked for" -l medium -- apt-get install -y ripgrep
rootpls -r "Read nginx logs" -l low -- 'journalctl -u nginx -n 50 --no-pager | tail'
```

- One argument after `--` runs via `/bin/sh -c`. Several arguments run directly (no shell).
- `-l low|medium|high` sets the badge color. `-t 120s` sets the auto-deny timeout.
- `--dialog-only` shows the dialog and exits 0 on approval without running anything.
- Exit codes: `0` ok, `126` denied, timed out, or auth failed, `2` usage error, anything else is the command's own exit code.
- Keys: `Esc` denies, `Ctrl+Enter` approves. Approval is disabled for the first 800ms so a stray keypress or click can't approve.
- On X11 the dialog is centered on the primary monitor and kept on top.

## Security model

Root access is protected by polkit authentication. The dialog does not protect anything.

1. `rootpls` runs as your user, the same user as the agent. Anything it can do, the agent can do directly. That includes calling `pkexec` itself or sending synthetic X11 key events to approve the dialog (`xdotool` can do this on X11). The dialog is there to show you what is about to run and why. It is not a security boundary.
2. The real boundary is `pkexec`. It is setuid root. It asks `polkitd` over the system D-Bus whether the caller is authorized for `org.freedesktop.policykit.exec`. On this system that action is `auth_admin` with no `_keep`, so polkitd makes the session's auth agent (lxpolkit) ask for your password every time, and nothing is cached. The agent never sees the password.
3. Hardening in rootpls:
   - `pkexec` is resolved from fixed absolute paths and must be setuid, so a `pkexec` earlier on a PATH the agent controls is not used.
   - `--disable-internal-agent` stops pkexec from falling back to a text password prompt on the agent's terminal.
   - pkexec clears the environment, so `LD_PRELOAD`, `PATH`, and similar variables from the agent do not reach the root process. The command runs as `/usr/bin/env -C <cwd> /bin/sh -c ...`.
   - stdin is `/dev/null`. Any failure (window can't open, timeout, Esc, close) counts as a denial.
4. Limits:
   - Read the command in the dialog. A command you approve can do anything, including installing a persistent backdoor.
   - The polkit prompt shows `/usr/bin/env`, not the full command. The rootpls dialog is where you check the command.
   - Wayland stops other clients from injecting input. X11 does not. Under X11, treat the dialog as informational only.

## Build

Needs Go and the Gio Linux dependencies (`libwayland-dev libx11-dev libxkbcommon-x11-dev libgles2-mesa-dev libegl1-mesa-dev libxcursor-dev libxfixes-dev libxrandr-dev libvulkan-dev`).

```bash
go build -trimpath -ldflags='-s -w' -o ~/.local/bin/rootpls .
```

Elevation is implemented for Linux and the BSDs. Other platforms support `--dialog-only` only.
