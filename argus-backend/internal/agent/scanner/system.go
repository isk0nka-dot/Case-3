package scanner

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// SystemInfo holds hardware and environment info collected by the agent.
type SystemInfo struct {
	Hostname      string
	MachineID     string // /etc/machine-id on Linux, machine GUID on Windows
	OS            string
	Arch          string
	MonitorCount  int
	IsVM          bool
	VMProduct     string // empty if not VM
	CPUInfo       string // first cpu entry summary
}

// CollectSystemInfo gathers system information.
func CollectSystemInfo() SystemInfo {
	info := SystemInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	info.Hostname, _ = os.Hostname()
	info.MachineID = getMachineID()
	info.MonitorCount = getMonitorCount()
	info.IsVM, info.VMProduct = detectVM()
	info.CPUInfo = getCPUSummary()

	return info
}

// getMachineID returns a stable machine identifier.
// Uses /etc/machine-id on Linux, hostname on others as fallback.
func getMachineID() string {
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/etc/machine-id"); err == nil {
			return strings.TrimSpace(string(data))
		}
		if data, err := os.ReadFile("/var/lib/dbus/machine-id"); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	if runtime.GOOS == "windows" {
		out, err := exec.Command("reg", "query",
			`HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Cryptography`,
			"/v", "MachineGuid").Output()
		if err == nil {
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				if strings.Contains(line, "MachineGuid") {
					parts := strings.Fields(line)
					if len(parts) >= 3 {
						return parts[len(parts)-1]
					}
				}
			}
		}
	}
	h, _ := os.Hostname()
	return h
}

// getMonitorCount detects the number of connected monitors.
func getMonitorCount() int {
	switch runtime.GOOS {
	case "linux":
		return getMonitorCountLinux()
	case "windows":
		return getMonitorCountWindows()
	default:
		return 1
	}
}

func getMonitorCountLinux() int {
	// Try xrandr first
	out, err := exec.Command("xrandr").Output()
	if err == nil {
		count := 0
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, " connected") {
				count++
			}
		}
		if count > 0 {
			return count
		}
	}
	// Fallback: count connected DRM connectors
	drm := "/sys/class/drm"
	entries, err := os.ReadDir(drm)
	if err == nil {
		count := 0
		for _, e := range entries {
			statusFile := drm + "/" + e.Name() + "/status"
			data, err := os.ReadFile(statusFile)
			if err == nil && strings.TrimSpace(string(data)) == "connected" {
				count++
			}
		}
		if count > 0 {
			return count
		}
	}
	return 1
}

func getMonitorCountWindows() int {
	// Use wmic to count monitors
	out, err := exec.Command("wmic", "path", "Win32_DesktopMonitor", "get", "DeviceID", "/format:list").Output()
	if err == nil {
		count := 0
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "DeviceID=") {
				count++
			}
		}
		if count > 0 {
			return count
		}
	}
	return 1
}

// detectVM checks common indicators that the system is running in a VM.
func detectVM() (bool, string) {
	switch runtime.GOOS {
	case "linux":
		return detectVMLinux()
	case "windows":
		return detectVMWindows()
	default:
		return false, ""
	}
}

func detectVMLinux() (bool, string) {
	// Check DMI product name
	dmis := []string{
		"/sys/class/dmi/id/product_name",
		"/sys/class/dmi/id/sys_vendor",
		"/sys/class/dmi/id/board_vendor",
	}
	vmKeywords := map[string]string{
		"vmware":       "VMware",
		"virtualbox":   "VirtualBox",
		"kvm":          "KVM",
		"qemu":         "QEMU",
		"xen":          "Xen",
		"hyperv":       "Hyper-V",
		"hyper-v":      "Hyper-V",
		"microsoft corporation": "Hyper-V",
		"parallels":    "Parallels",
		"bhyve":        "bhyve",
		"innotek":      "VirtualBox",
	}
	for _, dmiPath := range dmis {
		data, err := os.ReadFile(dmiPath)
		if err != nil {
			continue
		}
		lower := strings.ToLower(strings.TrimSpace(string(data)))
		for keyword, product := range vmKeywords {
			if strings.Contains(lower, keyword) {
				return true, product
			}
		}
	}
	// Check /proc/cpuinfo for hypervisor flag
	data, err := os.ReadFile("/proc/cpuinfo")
	if err == nil {
		if strings.Contains(string(data), "hypervisor") {
			return true, "Unknown VM"
		}
	}
	return false, ""
}

func detectVMWindows() (bool, string) {
	// Check BIOS/manufacturer via WMIC
	out, err := exec.Command("wmic", "computersystem", "get", "Manufacturer,Model", "/format:list").Output()
	if err == nil {
		lower := strings.ToLower(string(out))
		vmKeywords := map[string]string{
			"vmware":   "VMware",
			"virtual":  "VM",
			"kvm":      "KVM",
			"qemu":     "QEMU",
			"xen":      "Xen",
			"vbox":     "VirtualBox",
			"parallels": "Parallels",
		}
		for k, v := range vmKeywords {
			if strings.Contains(lower, k) {
				return true, v
			}
		}
	}
	return false, ""
}

// getCPUSummary returns a short CPU identification string.
func getCPUSummary() string {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/cpuinfo")
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "model name") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						return strings.TrimSpace(parts[1])
					}
				}
			}
		}
	}
	return runtime.GOOS + "/" + runtime.GOARCH
}
