//go:build linux || freebsd || openbsd || netbsd

package main

import (
	"fmt"
	"os"
	"os/exec"
)

// pkexec is resolved by absolute path so a PATH entry the agent controls can't shadow it.
var pkexecPaths = []string{"/usr/bin/pkexec", "/bin/pkexec", "/usr/local/bin/pkexec"}

// elevate builds the polkit-authenticated command. pkexec itself is setuid and talks to
// polkitd over the system bus. polkitd asks the session's auth agent (lxpolkit) for the password.
// pkexec clears the environment and runs with cwd "/", so the working dir is passed via env -C.
func elevate(req request) (*exec.Cmd, error) {
	pk := ""
	for _, p := range pkexecPaths {
		if st, err := os.Stat(p); err == nil && st.Mode()&os.ModeSetuid != 0 {
			pk = p
			break
		}
	}
	if pk == "" {
		return nil, fmt.Errorf("setuid pkexec not found in %v", pkexecPaths)
	}
	args := []string{"--disable-internal-agent", "/usr/bin/env", "-C", req.Dir}
	if req.Shell != "" {
		args = append(args, "/bin/sh", "-c", req.Shell)
	} else {
		args = append(args, req.Argv...)
	}
	return exec.Command(pk, args...), nil
}
