//go:build windows

package monitor

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// ── Constants ────────────────────────────────────────────────────────────────

const (
	wmDeviceChange           = 0x0219
	dbtDeviceArrival         = 0x8000
	dbtDevtypDeviceInterface = 5

	deviceNotifyWindowHandle        = 0x00000000
	deviceNotifyAllInterfaceClasses = 0x00000004

	// cfgmgr32 return code
	crSuccess = 0x00000000

	// CM device node registry property IDs
	cmDrpAddress = 0x0000001D // connection index on parent hub

	// CM_Locate_DevNode flags
	cmLocateDevnodeNormal = 0x00000000

	// USB hub IOCTL
	ioctlUsbGetNodeConnInfoEx = 0x00220448

	// USB endpoint direction bit
	usbEPDirIn = 0x80

	// USB transfer types (bmAttributes & 0x03)
	usbTransferControl  = 0
	usbTransferIso      = 1
	usbTransferBulk     = 2
	usbTransferInterrupt = 3

	// USB hub class GUID string (used to build hub device path)
	usbHubGUID = "{f18a0e88-c30c-11d0-8815-00a0c906bed8}"
)

// ── Win32 structures ─────────────────────────────────────────────────────────

type devBroadcastHdr struct {
	Size       uint32
	DeviceType uint32
	Reserved   uint32
}

// devBroadcastDeviceInterface mirrors DEV_BROADCAST_DEVICEINTERFACE_W.
// Name is variable-length; we only declare one element and index past it.
type devBroadcastDeviceInterface struct {
	Size       uint32
	DeviceType uint32
	Reserved   uint32
	ClassGuid  [16]byte // GUID
	Name       [1]uint16
}

// wndClassExW mirrors WNDCLASSEXW.
type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

// msgW mirrors MSG.
type msgW struct {
	HWND    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
	Private uint32
}

// usbEndpointDescriptor mirrors USB_ENDPOINT_DESCRIPTOR.
type usbEndpointDescriptor struct {
	Length          uint8
	DescriptorType  uint8
	EndpointAddress uint8
	Attributes      uint8
	MaxPacketSize   uint16
	Interval        uint8
}

// usbPipeInfo mirrors USB_PIPE_INFO.
type usbPipeInfo struct {
	ScheduleOffset     uint32
	EndpointDescriptor usbEndpointDescriptor
	_                  [3]byte // struct padding
}

// usbNodeConnInfoEx mirrors USB_NODE_CONNECTION_INFORMATION_EX (truncated pipe list).
// Real devices rarely have more than 32 endpoints.
type usbNodeConnInfoEx struct {
	ConnectionIndex          uint32
	DeviceDescriptor         [18]byte
	CurrentConfigurationValue uint8
	Speed                    uint8
	DeviceIsHub              uint8
	_                        [1]byte
	DeviceAddress            uint16
	_                        [2]byte
	NumberOfOpenPipes        uint32
	ConnectionStatus         uint32
	PipeList                 [32]usbPipeInfo
}

// ── Lazy-loaded DLLs ─────────────────────────────────────────────────────────

var (
	modUser32    = windows.NewLazySystemDLL("user32.dll")
	modKernel32  = windows.NewLazySystemDLL("kernel32.dll")
	modCfgMgr32  = windows.NewLazySystemDLL("cfgmgr32.dll")

	procGetModuleHandleW             = modKernel32.NewProc("GetModuleHandleW")
	procRegisterClassExW             = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW              = modUser32.NewProc("CreateWindowExW")
	procGetMessageW                  = modUser32.NewProc("GetMessageW")
	procTranslateMessage             = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW             = modUser32.NewProc("DispatchMessageW")
	procDefWindowProcW               = modUser32.NewProc("DefWindowProcW")
	procPostQuitMessage              = modUser32.NewProc("PostQuitMessage")
	procDestroyWindow                = modUser32.NewProc("DestroyWindow")
	procRegisterDeviceNotificationW  = modUser32.NewProc("RegisterDeviceNotificationW")
	procUnregisterDeviceNotification = modUser32.NewProc("UnregisterDeviceNotification")

	procCMLocateDevNodeW          = modCfgMgr32.NewProc("CM_Locate_DevNodeW")
	procCMGetDevNodeRegistryPropW = modCfgMgr32.NewProc("CM_Get_DevNode_Registry_PropertyW")
	procCMGetParent               = modCfgMgr32.NewProc("CM_Get_Parent")
	procCMGetDeviceIDW            = modCfgMgr32.NewProc("CM_Get_Device_IDW")
	procCMGetChild                = modCfgMgr32.NewProc("CM_Get_Child")
	procCMGetSibling              = modCfgMgr32.NewProc("CM_Get_Sibling")
)

// ── Package-level state (required for the WndProc callback) ──────────────────

var (
	gMu     sync.Mutex
	gMon    *windowsMonitor
	gEvents chan<- DeviceEvent
)

// ── windowsMonitor ────────────────────────────────────────────────────────────

type windowsMonitor struct {
	cacheMu sync.Mutex
	cache   map[string]DeviceEvent // keyed by normalised device path
}

// New returns a Monitor for Windows.
func New() Monitor {
	return &windowsMonitor{
		cache: make(map[string]DeviceEvent),
	}
}

// Run creates a hidden message window, registers for all device interface
// notifications, then runs the Win32 message loop on the locked OS thread.
func (m *windowsMonitor) Run(events chan<- DeviceEvent) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	gMu.Lock()
	gMon = m
	gEvents = events
	cb := syscall.NewCallback(wndProc)
	gMu.Unlock()

	hwnd, err := createMessageWindow(cb)
	if err != nil {
		return fmt.Errorf("create message window: %w", err)
	}

	notifyHandle, err := registerAllDeviceNotifications(hwnd)
	if err != nil {
		destroyWindow(hwnd)
		return fmt.Errorf("register device notifications: %w", err)
	}

	defer func() {
		procUnregisterDeviceNotification.Call(notifyHandle) //nolint:errcheck
		destroyWindow(hwnd)
	}()

	return messageLoop()
}

// wndProc is the Win32 window procedure for our hidden message window.
// It is called on the locked OS thread by DispatchMessage.
func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	if msg == wmDeviceChange && lParam != 0 {
		hdr := (*devBroadcastHdr)(unsafe.Pointer(lParam))
		if hdr.DeviceType == dbtDevtypDeviceInterface {
			dbi := (*devBroadcastDeviceInterface)(unsafe.Pointer(lParam))
			path := extractDevicePath(dbi)
			action := Removed
			if wParam == dbtDeviceArrival {
				action = Attached
			}
			go func() {
				evt := gMon.buildEvent(action, path)
				select {
				case gEvents <- evt:
				default:
				}
			}()
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// extractDevicePath reads the variable-length UTF-16 Name field from a
// DEV_BROADCAST_DEVICEINTERFACE structure and returns it as a Go string.
func extractDevicePath(dbi *devBroadcastDeviceInterface) string {
	// Name starts at offset 28 (4+4+4+16).
	const nameOffset = 28
	if int(dbi.Size) <= nameOffset {
		return ""
	}
	nameLen := (int(dbi.Size) - nameOffset) / 2
	namePtr := (*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(dbi)) + nameOffset))
	nameSlice := unsafe.Slice(namePtr, nameLen)
	return syscall.UTF16ToString(nameSlice)
}

// buildEvent constructs and enriches a DeviceEvent for the given action and
// Windows device interface path.
func (m *windowsMonitor) buildEvent(action Action, devicePath string) DeviceEvent {
	vid, pid, instanceID, serialFromPath := parseDevicePath(devicePath)

	deviceID := devicePath
	if vid != "" && pid != "" {
		deviceID = fmt.Sprintf("VID:%s PID:%s", strings.ToUpper(vid), strings.ToUpper(pid))
	}

	evt := DeviceEvent{
		Timestamp: time.Now().UTC(),
		Action:    action,
		DeviceID:  deviceID,
	}

	if action == Attached {
		// Enrich from registry.
		name, mfg, product := queryDeviceInfo(instanceID)
		evt.Name = name
		evt.Manufacturer = mfg
		evt.Product = product
		evt.SerialNumber = serialFromPath
		evt.Port = queryComPort(instanceID)
		evt.Endpoints = queryEndpoints(instanceID)

		// Cache for the corresponding remove event.
		m.cacheMu.Lock()
		m.cache[strings.ToLower(devicePath)] = evt
		m.cacheMu.Unlock()
	} else {
		// Attempt to serve from cache.
		m.cacheMu.Lock()
		cached, ok := m.cache[strings.ToLower(devicePath)]
		if ok {
			delete(m.cache, strings.ToLower(devicePath))
		}
		m.cacheMu.Unlock()

		if ok {
			cached.Timestamp = evt.Timestamp
			cached.Action = Removed
			return cached
		}
		// No cache hit - best-effort from path only.
		evt.SerialNumber = serialFromPath
	}

	return evt
}

// parseDevicePath extracts VID, PID, device instance ID, and serial number
// from a Windows device interface path.
//
//	Input:  \\?\USB#VID_0403&PID_6001#FT1ABC23#{...}
//	Output: vid="0403", pid="6001",
//	        instanceID="USB\VID_0403&PID_6001\FT1ABC23",
//	        serial="FT1ABC23"
func parseDevicePath(path string) (vid, pid, instanceID, serial string) {
	// Strip \\?\ prefix.
	s := strings.TrimPrefix(path, `\\?\`)
	// Strip trailing #{guid}.
	if idx := strings.LastIndex(s, "#{"); idx >= 0 {
		s = s[:idx]
	}
	// s is now like "USB#VID_0403&PID_6001#FT1ABC23"
	instanceID = strings.ReplaceAll(s, "#", `\`)

	parts := strings.Split(s, "#")
	if len(parts) >= 3 {
		serial = parts[2]
	}
	if len(parts) >= 2 {
		for _, seg := range strings.Split(parts[1], "&") {
			upper := strings.ToUpper(seg)
			if strings.HasPrefix(upper, "VID_") {
				vid = strings.ToLower(strings.TrimPrefix(upper, "VID_"))
			} else if strings.HasPrefix(upper, "PID_") {
				pid = strings.ToLower(strings.TrimPrefix(upper, "PID_"))
			}
		}
	}
	return
}

// queryDeviceInfo reads device name, manufacturer, and product description
// from the Windows registry under HKLM\SYSTEM\CurrentControlSet\Enum\<id>.
func queryDeviceInfo(instanceID string) (name, manufacturer, product string) {
	keyPath := `SYSTEM\CurrentControlSet\Enum\` + instanceID
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer k.Close()

	name, _, _ = k.GetStringValue("FriendlyName")
	if name == "" {
		name, _, _ = k.GetStringValue("DeviceDesc")
	}
	manufacturer, _, _ = k.GetStringValue("Mfg")
	product, _, _ = k.GetStringValue("DeviceDesc")

	name = cleanRegistryString(name)
	manufacturer = cleanRegistryString(manufacturer)
	product = cleanRegistryString(product)
	return
}

// cleanRegistryString strips the "@driver.inf,%Key%;" prefix that Windows
// sometimes stores in device registry strings.
func cleanRegistryString(s string) string {
	if len(s) > 0 && s[0] == '@' {
		if idx := strings.LastIndex(s, ";"); idx >= 0 {
			return strings.TrimSpace(s[idx+1:])
		}
	}
	return strings.TrimSpace(s)
}

// queryComPort finds the COM port name for a device. It first checks the
// device's own registry key (works for USB CDC ACM devices), then walks child
// device nodes up to three levels deep (needed for FTDI and other devices
// that expose their serial port via a virtual child bus).
func queryComPort(instanceID string) string {
	if port := readPortName(instanceID); port != "" {
		return port
	}
	instanceIDW, err := syscall.UTF16PtrFromString(instanceID)
	if err != nil {
		return ""
	}
	var devInst uint32
	r, _, _ := procCMLocateDevNodeW.Call(
		uintptr(unsafe.Pointer(&devInst)),
		uintptr(unsafe.Pointer(instanceIDW)),
		cmLocateDevnodeNormal,
	)
	if r != crSuccess {
		return ""
	}
	return walkForPort(devInst, 3)
}

// readPortName reads PortName from the Device Parameters registry key
// for a given device instance ID.
func readPortName(instanceID string) string {
	k, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Enum\`+instanceID+`\Device Parameters`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return ""
	}
	defer k.Close()
	port, _, err := k.GetStringValue("PortName")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(port)
}

// walkForPort performs a depth-first walk of child device nodes searching for
// one that has a PortName value in its Device Parameters registry key.
func walkForPort(devInst uint32, depth int) string {
	if depth == 0 {
		return ""
	}
	var child uint32
	r, _, _ := procCMGetChild.Call(
		uintptr(unsafe.Pointer(&child)),
		uintptr(devInst),
		0,
	)
	if r != crSuccess {
		return ""
	}
	for {
		buf := make([]uint16, 512)
		r, _, _ := procCMGetDeviceIDW.Call(
			uintptr(child),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
			0,
		)
		if r == crSuccess {
			childID := syscall.UTF16ToString(buf)
			if port := readPortName(childID); port != "" {
				return port
			}
			if port := walkForPort(child, depth-1); port != "" {
				return port
			}
		}
		var sibling uint32
		r, _, _ = procCMGetSibling.Call(
			uintptr(unsafe.Pointer(&sibling)),
			uintptr(child),
			0,
		)
		if r != crSuccess {
			break
		}
		child = sibling
	}
	return ""
}

// queryEndpoints attempts to retrieve USB endpoint descriptors by locating
// the parent hub via cfgmgr32 and issuing IOCTL_USB_GET_NODE_CONNECTION_INFORMATION_EX.
// Returns nil gracefully if any step fails.
func queryEndpoints(instanceID string) []Endpoint {
	// Locate the device node.
	instanceIDW, err := syscall.UTF16PtrFromString(instanceID)
	if err != nil {
		return nil
	}
	var devInst uint32
	r, _, _ := procCMLocateDevNodeW.Call(
		uintptr(unsafe.Pointer(&devInst)),
		uintptr(unsafe.Pointer(instanceIDW)),
		cmLocateDevnodeNormal,
	)
	if r != crSuccess {
		return nil
	}

	// Get the connection index (port number on the parent hub).
	var connIndex uint32
	var propType uint32
	propSize := uint32(unsafe.Sizeof(connIndex))
	r, _, _ = procCMGetDevNodeRegistryPropW.Call(
		uintptr(devInst),
		cmDrpAddress,
		uintptr(unsafe.Pointer(&propType)),
		uintptr(unsafe.Pointer(&connIndex)),
		uintptr(unsafe.Pointer(&propSize)),
		0,
	)
	if r != crSuccess || connIndex == 0 {
		return nil
	}

	// Get the parent device node (the hub).
	var parentInst uint32
	r, _, _ = procCMGetParent.Call(
		uintptr(unsafe.Pointer(&parentInst)),
		uintptr(devInst),
		0,
	)
	if r != crSuccess {
		return nil
	}

	// Get the parent device instance ID string.
	parentIDBuf := make([]uint16, 256)
	r, _, _ = procCMGetDeviceIDW.Call(
		uintptr(parentInst),
		uintptr(unsafe.Pointer(&parentIDBuf[0])),
		256,
		0,
	)
	if r != crSuccess {
		return nil
	}
	parentID := syscall.UTF16ToString(parentIDBuf)

	// Build the hub device interface path:
	// "USB\ROOT_HUB30\4&abc" → "\\?\usb#root_hub30#4&abc#{hub-guid}"
	hubPath := `\\?\` + strings.ToLower(strings.ReplaceAll(parentID, `\`, "#")) +
		"#" + usbHubGUID

	hubPathW, err := syscall.UTF16PtrFromString(hubPath)
	if err != nil {
		return nil
	}

	// Open the hub.
	hubHandle, err := windows.CreateFile(
		hubPathW,
		windows.GENERIC_WRITE,
		windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(hubHandle) //nolint:errcheck

	// Issue IOCTL_USB_GET_NODE_CONNECTION_INFORMATION_EX.
	var connInfo usbNodeConnInfoEx
	connInfo.ConnectionIndex = connIndex
	var bytesReturned uint32
	err = windows.DeviceIoControl(
		hubHandle,
		ioctlUsbGetNodeConnInfoEx,
		(*byte)(unsafe.Pointer(&connInfo)),
		uint32(unsafe.Sizeof(connInfo)),
		(*byte)(unsafe.Pointer(&connInfo)),
		uint32(unsafe.Sizeof(connInfo)),
		&bytesReturned,
		nil,
	)
	if err != nil {
		return nil
	}

	n := int(connInfo.NumberOfOpenPipes)
	if n > len(connInfo.PipeList) {
		n = len(connInfo.PipeList)
	}

	endpoints := make([]Endpoint, 0, n)
	for i := 0; i < n; i++ {
		desc := connInfo.PipeList[i].EndpointDescriptor
		direction := "OUT"
		if desc.EndpointAddress&usbEPDirIn != 0 {
			direction = "IN"
		}
		var epType EndpointType
		switch desc.Attributes & 0x03 {
		case usbTransferControl:
			epType = EndpointControl
		case usbTransferIso:
			epType = EndpointIsochronous
		case usbTransferBulk:
			epType = EndpointBulk
		case usbTransferInterrupt:
			epType = EndpointInterrupt
		}
		endpoints = append(endpoints, Endpoint{
			Address:   desc.EndpointAddress,
			Direction: direction,
			Type:      epType,
			MaxPacket: desc.MaxPacketSize,
			Interval:  desc.Interval,
		})
	}
	return endpoints
}

// ── Window helpers ────────────────────────────────────────────────────────────

// createMessageWindow registers a minimal window class and creates a
// message-only window (HWND_MESSAGE parent) to receive WM_DEVICECHANGE.
func createMessageWindow(wndProcCb uintptr) (uintptr, error) {
	className, _ := syscall.UTF16PtrFromString("dnode_devmon")
	instance, _, _ := procGetModuleHandleW.Call(0)

	wc := wndClassExW{
		WndProc:   wndProcCb,
		Instance:  uintptr(instance),
		ClassName: className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))

	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return 0, fmt.Errorf("RegisterClassExW: %w", err)
	}

	// HWND_MESSAGE = (HWND)(-3)
	hwndMessage := ^uintptr(0) - 2

	hwnd, _, err := procCreateWindowExW.Call(
		0,                              // dwExStyle
		uintptr(unsafe.Pointer(className)),
		0,                              // lpWindowName
		0,                              // dwStyle
		0, 0, 0, 0,                     // x, y, w, h
		hwndMessage,                    // hWndParent = HWND_MESSAGE
		0,                              // hMenu
		uintptr(instance),
		0,
	)
	if hwnd == 0 {
		return 0, fmt.Errorf("CreateWindowExW: %w", err)
	}
	return hwnd, nil
}

// destroyWindow posts WM_QUIT and destroys the window.
func destroyWindow(hwnd uintptr) {
	procPostQuitMessage.Call(0)    //nolint:errcheck
	procDestroyWindow.Call(hwnd)   //nolint:errcheck
}

// registerAllDeviceNotifications registers the window for all device
// interface arrival/removal events regardless of device class.
func registerAllDeviceNotifications(hwnd uintptr) (uintptr, error) {
	filter := devBroadcastDeviceInterface{
		DeviceType: dbtDevtypDeviceInterface,
	}
	filter.Size = uint32(unsafe.Sizeof(filter))

	handle, _, err := procRegisterDeviceNotificationW.Call(
		hwnd,
		uintptr(unsafe.Pointer(&filter)),
		deviceNotifyWindowHandle|deviceNotifyAllInterfaceClasses,
	)
	if handle == 0 {
		return 0, fmt.Errorf("RegisterDeviceNotificationW: %w", err)
	}
	return handle, nil
}

// messageLoop runs a standard Win32 message loop until WM_QUIT.
func messageLoop() error {
	var m msgW
	for {
		r, _, err := procGetMessageW.Call(
			uintptr(unsafe.Pointer(&m)),
			0, 0, 0,
		)
		// GetMessage returns 0 on WM_QUIT, -1 on error.
		if r == 0 {
			return nil
		}
		if int32(r) == -1 {
			return fmt.Errorf("GetMessageW: %w", err)
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))  //nolint:errcheck
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))  //nolint:errcheck
	}
}
