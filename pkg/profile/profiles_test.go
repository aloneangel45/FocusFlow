package profile

import "testing"

func TestProfiles_Defaults(t *testing.T) {
	dev := DevProfile()
	if dev.Name != "dev" {
		t.Errorf("expected dev name, got %s", dev.Name)
	}
	if len(dev.BlockedDomains) == 0 {
		t.Errorf("expected blocked domains in dev profile")
	}

	study := StudyProfile()
	if study.Name != "study" {
		t.Errorf("expected study name, got %s", study.Name)
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
