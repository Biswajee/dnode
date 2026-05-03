# rose

A cross-platform USB and hardware device monitor. Detects when devices are attached or removed, logs the event with full device details, and sends a desktop notification.

Runs on Windows and Linux.

---

## Usage

```
rose [flags]

  -monitor        Render a live device tree on the terminal
  -log <path>     Log file path  (default: rose.log)
  -no-notify      Disable desktop notifications
  -help           Show usage
```

**Log only (background / silent mode)**
```
rose
```

**Live terminal tree**
```
rose -monitor
```

**Custom log path, no notifications**
```
rose -monitor -log C:\logs\devices.log -no-notify
```

Press `Ctrl-C` to stop.

---

## Monitor mode

When `-monitor` is set, the terminal shows a live-updating device tree.
New devices are highlighted green. Removed devices flash red briefly before
disappearing. A recent-events log is shown at the bottom.

```
  rose — device monitor   2026-05-03  14:22:01
  ────────────────────────────────────────────────────────────────

  ├── VID:0403 PID:6001  FTDI USB Serial Converter  COM3
  │   │  manufacturer    FTDI
  │   │  product         USB Serial Converter
  │   │  serial          FT1ABC23
  │   │  port            COM3
  │   │  endpoints
  │   │    ├── EP0x81  IN   Bulk        64B
  │   │    └── EP0x02  OUT  Bulk        64B
  │
  └── VID:05AC PID:0250  Apple USB Mouse
        │  endpoints
        │    └── EP0x81  IN   Interrupt   4B  interval=8

  ────────────────────────────────────────────────────────────────
  recent events
  14:22:01  [+]  ATTACHED  FTDI USB Serial Converter  COM3
  14:21:45  [+]  ATTACHED  Apple USB Mouse
```

---

## Log file

Each event is written as a header line followed by indented detail lines.
The log accumulates across runs (append mode).

```
2026-05-03T14:22:01Z  ATTACHED  VID:0403 PID:6001  FTDI USB Serial Converter (COM3)
  manufacturer  : FTDI
  product       : USB Serial Converter
  serial        : FT1ABC23
  port          : COM3
  endpoints     : EP0x81  IN   Bulk        64B
                  EP0x02  OUT  Bulk        64B

2026-05-03T14:30:45Z  REMOVED   VID:0403 PID:6001  FTDI USB Serial Converter (COM3)
  manufacturer  : FTDI
  product       : USB Serial Converter
  serial        : FT1ABC23
  port          : COM3
```

Removed events are served from the in-memory cache populated at attach time,
so all fields are preserved even after the OS has torn down the device.

---

## Desktop notifications

One notification is sent per physical device attach or remove event.
The notification contains the device name and COM port (if applicable).
Endpoint details are written to the log but do not appear in notifications.

**Windows** — uses `Windows.UI.Notifications` via a PowerShell one-liner.
No WinRT dependency in the binary.

**Linux** — uses `notify-send` (provided by `libnotify-bin`).
If `notify-send` is not installed, notifications are silently skipped.

---

## Device detail

| Field | Source |
|---|---|
| VID / PID | Parsed from Windows device interface path or Linux uevent |
| Friendly name | Windows registry `FriendlyName` / Linux `ID_MODEL` |
| Manufacturer | Windows registry `Mfg` / Linux sysfs `manufacturer` |
| Product | Windows registry `DeviceDesc` / Linux sysfs `product` |
| Serial number | Embedded in Windows device instance ID / Linux sysfs `serial` |
| COM port | Windows: device tree walk via cfgmgr32 for `PortName` registry value (handles FTDI virtual bus and USB CDC ACM) / Linux: sysfs tty node walk |
| Endpoints | Windows: `IOCTL_USB_GET_NODE_CONNECTION_INFORMATION_EX` via parent hub / Linux: sysfs `ep_*` directories |

---

## Build

Requires [Go 1.22+](https://go.dev/dl/).

```bash
go mod tidy

# Windows binary
go build -o rose.exe .

# Linux binary (cross-compile from Windows)
GOOS=linux GOARCH=amd64 go build -o rose .
```

**Dependencies**

| Module | Maintained by | Used for |
|---|---|---|
| `golang.org/x/sys` | Go team | Windows message loop, cfgmgr32, ANSI console mode; Linux netlink constants |

---

## Platform notes

**Windows** — rose creates a hidden message-only window (`HWND_MESSAGE`) and
registers for `WM_DEVICECHANGE` events with `DEVICE_NOTIFY_ALL_INTERFACE_CLASSES`.
Run as a normal user; no elevated privileges required for monitoring.

**Linux** — rose opens a `NETLINK_KOBJECT_UEVENT` socket and reads kernel uevent
messages directly. No `udevadm` or `libudev` dependency. Requires access to the
netlink socket (standard for normal users on desktop distributions). The ANSI
terminal tree works in any VT100-compatible terminal.
