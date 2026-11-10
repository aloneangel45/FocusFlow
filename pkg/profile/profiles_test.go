package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfiles_DefaultsAndHermeticConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FOCUSFLOW_HOME", tmpDir)

	if GetConfigDir() != filepath.Clean(tmpDir) {
		t.Fatalf("GetConfigDir() = %q, want %q", GetConfigDir(), filepath.Clean(tmpDir))
	}

	dev := DevProfile()
	if dev.Name != "dev" || len(dev.BlockedDomains) == 0 {
		t.Errorf("unexpected dev profile: %+v", dev)
	}

	study := StudyProfile()
	if study.Name != "study" || study.DefaultWorkMinutes != 25 {
		t.Errorf("unexpected study profile: %+v", study)
	}

	hardcore := HardcoreProfile()
	if hardcore.Name != "hardcore" {
		t.Errorf("expected hardcore name, got %s", hardcore.Name)
	}

	fallback := LoadOrDefault("nonexistent")
	if fallback.Name != "study" {
		t.Errorf("expected fallback to study, got %s", fallback.Name)
	}
}

func TestProfiles_LegacyMigrationAndDenylistSanitization(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FOCUSFLOW_HOME", tmpDir)

	legacyJSON := `{
  "custom_sites": ["https://www.ExampleDistraction.com/feed", "127.0.0.1", "localhost"],
  "custom_apps": ["Battle.net", "explorer.exe", "python.exe"]
}`
	if err := os.WriteFile(GetConfigPath(), []byte(legacyJSON), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(cfg.CustomDomains) != 1 || cfg.CustomDomains[0] != "exampledistraction.com" {
		t.Fatalf("unexpected sanitized CustomDomains: %v", cfg.CustomDomains)
	}
	if len(cfg.CustomProcesses) != 1 || cfg.CustomProcesses[0] != "battle.net.exe" {
		t.Fatalf("unexpected sanitized CustomProcesses: %v", cfg.CustomProcesses)
	}

	merged := LoadOrDefault("dev")
	foundDomain := false
	for _, d := range merged.BlockedDomains {
		if d == "exampledistraction.com" {
			foundDomain = true
		}
	}
	if !foundDomain {
		t.Errorf("expected custom domain merged into dev profile")
	}
}
