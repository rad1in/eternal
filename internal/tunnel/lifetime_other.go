//go:build !windows

package tunnel

import "os/exec"

func attachChildLifetime(_ *exec.Cmd) {}
