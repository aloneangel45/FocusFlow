package profile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aloneangel45/focusflow/pkg/engine"
	"github.com/aloneangel45/focusflow/pkg/schedule"
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
	if cfg.DailyGoalMinutes != 120 {
		t.Errorf("expected omitted daily_goal_minutes to default to 120, got %d", cfg.DailyGoalMinutes)
	}
}

func TestProfiles_DailyGoalClamping(t *testing.T) {
	if got := NormalizeDailyGoalMinutes(0); got != 120 {
		t.Errorf("NormalizeDailyGoalMinutes(0) = %d, want 120", got)
	}
	if got := NormalizeDailyGoalMinutes(-30); got != 120 {
		t.Errorf("NormalizeDailyGoalMinutes(-30) = %d, want 120", got)
	}
	if got := NormalizeDailyGoalMinutes(2); got != 5 {
		t.Errorf("NormalizeDailyGoalMinutes(2) = %d, want 5", got)
	}
	if got := NormalizeDailyGoalMinutes(5000); got != 1440 {
		t.Errorf("NormalizeDailyGoalMinutes(5000) = %d, want 1440", got)
	}
	if got := NormalizeDailyGoalMinutes(180); got != 180 {
		t.Errorf("NormalizeDailyGoalMinutes(180) = %d, want 180", got)
	}
}

func TestProfiles_CustomProfileCRUDAndValidation(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FOCUSFLOW_HOME", tmpDir)

	// Invalid slugs
	badSlugs := []string{"", "-lead", "_lead", "Upper", "with space", "bad/slash", strings.Repeat("a", 33)}
	for _, bad := range badSlugs {
		if _, err := ValidateProfileSlug(bad); err == nil {
			t.Errorf("expected slug %q to be rejected", bad)
		}
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Refuse overwriting built-in profiles
	for _, b := range []string{"study", "dev", "hardcore"} {
		if _, _, err := cfg.UpsertCustomProfile(CustomProfileInput{Name: b}); !errors.Is(err, ErrBuiltInProfile) {
			t.Errorf("expected ErrBuiltInProfile for %q, got %v", b, err)
		}
	}

	// Reject protected system processes and reserved domains
	badProcs := []string{"explorer.exe"}
	if _, _, err := cfg.UpsertCustomProfile(CustomProfileInput{Name: "badproc", Processes: &badProcs}); !errors.Is(err, engine.ErrProtectedBinary) {
		t.Errorf("expected ErrProtectedBinary, got %v", err)
	}
	badSites := []string{"localhost"}
	if _, _, err := cfg.UpsertCustomProfile(CustomProfileInput{Name: "badsite", Domains: &badSites}); !errors.Is(err, engine.ErrReservedDomain) {
		t.Errorf("expected ErrReservedDomain, got %v", err)
	}

	// Create custom profile cloned from study + extra rules
	wMin := 45
	bMin := 10
	desc := "Deep writing workflow"
	sites := []string{"https://www.Medium.com/tag", "news.ycombinator.com"}
	apps := []string{"Slack"}
	prof, created, err := cfg.UpsertCustomProfile(CustomProfileInput{
		Name:         "deep-write_1",
		FromProfile:  "study",
		Description:  &desc,
		WorkMinutes:  &wMin,
		BreakMinutes: &bMin,
		Domains:      &sites,
		Processes:    &apps,
	})
	if err != nil || !created {
		t.Fatalf("UpsertCustomProfile create failed: created=%v err=%v", created, err)
	}
	if prof.DefaultWorkMinutes != 45 || prof.DefaultBreakMinutes != 10 {
		t.Errorf("unexpected durations: %+v", prof)
	}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded := LoadOrDefault("deep-write_1")
	if loaded.Name != "deep-write_1" || loaded.DefaultWorkMinutes != 45 {
		t.Fatalf("LoadOrDefault custom failed: %+v", loaded)
	}

	// Update existing custom profile work minutes without losing domains
	newWork := 60
	updatedProf, createdAgain, err := cfg.UpsertCustomProfile(CustomProfileInput{
		Name:        "deep-write_1",
		WorkMinutes: &newWork,
	})
	if err != nil || createdAgain {
		t.Fatalf("UpsertCustomProfile update failed: created=%v err=%v", createdAgain, err)
	}
	if updatedProf.DefaultWorkMinutes != 60 || len(updatedProf.BlockedDomains) != len(prof.BlockedDomains) {
		t.Errorf("unexpected updated profile: %+v", updatedProf)
	}

	// Refuse deleting built-in profiles
	for _, b := range []string{"study", "dev", "hardcore"} {
		if err := cfg.DeleteCustomProfile(b); !errors.Is(err, ErrBuiltInProfile) {
			t.Errorf("expected ErrBuiltInProfile deleting %q, got %v", b, err)
		}
	}

	// Delete custom profile
	if err := cfg.DeleteCustomProfile("deep-write_1"); err != nil {
		t.Fatalf("DeleteCustomProfile: %v", err)
	}
	if err := cfg.DeleteCustomProfile("deep-write_1"); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("expected ErrProfileNotFound on second delete, got %v", err)
	}
}

func TestProfiles_UTF8BOMZeroByteAndCorruptBackupRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FOCUSFLOW_HOME", tmpDir)
	cfgPath := GetConfigPath()

	// 1. UTF-8 BOM (PowerShell Set-Content -Encoding UTF8)
	bomPayload := "\xef\xbb\xbf{\n  \"daily_goal_minutes\": 180,\n  \"custom_domains\": [\"https://reddit.com:443/r/golang\"]\n}\n"
	if err := os.WriteFile(cfgPath, []byte(bomPayload), 0644); err != nil {
		t.Fatalf("write BOM config: %v", err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed on UTF-8 BOM config: %v", err)
	}
	if cfg.DailyGoalMinutes != 180 || len(cfg.CustomDomains) != 1 || cfg.CustomDomains[0] != "reddit.com" {
		t.Fatalf("unexpected BOM config values: %+v", cfg)
	}

	// 2. 0-byte config.json self-heals without creating .corrupt.bak
	if err := os.WriteFile(cfgPath, []byte{}, 0644); err != nil {
		t.Fatalf("write empty config: %v", err)
	}
	cfgEmpty, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed on 0-byte config: %v", err)
	}
	if cfgEmpty.DailyGoalMinutes != DefaultDailyGoalMinutes {
		t.Errorf("expected default goal %d on empty config, got %d", DefaultDailyGoalMinutes, cfgEmpty.DailyGoalMinutes)
	}
	if _, statErr := os.Stat(cfgPath + ".corrupt.bak"); !os.IsNotExist(statErr) {
		t.Errorf("did not expect .corrupt.bak for 0-byte config")
	}

	// 3. Truncated/corrupted config.json backs up to .corrupt.bak and self-heals
	corruptRaw := `{"daily_goal_minutes": 95, "profiles": [TRUNCATED`
	if err := os.WriteFile(cfgPath, []byte(corruptRaw), 0644); err != nil {
		t.Fatalf("write corrupt config: %v", err)
	}
	cfgRecovered, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig should self-heal corrupt config, got error: %v", err)
	}
	if cfgRecovered.DailyGoalMinutes != DefaultDailyGoalMinutes || len(cfgRecovered.Profiles) < 3 {
		t.Fatalf("unexpected recovered config: %+v", cfgRecovered)
	}
	bakBytes, err := os.ReadFile(cfgPath + ".corrupt.bak")
	if err != nil {
		t.Fatalf("expected config.json.corrupt.bak to exist: %v", err)
	}
	if string(bakBytes) != corruptRaw {
		t.Errorf("corrupt backup = %q, want %q", string(bakBytes), corruptRaw)
	}
}

func TestProfiles_V230SchemaNotificationsSchedulesAndHooks(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FOCUSFLOW_HOME", tmpDir)

	// Legacy v2.2 config omitting notifications_enabled defaults to true
	legacyRaw := `{"daily_goal_minutes": 150, "custom_domains": ["reddit.com"]}`
	if err := os.WriteFile(GetConfigPath(), []byte(legacyRaw), 0644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.NotificationsEnabled {
		t.Errorf("expected omitted notifications_enabled to default to true")
	}

	// Explicitly disable notifications, add schedule & hooks, verify round-trip
	cfg.NotificationsEnabled = false
	cfg.Hooks = engine.HooksConfig{
		OnStart: "echo start",
		OnEnd:   "echo end",
	}
	if _, err := cfg.AddSchedule(schedule.ScheduleRule{
		Name:      "Weekday Dev",
		Days:      []string{"mon-fri"},
		StartTime: "09:00",
		EndTime:   "12:00",
		Profile:   "dev",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("AddSchedule: %v", err)
	}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	reloaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig reload: %v", err)
	}
	if reloaded.NotificationsEnabled {
		t.Errorf("expected explicit notifications_enabled=false to persist")
	}
	if reloaded.Hooks.OnStart != "echo start" || reloaded.Hooks.OnEnd != "echo end" {
		t.Errorf("hooks did not round-trip: %+v", reloaded.Hooks)
	}
	if len(reloaded.Schedules) != 1 || reloaded.Schedules[0].Name != "Weekday Dev" {
		t.Errorf("schedules did not round-trip: %+v", reloaded.Schedules)
	}
}

func TestProfiles_AdversarialMalformedFieldsDegradeGracefully(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FOCUSFLOW_HOME", tmpDir)
	cfgPath := GetConfigPath()

	// Case 1: Wrong field types for schedules, hooks, notifications_enabled, and partial bad profiles
	// MUST preserve valid custom_domains, daily_goal_minutes, and valid custom profiles without wiping config.json!
	adversarial := `{
  "custom_domains": ["reddit.com", 123, null, "twitter.com"],
  "custom_processes": ["discord.exe", false],
  "daily_goal_minutes": 150,
  "notifications_enabled": "invalid",
  "schedules": "bad",
  "hooks": null,
  "profiles": {
    "good-custom": {
      "name": "good-custom",
      "description": "Kept intact",
      "blocked_domains": ["youtube.com"],
      "blocked_processes": ["steam.exe"],
      "default_work_minutes": 40,
      "default_break_minutes": 8
    },
    "bad-null": null,
    "bad-str": "not-an-object"
  }
}`
	if err := os.WriteFile(cfgPath, []byte(adversarial), 0644); err != nil {
		t.Fatalf("write adversarial config: %v", err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed on adversarial config: %v", err)
	}
	if cfg.DailyGoalMinutes != 150 {
		t.Errorf("expected daily_goal_minutes=150 preserved, got %d", cfg.DailyGoalMinutes)
	}
	if !cfg.NotificationsEnabled {
		t.Errorf("expected invalid notifications_enabled string to degrade to true")
	}
	if len(cfg.Schedules) != 0 {
		t.Errorf("expected bad schedules string to degrade to empty slice, got %+v", cfg.Schedules)
	}
	if cfg.Hooks.OnStart != "" || cfg.Hooks.OnEnd != "" {
		t.Errorf("expected null hooks to degrade to empty HooksConfig, got %+v", cfg.Hooks)
	}
	if len(cfg.CustomDomains) != 2 {
		t.Errorf("expected 2 valid custom domains preserved, got %v", cfg.CustomDomains)
	}
	if p, ok := cfg.GetProfile("good-custom"); !ok || p.DefaultWorkMinutes != 40 {
		t.Errorf("expected valid custom profile preserved, got %+v (ok=%v)", p, ok)
	}
	if _, statErr := os.Stat(cfgPath + ".corrupt.bak"); !os.IsNotExist(statErr) {
		t.Errorf("did not expect .corrupt.bak for valid JSON with wrong field types")
	}

	// Case 2: schedules: null and mixed valid/invalid schedule items + partial bad hook fields
	mixed := `{
  "daily_goal_minutes": "not-a-number",
  "notifications_enabled": null,
  "schedules": [
    {"id": "sch-ok", "name": "Valid Rule", "days": "mon-fri", "start_time": "09:00", "end_time": "11:00", "profile": "dev"},
    "not-an-object",
    {"name": 999, "days": ["mon"], "start_time": "09:00", "end_time": "10:00", "profile": "dev"},
    {"name": "Bad Time", "days": ["mon"], "start_time": "25:00", "end_time": "10:00", "profile": "dev"}
  ],
  "hooks": {"on_start": 42, "on_end": "echo ok"}
}`
	if err := os.WriteFile(cfgPath, []byte(mixed), 0644); err != nil {
		t.Fatalf("write mixed config: %v", err)
	}
	cfg2, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed on mixed config: %v", err)
	}
	if cfg2.DailyGoalMinutes != DefaultDailyGoalMinutes || !cfg2.NotificationsEnabled {
		t.Errorf("expected defaults for bad goal/notify, got goal=%d notify=%v", cfg2.DailyGoalMinutes, cfg2.NotificationsEnabled)
	}
	if len(cfg2.Schedules) != 1 || cfg2.Schedules[0].ID != "sch-ok" || !cfg2.Schedules[0].Enabled {
		t.Errorf("expected 1 valid schedule preserved with enabled=true default, got %+v", cfg2.Schedules)
	}
	if cfg2.Hooks.OnStart != "" || cfg2.Hooks.OnEnd != "echo ok" {
		t.Errorf("expected valid hook field preserved while bad field ignored, got %+v", cfg2.Hooks)
	}
}
