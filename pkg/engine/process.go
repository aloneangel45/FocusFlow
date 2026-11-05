package engine

import (
	"strings"
)

type ProcessSupervisor struct {
	blockedNames map[string]struct{}
}

func NewProcessSupervisor(blocked []string) *ProcessSupervisor {
	m := make(map[string]struct{})
	for _, b := range blocked {
		trimmed := strings.ToLower(strings.TrimSpace(b))
		if trimmed == "" {
			continue
		}
		m[trimmed] = struct{}{}
		// Support cross-platform matching with and without .exe
		if strings.HasSuffix(trimmed, ".exe") {
			m[strings.TrimSuffix(trimmed, ".exe")] = struct{}{}
		} else {
			m[trimmed+".exe"] = struct{}{}
		}
	}
	return &ProcessSupervisor{blockedNames: m}
}

// KillProcessesByNames terminates running processes matching any of the specified names.
func KillProcessesByNames(names []string) error {
	sup := NewProcessSupervisor(names)
	_ = sup.ScanAndTerminate()
	return nil
}

// KillProcessesByNames terminates processes for this supervisor.
func (p *ProcessSupervisor) KillProcessesByNames(names []string) error {
	sup := NewProcessSupervisor(names)
	_ = sup.ScanAndTerminate()
	return nil
}
