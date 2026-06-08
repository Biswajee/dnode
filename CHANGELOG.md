# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

---

## [0.1.0] - 2026-06-09

### Added
- Real-time USB and hardware device attach/remove detection on Windows and Linux
- Live terminal device tree with ANSI colour highlights (`-monitor` flag)
- Three verbosity levels for monitor display: `-v` (minimal), `-vv` (standard, default), `-vvv` (verbose with USB endpoints)
- Structured append-mode log file with full device metadata (`-log` flag)
- USB descriptor detail: endpoints, transfer types, max packet size, polling interval
- Serial/COM port discovery via device tree walk (handles FTDI virtual bus and USB CDC ACM)
- In-memory device cache so remove events retain all fields captured at attach time
- Windows: `WM_DEVICECHANGE` event loop via hidden `HWND_MESSAGE` window - no elevated privileges required
- Linux: raw `NETLINK_KOBJECT_UEVENT` socket - no udevadm or libudev dependency
- Cross-compiled release binaries for Windows (`dnode.exe`) and Linux (`dnode`)
- GitHub Actions CI pipeline: lint, test, and coverage gate (≥ 70%) on push to `main`, `develop`, and `feature/**` branches
- GitHub Actions release pipeline: builds and publishes binaries on version tags pushed to `main`

[Unreleased]: https://github.com/biswajee/dnode/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/biswajee/dnode/releases/tag/v0.1.0
