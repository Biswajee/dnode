//go:build windows

package monitor

import "testing"

func TestParseDevicePath_FTDI(t *testing.T) {
	path := `\\?\USB#VID_0403&PID_6001#FT1ABC23#{a5dcbf10-6530-11d2-901f-00c04fb951ed}`
	vid, pid, instanceID, serial := parseDevicePath(path)

	if vid != "0403" {
		t.Errorf("vid: want 0403, got %q", vid)
	}
	if pid != "6001" {
		t.Errorf("pid: want 6001, got %q", pid)
	}
	if instanceID != `USB\VID_0403&PID_6001\FT1ABC23` {
		t.Errorf("instanceID: got %q", instanceID)
	}
	if serial != "FT1ABC23" {
		t.Errorf("serial: want FT1ABC23, got %q", serial)
	}
}

func TestParseDevicePath_NoSerial(t *testing.T) {
	path := `\\?\USB#VID_046D&PID_C52B#{a5dcbf10-6530-11d2-901f-00c04fb951ed}`
	vid, pid, _, serial := parseDevicePath(path)

	if vid != "046d" {
		t.Errorf("vid: want 046d, got %q", vid)
	}
	if pid != "c52b" {
		t.Errorf("pid: want c52b, got %q", pid)
	}
	if serial != "" {
		t.Errorf("serial: want empty, got %q", serial)
	}
}

func TestParseDevicePath_MissingVIDPID(t *testing.T) {
	path := `\\?\HID#HID_DEVICE#7&abc#{a5dcbf10-6530-11d2-901f-00c04fb951ed}`
	vid, pid, _, _ := parseDevicePath(path)
	if vid != "" || pid != "" {
		t.Errorf("expected empty vid/pid for non-USB path, got vid=%q pid=%q", vid, pid)
	}
}

func TestCleanRegistryString_WithPrefix(t *testing.T) {
	s := `@oem42.inf,%USB\VID_0403&PID_6001.DeviceDesc%;USB Serial Converter`
	got := cleanRegistryString(s)
	want := "USB Serial Converter"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestCleanRegistryString_Plain(t *testing.T) {
	s := "USB Serial Converter"
	got := cleanRegistryString(s)
	if got != s {
		t.Errorf("want %q unchanged, got %q", s, got)
	}
}

func TestCleanRegistryString_Empty(t *testing.T) {
	if got := cleanRegistryString(""); got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

func TestCleanRegistryString_Whitespace(t *testing.T) {
	got := cleanRegistryString("  FTDI  ")
	if got != "FTDI" {
		t.Errorf("want trimmed, got %q", got)
	}
}
