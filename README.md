# dnode

[![CI](https://github.com/biswajee/dnode/actions/workflows/ci.yml/badge.svg)](https://github.com/biswajee/dnode/actions/workflows/ci.yml)
[![Release](https://github.com/biswajee/dnode/actions/workflows/release.yml/badge.svg)](https://github.com/biswajee/dnode/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/biswajee/dnode)](https://goreportcard.com/report/github.com/biswajee/dnode)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A cross-platform USB and hardware device monitor. Detects device attach and remove events in real time, logs full device details, and renders a live terminal tree.

Runs on **Windows** and **Linux** with no elevated privileges and no external runtime dependencies.

## Installation

### Download a binary

Grab the latest release for your platform from the [Releases](https://github.com/biswajee/dnode/releases) page.

| Platform | Binary |
|---|---|
| Windows | `dnode.exe` |
| Linux | `dnode` |

### Build from source

Requires [Go 1.22+](https://go.dev/dl/).

```bash
git clone https://github.com/biswajee/dnode.git
cd dnode
go build -o dnode .          # Linux
go build -o dnode.exe .      # Windows
```

## Usage

```
dnode [flags]

Flags:
  -monitor        Render a live device tree on the terminal
  -v              Minimal output: ID, name, port
  -vv             Standard output: + manufacturer, product, serial  (default)
  -vvv            Verbose output: + USB endpoints
  -log <path>     Log file path  (default: dnode.log)
  -help           Show this message
```

### Examples

| Command | Description |
|---|---|
| `dnode` | Log only, silent background mode |
| `dnode -monitor` | Live terminal tree, default verbosity |
| `dnode -monitor -v` | Live terminal tree, minimal output |
| `dnode -monitor -vvv` | Live terminal tree with USB endpoint detail |
| `dnode -monitor -v -log /var/log/dnode.log` | Minimal display, custom log path |

Press `Ctrl-C` to stop.

## Monitor output

```
  dnode - device monitor   2026-06-09  14:22:01
  ────────────────────────────────────────────────────────────────

  ├── VID:0403 PID:6001  FTDI USB Serial Converter  COM3
  │   │  manufacturer    FTDI
  │   │  product         USB Serial Converter
  │   │  serial          FT1ABC23
  │   │  port            COM3
  │   │  endpoints
  │   │    ├── EP0x81  IN   Bulk          64B
  │   │    └── EP0x02  OUT  Bulk          64B
  │
  └── VID:05AC PID:0250  Apple USB Mouse
        │  manufacturer  Apple Inc.
        │  endpoints
        │    └── EP0x81  IN   Interrupt   4B  interval=8

  ────────────────────────────────────────────────────────────────
  recent events
  14:22:01  [+]  ATTACHED  FTDI USB Serial Converter  COM3
  14:21:45  [+]  ATTACHED  Apple USB Mouse
```

New devices are highlighted **green**. Removed devices flash **red** briefly before disappearing.

| Flag | Tree shows | Footer shows |
|---|---|---|
| `-v` | ID, name, port | symbol + name |
| `-vv` | + manufacturer, product, serial | + timestamp, action, port |
| `-vvv` | + USB endpoints | + timestamp, action, port |

## Log file

Events are written in append mode and survive restarts. Remove events include the full detail captured at attach time.

```
2026-06-09T14:22:01Z  ATTACHED  VID:0403 PID:6001  FTDI USB Serial Converter (COM3)
  manufacturer  : FTDI
  product       : USB Serial Converter
  serial        : FT1ABC23
  port          : COM3
  endpoints     : EP0x81  IN   Bulk   64B
                  EP0x02  OUT  Bulk   64B

2026-06-09T14:30:45Z  REMOVED   VID:0403 PID:6001  FTDI USB Serial Converter (COM3)
  manufacturer  : FTDI
  product       : USB Serial Converter
  serial        : FT1ABC23
  port          : COM3
```

## Device fields

| Field | Windows source | Linux source |
|---|---|---|
| VID / PID | Device interface path | `ID_VENDOR_ID` / `ID_MODEL_ID` uevent keys, sysfs fallback |
| Name | Registry `FriendlyName` | `ID_MODEL` uevent key |
| Manufacturer | Registry `Mfg` | sysfs `manufacturer` |
| Product | Registry `DeviceDesc` | sysfs `product` |
| Serial | Device instance ID | sysfs `serial` |
| Port | cfgmgr32 device tree walk for `PortName` | sysfs tty node walk |
| Endpoints | `IOCTL_USB_GET_NODE_CONNECTION_INFORMATION_EX` via parent hub | sysfs `ep_*` directories |

## Platform notes

**Windows** - dnode creates a hidden message-only window (`HWND_MESSAGE`) and registers for `WM_DEVICECHANGE` with `DEVICE_NOTIFY_ALL_INTERFACE_CLASSES`. Runs as a normal user. Devices without a driver bound (shown as "Unknown Device" in Device Manager) do not fire `WM_DEVICECHANGE` and will not be detected.

**Linux** - dnode opens a `NETLINK_KOBJECT_UEVENT` socket and reads kernel uevent messages directly. No udevadm or libudev dependency. Works for driverless devices; VID/PID and name fields are populated from sysfs when udev keys are absent.

## Contributing

1. Fork the repository and create a `feature/your-feature` branch.
2. Make your changes and ensure `go test ./...` passes.
3. Open a pull request against `develop`.

CI runs lint (`golangci-lint`) and tests on both Ubuntu and Windows for every push and pull request.

## License

[MIT](LICENSE)
