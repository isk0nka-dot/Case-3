// Package scanner provides cross-platform system scanning for the Argus
// desktop agent: process detection, VM checks, monitor count, hardware ID.
package scanner

import (
	"os/exec"
	"runtime"
	"strings"
)

// ProcessInfo holds basic info about a running process.
type ProcessInfo struct {
	PID  string
	Name string
	Cmd  string
}

// ScanProcesses returns the list of running processes on the current OS.
// Uses `ps aux` on Linux/macOS and `tasklist` on Windows.
func ScanProcesses() ([]ProcessInfo, error) {
	switch runtime.GOOS {
	case "windows":
		return scanProcessesWindows()
	default:
		return scanProcessesUnix()
	}
}

func scanProcessesUnix() ([]ProcessInfo, error) {
	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		return nil, err
	}
	return parseProcessesUnix(string(out)), nil
}

func parseProcessesUnix(output string) []ProcessInfo {
	lines := strings.Split(output, "\n")
	procs := make([]ProcessInfo, 0, len(lines))
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // skip header
		}
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}
		cmd := strings.Join(fields[10:], " ")
		name := fields[10]
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		procs = append(procs, ProcessInfo{
			PID:  fields[1],
			Name: name,
			Cmd:  cmd,
		})
	}
	return procs
}

func scanProcessesWindows() ([]ProcessInfo, error) {
	out, err := exec.Command("tasklist", "/fo", "csv", "/nh").Output()
	if err != nil {
		// Fall back to simple tasklist
		out2, err2 := exec.Command("tasklist").Output()
		if err2 != nil {
			return nil, err
		}
		return parseProcessesWindowsSimple(string(out2)), nil
	}
	return parseProcessesWindowsCSV(string(out)), nil
}

func parseProcessesWindowsCSV(output string) []ProcessInfo {
	lines := strings.Split(output, "\n")
	procs := make([]ProcessInfo, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// CSV: "Image Name.exe","PID","Session Name","Session#","Mem Usage"
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		name := strings.Trim(parts[0], `"`)
		pid := strings.Trim(parts[1], `"`)
		procs = append(procs, ProcessInfo{PID: pid, Name: name, Cmd: name})
	}
	return procs
}

func parseProcessesWindowsSimple(output string) []ProcessInfo {
	lines := strings.Split(output, "\n")
	procs := make([]ProcessInfo, 0)
	for i, line := range lines {
		if i < 3 { // skip header lines
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			procs = append(procs, ProcessInfo{Name: fields[0], Cmd: line})
		}
	}
	return procs
}

// FindForbiddenProcesses checks if any running process matches the forbidden list.
// Match is case-insensitive substring. Returns matching processes.
func FindForbiddenProcesses(procs []ProcessInfo, forbidden []string) []ProcessInfo {
	var found []ProcessInfo
	for _, proc := range procs {
		nameLower := strings.ToLower(proc.Name)
		cmdLower := strings.ToLower(proc.Cmd)
		for _, f := range forbidden {
			fLower := strings.ToLower(f)
			if strings.Contains(nameLower, fLower) || strings.Contains(cmdLower, fLower) {
				found = append(found, proc)
				break
			}
		}
	}
	return found
}
