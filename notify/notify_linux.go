//go:build linux

package notify

import (
	"fmt"
	"os/exec"

	"github.com/biswajee/rose/monitor"
)

type linuxNotifier struct {
	available bool // false after first failed LookPath
}

func newPlatformNotifier() Notifier {
	_, err := exec.LookPath("notify-send")
	return &linuxNotifier{available: err == nil}
}

// Send dispatches a desktop notification via notify-send.
// If notify-send is not installed the call is silently skipped.
func (n *linuxNotifier) Send(evt monitor.DeviceEvent) error {
	if !n.available {
		return nil
	}

	summary := fmt.Sprintf("Device %s", evt.Action)

	body := evt.Name
	if body == "" {
		body = evt.DeviceID
	}
	if evt.Port != "" {
		body += " (" + evt.Port + ")"
	}

	cmd := exec.Command(
		"notify-send",
		summary, body,
		"--app-name=rose",
		"--expire-time=5000",
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("notify-send: %w", err)
	}
	return nil
}
