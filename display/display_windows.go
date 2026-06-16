//go:build windows

package display

import (
	"os"

	"golang.org/x/sys/windows"
)

const enableVirtualTerminalProcessing = 0x0004

// enableANSI turns on VT100/ANSI escape code processing on the Windows
// console output handle. This is a no-op if the handle is not a console
// (e.g. when output is redirected to a file).
func enableANSI() {
	handle := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return
	}
	windows.SetConsoleMode(handle, mode|enableVirtualTerminalProcessing) //nolint:errcheck
}
