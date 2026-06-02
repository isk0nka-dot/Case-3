package scanner

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// USBDevice represents a USB device detected on the system.
type USBDevice struct {
	ID          string
	Vendor      string
	Product     string
	Class       string
	ConnectedAt time.Time
}

// ScanUSBDevices returns the list of currently connected USB devices.
func ScanUSBDevices() ([]USBDevice, error) {
	switch runtime.GOOS {
	case "linux":
		return scanUSBLinux()
	case "windows":
		return scanUSBWindows()
	default:
		return nil, nil
	}
}

func scanUSBLinux() ([]USBDevice, error) {
	// Read /sys/bus/usb/devices — each directory is a USB device or interface
	entries, err := os.ReadDir("/sys/bus/usb/devices")
	if err != nil {
		// Fallback to lsusb
		return scanUSBLsusb()
	}

	var devices []USBDevice
	for _, entry := range entries {
		name := entry.Name()
		// Skip interfaces (contain colon) and root hubs
		if strings.Contains(name, ":") || strings.HasPrefix(name, "usb") {
			continue
		}
		base := "/sys/bus/usb/devices/" + name

		vendor := readSysFile(base + "/idVendor")
		product := readSysFile(base + "/idProduct")
		productName := readSysFile(base + "/product")
		vendorName := readSysFile(base + "/manufacturer")
		class := readSysFile(base + "/bDeviceClass")

		if vendor == "" && product == "" {
			continue
		}

		devices = append(devices, USBDevice{
			ID:      name,
			Vendor:  coalesce(vendorName, vendor),
			Product: coalesce(productName, product),
			Class:   class,
		})
	}
	return devices, nil
}

func scanUSBLsusb() ([]USBDevice, error) {
	out, err := exec.Command("lsusb").Output()
	if err != nil {
		return nil, err
	}
	var devices []USBDevice
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: Bus 001 Device 002: ID 1234:5678 Product Name
		parts := strings.Fields(line)
		if len(parts) < 6 {
			continue
		}
		devices = append(devices, USBDevice{
			ID:      parts[1] + ":" + parts[3],
			Product: strings.Join(parts[6:], " "),
		})
	}
	return devices, nil
}

func scanUSBWindows() ([]USBDevice, error) {
	out, err := exec.Command("wmic", "path", "Win32_USBHub",
		"get", "DeviceID,Description", "/format:list").Output()
	if err != nil {
		return nil, err
	}
	var devices []USBDevice
	var current USBDevice
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if current.ID != "" {
				devices = append(devices, current)
				current = USBDevice{}
			}
			continue
		}
		if strings.HasPrefix(line, "DeviceID=") {
			current.ID = strings.TrimPrefix(line, "DeviceID=")
		}
		if strings.HasPrefix(line, "Description=") {
			current.Product = strings.TrimPrefix(line, "Description=")
		}
	}
	if current.ID != "" {
		devices = append(devices, current)
	}
	return devices, nil
}

// USBSnapshot is a fingerprint of connected USB devices at a point in time.
type USBSnapshot struct {
	Devices   []USBDevice
	TakenAt   time.Time
	DeviceIDs []string // sorted set of IDs for change detection
}

// NewUSBSnapshot captures a snapshot of current USB devices.
func NewUSBSnapshot() (USBSnapshot, error) {
	devices, err := ScanUSBDevices()
	snap := USBSnapshot{
		Devices: devices,
		TakenAt: time.Now(),
	}
	for _, d := range devices {
		snap.DeviceIDs = append(snap.DeviceIDs, d.ID)
	}
	return snap, err
}

// USBChange describes a USB device change between two snapshots.
type USBChange struct {
	Added   []USBDevice
	Removed []USBDevice
}

// Diff computes devices added/removed since a previous snapshot.
func (s USBSnapshot) Diff(prev USBSnapshot) USBChange {
	prevIDs := make(map[string]USBDevice, len(prev.Devices))
	for _, d := range prev.Devices {
		prevIDs[d.ID] = d
	}
	currIDs := make(map[string]USBDevice, len(s.Devices))
	for _, d := range s.Devices {
		currIDs[d.ID] = d
	}

	var change USBChange
	for id, d := range currIDs {
		if _, found := prevIDs[id]; !found {
			change.Added = append(change.Added, d)
		}
	}
	for id, d := range prevIDs {
		if _, found := currIDs[id]; !found {
			change.Removed = append(change.Removed, d)
		}
	}
	return change
}

func readSysFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
