package scanner

import "testing"

func TestClassifyRemoteAccessProcesses(t *testing.T) {
	procs := []ProcessInfo{
		{PID: "100", Name: "AnyDesk.exe", Cmd: "C:\\Program Files\\AnyDesk\\AnyDesk.exe"},
		{PID: "101", Name: "rustdesk.exe", Cmd: "rustdesk.exe --service"},
		{PID: "102", Name: "chrome.exe", Cmd: "chrome.exe"},
	}
	got := ClassifyRemoteAccessProcesses(procs)
	if len(got) != 2 {
		t.Fatalf("expected 2 remote-access tools, got %d: %+v", len(got), got)
	}
	names := map[string]bool{}
	for _, tool := range got {
		names[tool.Name] = true
		if tool.Active {
			t.Errorf("process-only detection must not be Active: %+v", tool)
		}
	}
	if !names["AnyDesk"] || !names["RustDesk"] {
		t.Errorf("expected AnyDesk and RustDesk, got %v", names)
	}
}

func TestIsRemoteAccessProcess(t *testing.T) {
	if !IsRemoteAccessProcess(ProcessInfo{Name: "TeamViewer.exe"}) {
		t.Error("TeamViewer should be classified as remote access")
	}
	if IsRemoteAccessProcess(ProcessInfo{Name: "obs64.exe"}) {
		t.Error("OBS is a forbidden app but not remote access")
	}
}

func TestDetectRemoteAccessPortsActive(t *testing.T) {
	// AnyDesk port 7070 ESTABLISHED => active session; VNC 5900 LISTEN => idle.
	sample := "" +
		"  TCP    0.0.0.0:7070      93.184.216.34:51000   ESTABLISHED     1234\n" +
		"  TCP    0.0.0.0:5900      0.0.0.0:0             LISTENING       2222\n"
	conns := parseNetstatWindows(sample)
	tools := DetectRemoteAccessPorts(conns)

	var anydesk, vnc *RemoteAccessTool
	for i := range tools {
		switch tools[i].Name {
		case "AnyDesk":
			anydesk = &tools[i]
		case "VNC":
			vnc = &tools[i]
		}
	}
	if anydesk == nil || !anydesk.Active {
		t.Errorf("AnyDesk should be detected as active, got %+v", anydesk)
	}
	if vnc == nil || vnc.Active {
		t.Errorf("VNC should be detected as listening (not active), got %+v", vnc)
	}
	if !AnyActiveRemoteAccess(tools) {
		t.Error("AnyActiveRemoteAccess should be true when AnyDesk is established")
	}
}

func TestMergeRemoteAccessPrefersActive(t *testing.T) {
	proc := []RemoteAccessTool{{Name: "AnyDesk", Via: "process", Active: false}}
	net := []RemoteAccessTool{{Name: "AnyDesk", Via: "network", Active: true}}
	merged := MergeRemoteAccess(proc, net)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged tool, got %d", len(merged))
	}
	if !merged[0].Active {
		t.Errorf("merge should prefer the active network signal, got %+v", merged[0])
	}
}
