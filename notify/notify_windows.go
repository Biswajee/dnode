//go:build windows

package notify

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/biswajee/rose/monitor"
)

type windowsNotifier struct{}

func newPlatformNotifier() Notifier {
	return &windowsNotifier{}
}

// Send dispatches a Windows toast notification via PowerShell.
// The WinRT Windows.UI.Notifications API is invoked entirely in the
// PowerShell process, keeping the Go binary free of COM/WinRT dependencies.
func (n *windowsNotifier) Send(evt monitor.DeviceEvent) error {
	title := fmt.Sprintf("Device %s", evt.Action)

	body := evt.Name
	if body == "" {
		body = evt.DeviceID
	}
	if evt.Port != "" {
		body += " (" + evt.Port + ")"
	}

	// Escape single quotes for embedding in the PowerShell string.
	title = strings.ReplaceAll(title, "'", "''")
	body = strings.ReplaceAll(body, "'", "''")
	// Escape XML special characters for the toast XML payload.
	body = strings.ReplaceAll(body, "&", "&amp;")
	body = strings.ReplaceAll(body, "<", "&lt;")
	body = strings.ReplaceAll(body, ">", "&gt;")

	script := fmt.Sprintf(`
$null = [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType=WindowsRuntime]
$null = [Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom, ContentType=WindowsRuntime]
$xml = [Windows.Data.Xml.Dom.XmlDocument]::new()
$xml.LoadXml('<toast><visual><binding template="ToastText02"><text id="1">%s</text><text id="2">%s</text></binding></visual></toast>')
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('rose').Show($toast)
`, title, body)

	cmd := exec.Command(
		"powershell",
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
		"-Command", script,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("toast notification: %w", err)
	}
	return nil
}
