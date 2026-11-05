package engine

import (
	"testing"
)

func TestProcessSupervisor_Normalization(t *testing.T) {
	blocked := []string{"Discord.exe", "spotify", " STEAM.EXE "}
	sup := NewProcessSupervisor(blocked)

	expectedKeys := []string{"discord.exe", "discord", "spotify", "spotify.exe", "steam.exe", "steam"}
	for _, key := range expectedKeys {
		if _, exists := sup.blockedNames[key]; !exists {
			t.Errorf("expected %q to be in blockedNames map", key)
		}
	}
}

func TestProcessSupervisor_ScanAndTerminate_Empty(t *testing.T) {
	sup := NewProcessSupervisor(nil)
	terminated := sup.ScanAndTerminate()
	if len(terminated) != 0 {
		t.Errorf("expected 0 terminated processes for empty supervisor, got %d", len(terminated))
	}

	err := KillProcessesByNames(nil)
	if err != nil {
		t.Errorf("expected nil error for empty KillProcessesByNames, got %v", err)
	}
}

func TestProcessSupervisor_ScanAndTerminate_NonExistent(t *testing.T) {
	sup := NewProcessSupervisor([]string{"nonexistent_process_123456789.exe"})
	terminated := sup.ScanAndTerminate()
	if len(terminated) != 0 {
		t.Errorf("expected 0 terminated processes for nonexistent process, got %d", len(terminated))
	}
}
