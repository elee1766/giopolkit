//go:build !(linux || freebsd || openbsd || netbsd)

package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

func elevate(req request) (*exec.Cmd, error) {
	return nil, fmt.Errorf("elevation not implemented on %s (use --dialog-only)", runtime.GOOS)
}
