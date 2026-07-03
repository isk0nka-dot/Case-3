package scanner

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// NetConnection is a parsed TCP connection with just the fields we need to
// reason about remote-access activity.
type NetConnection struct {
	LocalPort  int
	RemotePort int
	State      string // uppercased: "LISTEN", "ESTABLISHED", …
}

// ScanConnections lists TCP connections on the current OS.
// Windows: `netstat -ano -p tcp`; Unix: `ss -tan` with a `netstat -tan` fallback.
// Errors are non-fatal to the caller — an empty slice simply means the
// network signal is unavailable and detection falls back to process scanning.
func ScanConnections() ([]NetConnection, error) {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("netstat", "-ano", "-p", "tcp").Output()
		if err != nil {
			return nil, err
		}
		return parseNetstatWindows(string(out)), nil
	default:
		if out, err := exec.Command("ss", "-tan").Output(); err == nil {
			return parseSS(string(out)), nil
		}
		out, err := exec.Command("netstat", "-tan").Output()
		if err != nil {
			return nil, err
		}
		return parseNetstatUnix(string(out)), nil
	}
}

// portOf extracts the port from an "addr:port" or "[::1]:port" token.
func portOf(addr string) int {
	if i := strings.LastIndex(addr, ":"); i >= 0 && i < len(addr)-1 {
		if p, err := strconv.Atoi(addr[i+1:]); err == nil {
			return p
		}
	}
	return 0
}

func parseNetstatWindows(out string) []NetConnection {
	var conns []NetConnection
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		// Proto Local Foreign State PID  → e.g. "TCP 0.0.0.0:7070 0.0.0.0:0 LISTENING 1234"
		if len(f) < 4 || !strings.EqualFold(f[0], "TCP") {
			continue
		}
		conns = append(conns, NetConnection{
			LocalPort:  portOf(f[1]),
			RemotePort: portOf(f[2]),
			State:      normalizeState(f[3]),
		})
	}
	return conns
}

func parseSS(out string) []NetConnection {
	var conns []NetConnection
	for i, line := range strings.Split(out, "\n") {
		if i == 0 { // header: "State Recv-Q Send-Q Local Address:Port Peer Address:Port"
			continue
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		conns = append(conns, NetConnection{
			State:      normalizeState(f[0]),
			LocalPort:  portOf(f[3]),
			RemotePort: portOf(f[4]),
		})
	}
	return conns
}

func parseNetstatUnix(out string) []NetConnection {
	var conns []NetConnection
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		// Proto Recv-Q Send-Q Local Foreign State → 6 columns for tcp
		if len(f) < 6 || !strings.HasPrefix(strings.ToLower(f[0]), "tcp") {
			continue
		}
		conns = append(conns, NetConnection{
			LocalPort:  portOf(f[3]),
			RemotePort: portOf(f[4]),
			State:      normalizeState(f[5]),
		})
	}
	return conns
}

func normalizeState(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "LISTENING" { // Windows spells it LISTENING; normalize to LISTEN
		return "LISTEN"
	}
	return s
}

// DetectRemoteAccessPorts matches connections against the port registry.
// An ESTABLISHED connection on a tool's port is treated as an *active* remote
// session; a LISTEN-only match means the tool is running but idle.
func DetectRemoteAccessPorts(conns []NetConnection) []RemoteAccessTool {
	// Build port -> tool index once.
	portTool := map[int]string{}
	for _, sig := range RemoteAccessSignatures {
		for _, p := range sig.Ports {
			portTool[p] = sig.Name
		}
	}

	// Track the strongest state seen per tool.
	active := map[string]bool{}
	listen := map[string]bool{}
	for _, c := range conns {
		name := ""
		if t, ok := portTool[c.LocalPort]; ok {
			name = t
		} else if t, ok := portTool[c.RemotePort]; ok {
			name = t
		}
		if name == "" {
			continue
		}
		if c.State == "ESTABLISHED" {
			active[name] = true
		} else if c.State == "LISTEN" {
			listen[name] = true
		}
	}

	var tools []RemoteAccessTool
	emit := func(name string, isActive bool) {
		state := "listening"
		if isActive {
			state = "established"
		}
		tools = append(tools, RemoteAccessTool{
			Name:   name,
			Via:    "network",
			Detail: "port state=" + state,
			Active: isActive,
		})
	}
	for name := range active {
		emit(name, true)
	}
	for name := range listen {
		if !active[name] {
			emit(name, false)
		}
	}
	return tools
}
