// Package inspect turns a polkit request into facts worth showing: who asked, what will run, and
// whether the requester could change it after approval.
package inspect

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const ExecAction = "org.freedesktop.policykit.exec"

type Proc struct {
	PID int
	UID int
	Exe string
	Cmd string
}

type Report struct {
	Program    string   // program pkexec will run, if this is a pkexec request
	Argv       []string // exact argv pkexec will run
	TargetUser string   // pkexec --user, default root
	Cwd        string   // working directory of the command, best effort
	Trusted    bool     // Argv was read from a setuid pkexec process, which the caller can't alter
	Chain      []Proc   // the requester and its parents, pkexec excluded
	Warnings   []string // things the approver should know
}

// Build inspects a request. polkitd forwards only polkit.subject-pid and polkit.caller-pid to
// agents, not pkexec's program and command_line details. pkexec makes its parent the subject and
// calls polkitd itself, so caller-pid is the pkexec process. Its exact argv is read from
// /proc/PID/cmdline. pkexec runs setuid root, so a same-user caller can't ptrace it or rewrite its
// memory: that argv is what pkexec will run.
func Build(actionID string, details map[string]string) Report {
	r := Report{}
	subject, _ := strconv.Atoi(details["polkit.subject-pid"])
	caller, _ := strconv.Atoi(details["polkit.caller-pid"])
	if subject <= 0 && caller <= 0 {
		return r
	}
	isPkexec := actionID == ExecAction && caller > 0 && readPkexec(&r, caller)
	start := subject
	if start <= 0 {
		start = caller
	}
	r.Chain = chain(start, 6)
	if len(r.Chain) == 0 {
		return r
	}
	req := r.Chain[0]
	if !isPkexec {
		return r
	}
	if r.Cwd == "" {
		if cwd, err := os.Readlink("/proc/" + strconv.Itoa(req.PID) + "/cwd"); err == nil {
			r.Cwd = cwd
		}
	}
	if r.Program == "" {
		return r
	}
	groups := groupsOf(req.PID)
	if mod, via := CanModify(r.Program, req.UID, groups); mod {
		r.Warnings = append(r.Warnings, fmt.Sprintf("%s can be modified by the requesting user (via %s): it could be replaced after you approve", r.Program, via))
	}
	for i, a := range r.Argv {
		if i == 0 {
			continue
		}
		for _, c := range pathCandidates(a, r.Cwd) {
			if mod, via := CanModify(c, req.UID, groups); mod {
				r.Warnings = append(r.Warnings, fmt.Sprintf("argument %d (%s) can be modified by the requesting user (via %s)", i, c, via))
				break
			}
		}
	}
	if isShellOrInterp(filepath.Base(r.Program)) {
		r.Warnings = append(r.Warnings, filepath.Base(r.Program)+" runs code from its arguments or from files: read them carefully")
	}
	return r
}

// readPkexec fills Argv, Program, TargetUser, and Cwd from a pkexec process. It returns false if
// pid is not a setuid pkexec, since only then can the argv be trusted.
func readPkexec(r *Report, pid int) bool {
	dir := "/proc/" + strconv.Itoa(pid)
	comm, _ := os.ReadFile(dir + "/comm")
	if strings.TrimSpace(string(comm)) != "pkexec" || euid(pid) != 0 {
		r.Warnings = append(r.Warnings, "the requesting process is not a setuid pkexec: the command can't be verified")
		return false
	}
	b, err := os.ReadFile(dir + "/cmdline")
	if err != nil || len(b) == 0 {
		return false
	}
	args := strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00")
	user, keepCwd := "root", false
	i := 1
loop:
	for ; i < len(args); i++ {
		switch args[i] {
		case "--user", "-u":
			i++
			if i < len(args) {
				user = args[i]
			}
		case "--disable-internal-agent":
		case "--keep-cwd":
			keepCwd = true
		default:
			break loop
		}
	}
	r.TargetUser = user
	r.Trusted = true
	if i >= len(args) {
		r.Argv = []string{"(login shell of " + user + ")"}
		return true
	}
	r.Argv = args[i:]
	r.Program = args[i]
	if !strings.HasPrefix(r.Program, "/") {
		r.Warnings = append(r.Warnings, "relative program name: pkexec looks it up in the caller's PATH, which the caller controls")
	}
	if !keepCwd {
		r.Cwd = "(home of " + user + ")"
	}
	return true
}

// pathCandidates returns the absolute paths an argument may refer to: itself, or the value after
// "=" in --opt=value. Relative names are resolved against cwd when they exist.
func pathCandidates(a, cwd string) []string {
	vals := []string{a}
	if k := strings.IndexByte(a, '='); k >= 0 {
		vals = append(vals, a[k+1:])
	}
	var out []string
	for _, v := range vals {
		switch {
		case strings.HasPrefix(v, "/"):
			out = append(out, v)
		case v != "" && strings.HasPrefix(cwd, "/") && !strings.HasPrefix(v, "-"):
			p := filepath.Join(cwd, v)
			if _, err := os.Lstat(p); err == nil {
				out = append(out, p)
			}
		}
	}
	return out
}

func isShellOrInterp(b string) bool {
	switch b {
	case "sh", "bash", "dash", "zsh", "fish", "env", "perl", "ruby", "node", "deno", "bun", "lua", "php", "xargs", "find", "busybox", "nsenter", "systemd-run", "su", "sudo", "doas", "pkexec", "make", "awk", "gawk", "tclsh":
		return true
	}
	return strings.HasPrefix(b, "python")
}

func chain(pid, max int) []Proc {
	var out []Proc
	for i := 0; i < max && pid > 1; i++ {
		p, ok := readProc(pid)
		if !ok {
			break
		}
		out = append(out, p)
		_, pid = ppidOf(pid)
	}
	return out
}

func readProc(pid int) (Proc, bool) {
	dir := "/proc/" + strconv.Itoa(pid)
	p := Proc{PID: pid, UID: -1}
	b, err := os.ReadFile(dir + "/status")
	if err != nil {
		return p, false
	}
	for _, l := range strings.Split(string(b), "\n") {
		if f, ok := strings.CutPrefix(l, "Uid:"); ok {
			if fs := strings.Fields(f); len(fs) > 0 {
				p.UID, _ = strconv.Atoi(fs[0])
			}
		}
	}
	p.Exe, _ = os.Readlink(dir + "/exe")
	if b, err := os.ReadFile(dir + "/cmdline"); err == nil {
		p.Cmd = strings.Join(strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00"), " ")
	}
	return p, true
}

func ppidOf(pid int) (bool, int) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false, 0
	}
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return false, 0
	}
	f := strings.Fields(s[i+1:])
	if len(f) < 2 {
		return false, 0
	}
	ppid, _ := strconv.Atoi(f[1])
	return true, ppid
}

func euid(pid int) int {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return -1
	}
	for _, l := range strings.Split(string(b), "\n") {
		if f, ok := strings.CutPrefix(l, "Uid:"); ok {
			if fs := strings.Fields(f); len(fs) > 1 {
				v, _ := strconv.Atoi(fs[1])
				return v
			}
		}
	}
	return -1
}

func groupsOf(pid int) map[uint32]bool {
	g := map[uint32]bool{}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return g
	}
	for _, l := range strings.Split(string(b), "\n") {
		var f string
		var ok bool
		if f, ok = strings.CutPrefix(l, "Groups:"); !ok {
			if f, ok = strings.CutPrefix(l, "Gid:"); ok {
				if fs := strings.Fields(f); len(fs) > 0 {
					f = fs[0]
				}
			}
		}
		if !ok {
			continue
		}
		for _, x := range strings.Fields(f) {
			if v, err := strconv.ParseUint(x, 10, 32); err == nil {
				g[uint32(v)] = true
			}
		}
	}
	return g
}

// CanModify reports whether uid (with groups) could change what path refers to: by writing the
// file, or by replacing it, or any directory or symlink on the way to it. The path is walked one
// component at a time, following symlinks as the kernel would.
func CanModify(path string, uid int, groups map[uint32]bool) (bool, string) {
	if uid == 0 || !filepath.IsAbs(path) {
		return false, ""
	}
	return walk(filepath.Clean(path), uid, groups, 0)
}

func walk(path string, uid int, groups map[uint32]bool, depth int) (bool, string) {
	if depth > 16 {
		return true, path + " (symlink loop)"
	}
	if path == "/" {
		return false, ""
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	cur := "/"
	for i, part := range parts {
		parent, next := cur, filepath.Join(cur, part)
		var pst, st unix.Stat_t
		if unix.Lstat(parent, &pst) != nil || unix.Lstat(next, &st) != nil {
			return false, ""
		}
		if writable(parent, &pst, uid, groups) {
			sticky := pst.Mode&unix.S_ISVTX != 0
			if !sticky || int(st.Uid) == uid || int(pst.Uid) == uid {
				return true, parent
			}
		}
		if st.Mode&unix.S_IFMT == unix.S_IFLNK {
			target, err := os.Readlink(next)
			if err != nil {
				return false, ""
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(parent, target)
			}
			rest := filepath.Join(append([]string{target}, parts[i+1:]...)...)
			return walk(filepath.Clean(rest), uid, groups, depth+1)
		}
		if i == len(parts)-1 && writable(next, &st, uid, groups) {
			return true, next
		}
		cur = next
	}
	return false, ""
}

func writable(path string, st *unix.Stat_t, uid int, groups map[uint32]bool) bool {
	if int(st.Uid) == uid {
		return true
	}
	mode := st.Mode & 0o777
	if mode&0o002 != 0 {
		return true
	}
	if mode&0o020 != 0 {
		if groups[st.Gid] {
			return true
		}
		if n, _ := unix.Lgetxattr(path, "system.posix_acl_access", nil); n > 0 {
			return true
		}
	}
	return false
}
