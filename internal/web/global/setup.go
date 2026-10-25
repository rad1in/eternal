package global

import "sync/atomic"

var setupPending atomic.Bool

// SetupPending reports that the first-run wizard has not been completed, so the
// panel serves nothing but the wizard.
func SetupPending() bool { return setupPending.Load() }

func SetSetupPending(pending bool) { setupPending.Store(pending) }
