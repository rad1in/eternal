//go:build linux

package tunnel

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// killStrayWattun kills wattun processes a previous panel run left behind. The panel owns
// every process of this binary name, and a survivor would keep a tunnel port bound with a
// stale config.
func killStrayWattun(binaryPath string) int {
	base := filepath.Base(binaryPath)
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	self, killed := os.Getpid(), 0
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		exe, _ := os.Readlink("/proc/" + e.Name() + "/exe")
		cmdline, _ := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		argv0 := string(cmdline)
		for i := range cmdline {
			if cmdline[i] == 0 {
				argv0 = string(cmdline[:i])
				break
			}
		}
		if filepath.Base(exe) != base && filepath.Base(argv0) != base {
			continue
		}
		if syscall.Kill(pid, syscall.SIGKILL) == nil {
			killed++
		}
	}
	return killed
}
