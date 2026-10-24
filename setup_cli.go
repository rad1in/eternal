package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
)

// Exit codes of "eternal setup-init" that install.sh tells apart.
const (
	setupInitOK         = 0
	setupInitFailed     = 1
	setupInitConfigured = 3
)

// runSetupInit opens the first-run wizard and prints its token and port as
// key=value lines. It rotates the token when the wizard is already open and
// refuses a panel whose default login has been replaced.
func runSetupInit(out io.Writer, port int) int {
	if err := database.InitDB(config.GetDBPath()); err != nil {
		fmt.Fprintln(out, "setup_error=database:", err)
		return setupInitFailed
	}
	token, chosen, err := (&panel.SetupService{}).Init(port)
	switch {
	case errors.Is(err, panel.ErrSetupConfigured):
		fmt.Fprintln(out, "setup_state=configured")
		return setupInitConfigured
	case err != nil:
		fmt.Fprintln(out, "setup_error=", err)
		return setupInitFailed
	}
	fmt.Fprintln(out, "setup_state=pending")
	fmt.Fprintf(out, "setup_port=%d\n", chosen)
	fmt.Fprintf(out, "setup_token=%s\n", token)
	return setupInitOK
}
