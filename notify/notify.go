package notify

import "github.com/biswajee/rose/monitor"

// Notifier dispatches a desktop notification for a device event.
type Notifier interface {
	Send(evt monitor.DeviceEvent) error
}

// New returns a platform-appropriate Notifier.
func New() Notifier {
	return newPlatformNotifier()
}
