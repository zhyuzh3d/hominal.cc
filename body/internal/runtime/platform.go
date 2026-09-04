package runtime

import (
	"os"
	"path/filepath"
)

// Platform is supplied by the target manifest, not inferred from an experiment number.
type PlatformConfig struct {
	Hostname       string `json:"hostname"`
	OS             string `json:"os"`
	DesktopService string `json:"desktop_service"`
	DataRoot       string `json:"data_root"`
	LifeRoot       string `json:"life_root"`
	DesktopHome    string `json:"desktop_home"`
	Service        string `json:"service"`
}

func runtimeSocketPath() string {
	if value := os.Getenv("HOMINAL_MENTOR_SOCKET"); value != "" {
		return value
	}
	return "/run/hominal/hominal.sock"
}

func platformCapabilities(request CognitiveRequest) map[string]any {
	p := request.Config.Platform
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	life := p.LifeRoot
	if life == "" {
		life = filepath.Join(cwd, "life")
	}
	return map[string]any{
		"platform":                map[string]any{"hostname": p.Hostname, "os": p.OS, "experiment_line": "20.0", "cognitive_core": request.Config.CognitiveCore},
		"process":                 map[string]any{"service": p.Service, "user": os.Getenv("USER"), "uid": os.Getuid(), "home": home, "working_directory": cwd, "administrator": os.Geteuid() == 0},
		"filesystem":              map[string]any{"read_write": true, "life_space": life, "desktop_home": p.DesktopHome, "software_install": "user environment; no host privilege escalation"},
		"organs":                  request.State.Body.Organs,
		"network_probe_reachable": request.State.Body.NetworkAvailable,
		"network_probe_scope":     "辅助探测只描述其目标，实际调用独立报告可用性。",
		"mentor_channel":          map[string]any{"available": true, "use": "交流、讨论、求助、分享"},
	}
}
