package logger

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/biswajee/rose/monitor"
)

// Logger writes formatted device events to a log file and optionally stdout.
type Logger struct {
	mu      sync.Mutex
	file    *os.File
	console bool
}

// New opens (or creates and appends to) the log file at path.
// If console is true, each event is also printed to stdout.
func New(path string, console bool) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &Logger{file: f, console: console}, nil
}

// Close closes the underlying log file.
func (l *Logger) Close() error {
	return l.file.Close()
}

// Log writes a formatted device event to the log file (and stdout if enabled).
func (l *Logger) Log(evt monitor.DeviceEvent) {
	lines := formatEvent(evt)

	l.mu.Lock()
	defer l.mu.Unlock()

	for _, line := range lines {
		fmt.Fprintln(l.file, line)
		if l.console {
			fmt.Println(line)
		}
	}
}

// formatEvent returns one or more lines describing the event.
// The first line is always the summary; subsequent lines carry optional detail.
func formatEvent(evt monitor.DeviceEvent) []string {
	name := evt.Name
	if name == "" {
		name = "(unknown device)"
	}

	port := ""
	if evt.Port != "" {
		port = " (" + evt.Port + ")"
	}

	header := fmt.Sprintf("%-30s  %-8s  %-30s  %s%s",
		evt.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
		string(evt.Action),
		evt.DeviceID,
		name,
		port,
	)

	lines := []string{header}

	detail := func(key, val string) {
		if val != "" {
			lines = append(lines, fmt.Sprintf("  %-14s: %s", key, val))
		}
	}

	detail("manufacturer", evt.Manufacturer)
	detail("product", evt.Product)
	detail("serial", evt.SerialNumber)

	if len(evt.Endpoints) > 0 {
		for i, ep := range evt.Endpoints {
			epStr := fmtEndpoint(ep)
			if i == 0 {
				lines = append(lines, fmt.Sprintf("  %-14s: %s", "endpoints", epStr))
			} else {
				lines = append(lines, fmt.Sprintf("  %-14s  %s", "", epStr))
			}
		}
	}

	return lines
}

func fmtEndpoint(ep monitor.Endpoint) string {
	s := fmt.Sprintf("EP0x%02X %-3s %-14s %dB",
		ep.Address,
		ep.Direction,
		string(ep.Type),
		ep.MaxPacket,
	)
	if ep.Interval > 0 {
		s += fmt.Sprintf("  interval=%d", ep.Interval)
	}
	return strings.TrimRight(s, " ")
}
