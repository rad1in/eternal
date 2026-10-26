package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
)

// BinaryName is the wattun file name for this OS and arch, matching the xray and mtg naming
// in the release's bin folder.
func BinaryName() string {
	name := fmt.Sprintf("wattun-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// BinaryPath is where the release puts wattun, next to the xray binary.
func BinaryPath() string { return filepath.Join(config.GetBinFolderPath(), BinaryName()) }

// BinaryAvailable reports whether the wattun binary is installed.
func BinaryAvailable() bool {
	st, err := os.Stat(BinaryPath())
	return err == nil && !st.IsDir()
}

// StateDir holds the generated configs, keys and counters, beside the panel database.
func StateDir() string { return filepath.Join(config.GetDBFolderPath(), "wattun") }

// ProbeLinkWait overrides how long a carrier probe waits for each link; zero keeps the default.
var ProbeLinkWait time.Duration
