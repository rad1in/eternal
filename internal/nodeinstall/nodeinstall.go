// Package nodeinstall holds the one-command node installer the master panel serves.
package nodeinstall

import (
	_ "embed"
	"strings"
)

//go:embed install-node.sh
var script string

const versionPlaceholder = "@@VERSION@@"

// Script returns the installer with version (a release number such as "1.4.0")
// baked in as the default release a new node installs.
func Script(version string) []byte {
	version = strings.TrimSpace(version)
	if version != "" && !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return []byte(strings.Replace(script, versionPlaceholder, version, 1))
}
