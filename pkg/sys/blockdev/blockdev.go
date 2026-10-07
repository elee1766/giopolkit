// Package blockdev finds block devices and where they are mounted.
package blockdev

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Lookup reports whether path resolves to a block device, and its device number.
func Lookup(path string) (uint64, bool) {
	var st unix.Stat_t
	if unix.Stat(path, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFBLK {
		return 0, false
	}
	return uint64(st.Rdev), true
}

// Mounts returns the mount points of a block device and any of its partitions, shortest first.
func Mounts(dev uint64) []string {
	devs := map[string]bool{devString(dev): true}
	sys := fmt.Sprintf("/sys/dev/block/%s", devString(dev))
	if ents, err := os.ReadDir(sys); err == nil {
		for _, e := range ents {
			if b, err := os.ReadFile(filepath.Join(sys, e.Name(), "dev")); err == nil {
				if _, err := os.Stat(filepath.Join(sys, e.Name(), "partition")); err == nil {
					devs[strings.TrimSpace(string(b))] = true
				}
			}
		}
	}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) < 5 || !devs[fs[2]] {
			continue
		}
		mp := unescapeMount(fs[4])
		if !seen[mp] {
			seen[mp] = true
			out = append(out, mp)
		}
	}
	slices.SortStableFunc(out, func(a, b string) int { return len(a) - len(b) })
	return out
}

func devString(dev uint64) string {
	return fmt.Sprintf("%d:%d", unix.Major(dev), unix.Minor(dev))
}

// unescapeMount decodes the \ooo octal escapes mountinfo uses for spaces and similar.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
