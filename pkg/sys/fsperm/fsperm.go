// Package fsperm decides whether a user could change what a path refers to.
package fsperm

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

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
