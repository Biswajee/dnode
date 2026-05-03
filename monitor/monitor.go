package monitor

import "time"

// Action describes whether a device was attached or removed.
type Action string

const (
	Attached Action = "ATTACHED"
	Removed  Action = "REMOVED"
)

// EndpointType is the USB transfer type for an endpoint.
type EndpointType string

const (
	EndpointControl     EndpointType = "Control"
	EndpointIsochronous EndpointType = "Isochronous"
	EndpointBulk        EndpointType = "Bulk"
	EndpointInterrupt   EndpointType = "Interrupt"
)

// Endpoint describes a single USB endpoint on a device.
type Endpoint struct {
	Address   uint8
	Direction string // "IN" or "OUT"
	Type      EndpointType
	MaxPacket uint16
	Interval  uint8 // relevant for Interrupt and Isochronous
}

// DeviceEvent is emitted when a device is attached or removed.
// Fields beyond Action and DeviceID are populated on a best-effort basis.
type DeviceEvent struct {
	Timestamp time.Time
	Action    Action
	DeviceID  string // "VID:XXXX PID:XXXX" or kernel path
	Name      string // friendly name / product description

	// USB descriptor strings
	Manufacturer string
	Product      string
	SerialNumber string

	// Serial/COM port assignment (serial devices only)
	Port string // "COM3" on Windows, "/dev/ttyUSB0" on Linux

	// USB endpoints (populated when available)
	Endpoints []Endpoint
}

// Monitor watches for device attach/remove events and sends them to the channel.
type Monitor interface {
	Run(events chan<- DeviceEvent) error
}
