package scanner

import "strings"

// RemoteAccessTool describes a detected remote-control / screen-sharing tool.
//
// Unlike the generic forbidden-process list, this focuses specifically on tools
// that let a third party operate the student's machine (AnyDesk, RustDesk,
// TeamViewer, VNC, …). `Active` is true when there is evidence of a *live*
// remote session (an established connection on the tool's port), as opposed to
// the application merely being installed or idle.
type RemoteAccessTool struct {
	Name   string `json:"name"`   // canonical tool name, e.g. "AnyDesk"
	Via    string `json:"via"`    // "process" | "network"
	Detail string `json:"detail"` // pid/name or port context
	Active bool   `json:"active"` // established remote session likely in progress
}

// remoteAccessSignature ties a canonical tool to the process-name fragments and
// TCP ports that indicate its presence. Ports are used to distinguish an active
// remote session (ESTABLISHED) from an idle/listening install (LISTEN).
type remoteAccessSignature struct {
	Name  string
	Procs []string
	Ports []int
}

// RemoteAccessSignatures is the curated registry. It is intentionally broad on
// process names and precise on the ports that imply an active session.
var RemoteAccessSignatures = []remoteAccessSignature{
	{"AnyDesk", []string{"anydesk"}, []int{7070}},
	{"RustDesk", []string{"rustdesk"}, []int{21115, 21116, 21117, 21118, 21119}},
	{"TeamViewer", []string{"teamviewer", "tv_w32", "tv_x64"}, []int{5938}},
	{"VNC", []string{"winvnc", "tvnserver", "vncserver", "vncviewer", "uvnc", "tigervnc", "ultravnc", "tightvnc", "realvnc"}, []int{5900, 5901, 5902, 5903}},
	{"Chrome Remote Desktop", []string{"remoting_host", "remote_assistance_host", "chrome remote desktop"}, nil},
	{"Parsec", []string{"parsec"}, nil},
	{"AmmyyAdmin", []string{"ammyy"}, nil},
	{"Supremo", []string{"supremo"}, nil},
	{"Splashtop", []string{"splashtop"}, nil},
	{"NoMachine", []string{"nomachine", "nxnode", "nxserver"}, nil},
	{"RemotePC", []string{"remotepc"}, nil},
	{"Radmin", []string{"radmin", "rserver3"}, nil},
	{"DWAgent", []string{"dwagent"}, nil},
	{"LiteManager", []string{"romserver", "romfusclient", "litemanager"}, nil},
}

// ClassifyRemoteAccessProcesses returns the remote-access tools found among the
// running processes. Detection is a case-insensitive substring match on the
// process name or command line (mirrors FindForbiddenProcesses semantics).
func ClassifyRemoteAccessProcesses(procs []ProcessInfo) []RemoteAccessTool {
	var found []RemoteAccessTool
	seen := map[string]bool{}
	for _, proc := range procs {
		name := strings.ToLower(proc.Name)
		cmd := strings.ToLower(proc.Cmd)
		for _, sig := range RemoteAccessSignatures {
			for _, frag := range sig.Procs {
				if strings.Contains(name, frag) || strings.Contains(cmd, frag) {
					key := sig.Name + "|proc"
					if !seen[key] {
						seen[key] = true
						found = append(found, RemoteAccessTool{
							Name:   sig.Name,
							Via:    "process",
							Detail: "process=" + proc.Name + " pid=" + proc.PID,
							Active: false, // process presence alone is not proof of a live session
						})
					}
					break
				}
			}
		}
	}
	return found
}

// IsRemoteAccessProcess reports whether a process name/command matches any
// remote-access signature. Used to avoid double-reporting a remote-access tool
// as both REMOTE_ACCESS_DETECTED and the generic FORBIDDEN_PROCESS_DETECTED.
func IsRemoteAccessProcess(p ProcessInfo) bool {
	name := strings.ToLower(p.Name)
	cmd := strings.ToLower(p.Cmd)
	for _, sig := range RemoteAccessSignatures {
		for _, frag := range sig.Procs {
			if strings.Contains(name, frag) || strings.Contains(cmd, frag) {
				return true
			}
		}
	}
	return false
}

// MergeRemoteAccess combines process- and network-derived detections, keeping
// the strongest signal per tool (an Active network hit wins over a passive
// process hit).
func MergeRemoteAccess(a, b []RemoteAccessTool) []RemoteAccessTool {
	byName := map[string]RemoteAccessTool{}
	order := []string{}
	add := func(t RemoteAccessTool) {
		if cur, ok := byName[t.Name]; ok {
			if t.Active && !cur.Active {
				byName[t.Name] = t
			}
			return
		}
		byName[t.Name] = t
		order = append(order, t.Name)
	}
	for _, t := range a {
		add(t)
	}
	for _, t := range b {
		add(t)
	}
	out := make([]RemoteAccessTool, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out
}

// AnyActiveRemoteAccess reports whether any detected tool has a live session.
func AnyActiveRemoteAccess(tools []RemoteAccessTool) bool {
	for _, t := range tools {
		if t.Active {
			return true
		}
	}
	return false
}
