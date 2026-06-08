//go:build linux

package monitor

import (
	"strings"
	"testing"
)

func TestParseUevent_Basic(t *testing.T) {
	raw := "add@/devices/pci0000:00/0000:00:14.0/usb1/1-2\x00" +
		"ACTION=add\x00" +
		"DEVPATH=/devices/pci0000:00/0000:00:14.0/usb1/1-2\x00" +
		"SUBSYSTEM=usb\x00" +
		"DEVTYPE=usb_device\x00" +
		"ID_VENDOR_ID=0403\x00" +
		"ID_MODEL_ID=6001\x00" +
		"ID_MODEL=FT232R_USB_UART\x00"

	m := parseUevent([]byte(raw))

	if m["ACTION"] != "add" {
		t.Errorf("ACTION: want add, got %q", m["ACTION"])
	}
	if m["SUBSYSTEM"] != "usb" {
		t.Errorf("SUBSYSTEM: want usb, got %q", m["SUBSYSTEM"])
	}
	if m["ID_VENDOR_ID"] != "0403" {
		t.Errorf("ID_VENDOR_ID: want 0403, got %q", m["ID_VENDOR_ID"])
	}
	if m["ID_MODEL"] != "FT232R_USB_UART" {
		t.Errorf("ID_MODEL: want FT232R_USB_UART, got %q", m["ID_MODEL"])
	}
}

func TestParseUevent_Remove(t *testing.T) {
	raw := "remove@/devices/usb1/1-2\x00ACTION=remove\x00SUBSYSTEM=usb\x00"
	m := parseUevent([]byte(raw))
	if m["ACTION"] != "remove" {
		t.Errorf("ACTION: want remove, got %q", m["ACTION"])
	}
}

func TestParseUevent_Empty(t *testing.T) {
	m := parseUevent([]byte{})
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestParseUevent_MissingEquals(t *testing.T) {
	raw := "add@/path\x00NOEQUALS\x00KEY=val\x00"
	m := parseUevent([]byte(raw))
	if m["KEY"] != "val" {
		t.Errorf("KEY: want val, got %q", m["KEY"])
	}
	if _, ok := m["NOEQUALS"]; ok {
		t.Error("NOEQUALS should not be in map")
	}
}

func TestBuildLinuxEvent_AttachWithVIDPID(t *testing.T) {
	uevent := map[string]string{
		"ACTION":       "add",
		"DEVPATH":      "/devices/pci0000:00/usb1/1-2",
		"SUBSYSTEM":    "usb",
		"DEVTYPE":      "usb_device",
		"ID_VENDOR_ID": "0403",
		"ID_MODEL_ID":  "6001",
		"ID_MODEL":     "FT232R",
	}
	evt := buildLinuxEvent(uevent)
	if evt.Action != Attached {
		t.Errorf("action: want Attached, got %q", evt.Action)
	}
	if evt.DeviceID != "VID:0403 PID:6001" {
		t.Errorf("deviceID: want VID:0403 PID:6001, got %q", evt.DeviceID)
	}
	if evt.Name != "FT232R" {
		t.Errorf("name: want FT232R, got %q", evt.Name)
	}
}

func TestBuildLinuxEvent_RemoveAction(t *testing.T) {
	uevent := map[string]string{
		"ACTION":    "remove",
		"DEVPATH":   "/devices/usb1/1-2",
		"SUBSYSTEM": "usb",
	}
	evt := buildLinuxEvent(uevent)
	if evt.Action != Removed {
		t.Errorf("action: want Removed, got %q", evt.Action)
	}
}

func TestBuildLinuxEvent_DevpathFallback(t *testing.T) {
	uevent := map[string]string{
		"ACTION":    "add",
		"DEVPATH":   "/devices/usb1/1-2",
		"SUBSYSTEM": "usb",
	}
	evt := buildLinuxEvent(uevent)
	if !strings.HasPrefix(evt.DeviceID, "/") {
		t.Errorf("expected devpath fallback, got %q", evt.DeviceID)
	}
}
