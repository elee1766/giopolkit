# giopolkit

A polkit authentication agent with a readable approval window, built with [Gio](https://gioui.org).
It replaces lxpolkit (or any other agent) in your session.

When something asks polkit for authorization (usually `pkexec`), giopolkit shows:

- **run as root** (or the target user).
- The **exact argv** pkexec will run, quoted so argument boundaries are visible, plus the working directory.
- **Warnings** (`!!` lines): the program or an argument can be modified by the requesting user, a
  relative program name, a shell or interpreter, or a script file whose contents aren't shown
  (`sh fix.sh`, `python3 x.py`, an executable script). The argument a warning is about is
  highlighted in the command. A flagged request gets a red border and the rule name in the header.
- **from**: the process chain that asked.

You review it first. Deny is the filled button and Esc denies. Approve is disabled for 1s after the
window appears, or 3s if anything was flagged.
After approval, the same window asks for your password (or shows "touch your key" for `pam_u2f`),
using polkit's own setuid helper. Other polkit actions (NetworkManager, udisks, and so on) show the
action, message, and details.

## How it works

```
pkexec ──D-Bus──▶ polkitd ──BeginAuthentication(details, cookie)──▶ giopolkit (your session)
                                                                     │ show request, wait for Approve
                                                                     │ run polkit-agent-helper-1 (setuid, PAM)
                     ◀── helper reports success to polkitd ──────────┘
pkexec runs the command
```

polkitd sends agents only the action, message, and `polkit.subject-pid` / `polkit.caller-pid`.
For pkexec, the caller is the pkexec process itself. giopolkit reads its argv from
`/proc/PID/cmdline`, and only trusts it if the process is named `pkexec` and runs with euid 0.
A same-user process can't ptrace or modify a setuid process, so that argv is what pkexec runs.

## Install

```bash
go build -o ~/.local/bin/giopolkit ./cmd/giopolkit
```

Start it in your session instead of lxpolkit. Only one agent can register per session.

- nitro: `dist/nitro/giopolkit/run` (copy to `~/.config/nitro/giopolkit/`, remove the lxpolkit service, `nitroctl rescan`).
- Other setups: run `giopolkit` from your session startup, after removing `lxpolkit` from autostart.

`giopolkit -session ID` overrides the logind session (default `XDG_SESSION_ID` or detected).

## Colors

Colors come from `$XDG_CONFIG_HOME/giopolkit/config.yaml` (`-config` to change). See
[`dist/config.yaml`](dist/config.yaml) for every option.

```yaml
xresources: true      # take colors from xrdb: background, foreground, color0/1/2/8
colors:
  red: "#ff5555"      # then override single colors
```

With `xresources: true`, `giopolkit.red` (and the other names) are used if set, then the
terminal colors. `xresources_file: ~/.Xresources` reads a file instead of running `xrdb`.
Config errors are printed and the remaining colors still apply.

## Security notes

- The real check is unchanged: `polkit-agent-helper-1` runs PAM and tells polkitd the result. giopolkit
  only relays prompts. Passwords go to the helper on a pipe and are not kept.
- giopolkit only accepts `BeginAuthentication` and `CancelAuthentication` from the owner of
  `org.freedesktop.PolicyKit1`, so other processes can't make it show fake requests.
- Text is escaped before display: control characters, bidi overrides, zero-width and non-ASCII
  characters show as `\u{...}`.
- What it can't do: if the agent requesting root runs as your own user, it can also send synthetic
  input to your X11 session or read your keystrokes. The password (or a `pam_u2f` key touch) is what
  stops it, not the window. Wayland, `kernel.yama.ptrace_scope=2`, and `pam_u2f` close most of that gap.
- A command you approve can do anything root can. Read it.
