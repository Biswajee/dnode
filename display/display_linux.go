//go:build linux

package display

// enableANSI is a no-op on Linux; ANSI escape codes work in any standard terminal.
func enableANSI() {}
