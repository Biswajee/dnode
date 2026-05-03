//go:build linux

package monitor

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const netlinkKernelUevent = 15 // NETLINK_KOBJECT_UEVENT

type linuxMonitor struct{}

// New returns a Monitor for Linux.
func New() Monitor {
	return &linuxMonitor{}
}

// Run opens a NETLINK_KOBJECT_UEVENT socket and dispatches device events.
// It blocks until the socket is closed or an unrecoverable error occurs.
func (m *linuxMonitor) Run(events chan<- DeviceEvent) error {
	fd, err := syscall.Socket(
		syscall.AF_NETLINK,
		syscall.SOCK_RAW|syscall.SOCK_CLOEXEC,
		netlinkKernelUevent,
	)
	if err != nil {
		return fmt.Errorf("netlink socket: %w", err)
	}
	defer syscall.Close(fd)

	addr := &syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
		Groups: 1, // kernel uevents multicast group
	}
	if err := syscall.Bind(fd, addr); err != nil {
		return fmt.Errorf("bind netlink: %w", err)
	}

	buf := make([]byte, 16384)
	for {
		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			return fmt.Errorf("recvfrom: %w", err)
		}

		uevent := parseUevent(buf[:n])
		action, ok := uevent["ACTION"]
		if !ok {
			continue
		}
		if action != "add" && action != "remove" {
			continue
		}

		// Only report subsystems that represent physical hardware.
		subsystem := uevent["SUBSYSTEM"]
		switch subsystem {
		case "usb", "tty", "block", "net", "input", "hidraw", "hid":
		default:
			continue
		}

		// Skip usb_interface entries – we want the usb_device level.
		if subsystem == "usb" && uevent["DEVTYPE"] != "usb_device" {
			continue
		}

		evt := buildLinuxEvent(uevent)
		events <- evt
	}
}

// parseUevent splits the raw netlink uevent buffer into a key=value map.
// The first record is a "ACTION@/path" header line.
func parseUevent(data []byte) map[string]string {
	result := make(map[string]string)
	start := 0
	for i, b := range data {
		if b == 0 {
			line := string(data[start:i])
			start = i + 1
			if line == "" {
				continue
			}
			if idx := strings.IndexByte(line, '='); idx >= 0 {
				result[line[:idx]] = line[idx+1:]
			} else if idx := strings.IndexByte(line, '@'); idx >= 0 {
				// Header: "add@/devices/..."
				result["ACTION"] = line[:idx]
				result["DEVPATH"] = line[idx+1:]
			}
		}
	}
	return result
}

// buildLinuxEvent constructs a DeviceEvent from a parsed uevent map.
func buildLinuxEvent(uevent map[string]string) DeviceEvent {
	action := Removed
	if uevent["ACTION"] == "add" {
		action = Attached
	}

	devpath := uevent["DEVPATH"]
	sysfsPath := "/sys" + devpath

	vid := strings.ToUpper(uevent["ID_VENDOR_ID"])
	pid := strings.ToUpper(uevent["ID_MODEL_ID"])

	deviceID := devpath
	if vid != "" && pid != "" {
		deviceID = fmt.Sprintf("VID:%s PID:%s", vid, pid)
	}

	evt := DeviceEvent{
		Timestamp: time.Now().UTC(),
		Action:    action,
		DeviceID:  deviceID,
		Name:      uevent["ID_MODEL"],
	}

	subsystem := uevent["SUBSYSTEM"]

	if subsystem == "usb" {
		evt.Manufacturer = readSysfsFile(sysfsPath, "manufacturer")
		evt.Product = readSysfsFile(sysfsPath, "product")
		evt.SerialNumber = readSysfsFile(sysfsPath, "serial")
		if evt.Name == "" {
			evt.Name = evt.Product
		}
		evt.Endpoints = readSysfsEndpoints(sysfsPath)
		evt.Port = findTTYPort(sysfsPath)
	}

	if subsystem == "tty" {
		devname := uevent["DEVNAME"]
		if devname != "" {
			if !strings.HasPrefix(devname, "/") {
				devname = "/dev/" + devname
			}
			evt.Port = devname
		}
		// Walk up sysfs to find the parent USB device for descriptor strings.
		parentPath := findUSBParent(sysfsPath)
		if parentPath != "" {
			evt.Manufacturer = readSysfsFile(parentPath, "manufacturer")
			evt.Product = readSysfsFile(parentPath, "product")
			evt.SerialNumber = readSysfsFile(parentPath, "serial")
			parentVID := strings.ToUpper(readSysfsFile(parentPath, "idVendor"))
			parentPID := strings.ToUpper(readSysfsFile(parentPath, "idProduct"))
			if parentVID != "" && parentPID != "" {
				evt.DeviceID = fmt.Sprintf("VID:%s PID:%s", parentVID, parentPID)
			}
			if evt.Name == "" {
				evt.Name = evt.Product
			}
		}
	}

	return evt
}

// readSysfsFile reads a single sysfs file and returns its trimmed content.
func readSysfsFile(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// readSysfsEndpoints walks sysfs interface directories for ep_* subdirectories
// and builds an Endpoint slice from the USB descriptor files found there.
func readSysfsEndpoints(devpath string) []Endpoint {
	entries, err := os.ReadDir(devpath)
	if err != nil {
		return nil
	}

	var endpoints []Endpoint
	seen := make(map[uint8]bool)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Interface directories look like "X-X:Y.Z"
		ifPath := filepath.Join(devpath, entry.Name())
		epEntries, err := os.ReadDir(ifPath)
		if err != nil {
			continue
		}
		for _, epEntry := range epEntries {
			if !strings.HasPrefix(epEntry.Name(), "ep_") {
				continue
			}
			epPath := filepath.Join(ifPath, epEntry.Name())
			ep := readEndpoint(epPath)
			if ep == nil {
				continue
			}
			// Deduplicate endpoints shared across interfaces.
			if seen[ep.Address] {
				continue
			}
			seen[ep.Address] = true
			endpoints = append(endpoints, *ep)
		}
	}
	return endpoints
}

// readEndpoint parses a sysfs ep_XX directory into an Endpoint.
func readEndpoint(epPath string) *Endpoint {
	addrStr := readSysfsFile(epPath, "bEndpointAddress")
	if addrStr == "" {
		return nil
	}

	var addr, attr, pkt, interval uint64
	fmt.Sscanf(addrStr, "%x", &addr)
	fmt.Sscanf(readSysfsFile(epPath, "bmAttributes"), "%x", &attr)
	fmt.Sscanf(readSysfsFile(epPath, "wMaxPacketSize"), "%x", &pkt)
	fmt.Sscanf(readSysfsFile(epPath, "bInterval"), "%x", &interval)

	direction := "OUT"
	if addr&0x80 != 0 {
		direction = "IN"
	}

	var epType EndpointType
	switch attr & 0x03 {
	case 0:
		epType = EndpointControl
	case 1:
		epType = EndpointIsochronous
	case 2:
		epType = EndpointBulk
	case 3:
		epType = EndpointInterrupt
	}

	return &Endpoint{
		Address:   uint8(addr),
		Direction: direction,
		Type:      epType,
		MaxPacket: uint16(pkt),
		Interval:  uint8(interval),
	}
}

// findUSBParent walks up the sysfs path until it finds a directory that
// contains an "idVendor" file, indicating a USB device node.
func findUSBParent(sysfsPath string) string {
	dir := filepath.Dir(sysfsPath)
	for dir != "/" && dir != "." {
		if _, err := os.Stat(filepath.Join(dir, "idVendor")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

// findTTYPort walks the sysfs device directory tree looking for a tty node
// (ttyUSB*, ttyACM*, etc.) and returns its /dev path if found.
func findTTYPort(devpath string) string {
	var port string
	filepath.WalkDir(devpath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, "ttyUSB") ||
			strings.HasPrefix(name, "ttyACM") ||
			strings.HasPrefix(name, "ttyS") {
			port = "/dev/" + name
			return filepath.SkipAll
		}
		return nil
	})
	return port
}
