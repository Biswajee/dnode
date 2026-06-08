package logger

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/biswajee/dnode/monitor"
)

var baseTime = time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)

func TestNew_CreatesFile(t *testing.T) {
	f, err := os.CreateTemp("", "dnode_test_*.log")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	l, err := New(f.Name(), false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestLog_WritesToFile(t *testing.T) {
	f, err := os.CreateTemp("", "dnode_test_*.log")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	l, err := New(f.Name(), false)
	if err != nil {
		t.Fatal(err)
	}

	l.Log(monitor.DeviceEvent{
		Timestamp: baseTime,
		Action:    monitor.Attached,
		DeviceID:  "VID:0403 PID:6001",
		Name:      "FTDI",
	})
	l.Close()

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "ATTACHED") {
		t.Errorf("log file missing ATTACHED: %q", string(data))
	}
}

func TestFormatEvent_BasicAttach(t *testing.T) {
	evt := monitor.DeviceEvent{
		Timestamp: baseTime,
		Action:    monitor.Attached,
		DeviceID:  "VID:0403 PID:6001",
		Name:      "FTDI USB Serial",
	}
	lines := FormatEvent(evt)
	if len(lines) < 1 {
		t.Fatal("expected at least one line")
	}
	header := lines[0]
	if !strings.Contains(header, "ATTACHED") {
		t.Errorf("header missing action: %q", header)
	}
	if !strings.Contains(header, "VID:0403 PID:6001") {
		t.Errorf("header missing device ID: %q", header)
	}
	if !strings.Contains(header, "FTDI USB Serial") {
		t.Errorf("header missing name: %q", header)
	}
}

func TestFormatEvent_UnknownName(t *testing.T) {
	evt := monitor.DeviceEvent{
		Timestamp: baseTime,
		Action:    monitor.Attached,
		DeviceID:  "VID:1234 PID:5678",
	}
	lines := FormatEvent(evt)
	if !strings.Contains(lines[0], "(unknown device)") {
		t.Errorf("expected '(unknown device)' placeholder, got: %q", lines[0])
	}
}

func TestFormatEvent_WithPort(t *testing.T) {
	evt := monitor.DeviceEvent{
		Timestamp: baseTime,
		Action:    monitor.Attached,
		DeviceID:  "VID:0403 PID:6001",
		Name:      "FTDI",
		Port:      "COM3",
	}
	lines := FormatEvent(evt)
	if !strings.Contains(lines[0], "(COM3)") {
		t.Errorf("expected port in header, got: %q", lines[0])
	}
}

func TestFormatEvent_DetailFields(t *testing.T) {
	evt := monitor.DeviceEvent{
		Timestamp:    baseTime,
		Action:       monitor.Attached,
		DeviceID:     "VID:0403 PID:6001",
		Name:         "FTDI",
		Manufacturer: "Future Technology Devices",
		Product:      "USB Serial Converter",
		SerialNumber: "FT1ABC23",
	}
	lines := FormatEvent(evt)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Future Technology Devices", "USB Serial Converter", "FT1ABC23"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing field %q in output:\n%s", want, joined)
		}
	}
}

func TestFormatEvent_Endpoints(t *testing.T) {
	evt := monitor.DeviceEvent{
		Timestamp: baseTime,
		Action:    monitor.Attached,
		DeviceID:  "VID:0403 PID:6001",
		Name:      "FTDI",
		Endpoints: []monitor.Endpoint{
			{Address: 0x81, Direction: "IN", Type: monitor.EndpointBulk, MaxPacket: 64},
			{Address: 0x02, Direction: "OUT", Type: monitor.EndpointBulk, MaxPacket: 64},
		},
	}
	lines := FormatEvent(evt)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "EP0x81") {
		t.Errorf("missing EP0x81 in output:\n%s", joined)
	}
	if !strings.Contains(joined, "EP0x02") {
		t.Errorf("missing EP0x02 in output:\n%s", joined)
	}
}

func TestFormatEvent_RemoveAction(t *testing.T) {
	evt := monitor.DeviceEvent{
		Timestamp: baseTime,
		Action:    monitor.Removed,
		DeviceID:  "VID:0403 PID:6001",
		Name:      "FTDI",
	}
	lines := FormatEvent(evt)
	if !strings.Contains(lines[0], "REMOVED") {
		t.Errorf("expected REMOVED action, got: %q", lines[0])
	}
}

func TestFmtEndpoint_WithInterval(t *testing.T) {
	ep := monitor.Endpoint{
		Address:   0x81,
		Direction: "IN",
		Type:      monitor.EndpointInterrupt,
		MaxPacket: 8,
		Interval:  10,
	}
	s := fmtEndpoint(ep)
	if !strings.Contains(s, "interval=10") {
		t.Errorf("expected interval=10, got: %q", s)
	}
}

func TestFmtEndpoint_NoInterval(t *testing.T) {
	ep := monitor.Endpoint{
		Address:   0x02,
		Direction: "OUT",
		Type:      monitor.EndpointBulk,
		MaxPacket: 512,
		Interval:  0,
	}
	s := fmtEndpoint(ep)
	if strings.Contains(s, "interval") {
		t.Errorf("unexpected interval field, got: %q", s)
	}
}
