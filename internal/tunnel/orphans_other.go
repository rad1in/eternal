//go:build !linux

package tunnel

// killStrayWattun is a no-op off Linux: Windows ties the child to the panel with a job object.
func killStrayWattun(_ string) int { return 0 }
