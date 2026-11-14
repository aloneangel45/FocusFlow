package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aloneangel45/focusflow/pkg/engine"
	"github.com/aloneangel45/focusflow/pkg/storage"
)

func setupHermeticCLIEnv(t *testing.T) (string, string) {
	t.Helper()
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")
	hostsPath := filepath.Join(tmpDir, "hosts")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	baselineHosts := "127.0.0.1 localhost\n::1 localhost\n"
	if err := os.WriteFile(hostsPath, []byte(baselineHosts), 0644); err != nil {
		t.Fatalf("write hosts: %v", err)
	}
	t.Setenv("FOCUSFLOW_HOME", stateDir)
	t.Setenv("FOCUSFLOW_HOSTS_PATH", hostsPath)
	return stateDir, hostsPath
}

func execCLI(args ...string) (int, string, string) {
	var outBuf, errBuf bytes.Buffer
	code := runCLI(args, strings.NewReader(""), &outBuf, &errBuf)
	return code, outBuf.String(), errBuf.String()
}

func TestCLI_ProfileSubcommandsAndValidation(t *testing.T) {
	setupHermeticCLIEnv(t)

	// 1. List initial built-in profiles
	code, out, errOut := execCLI("profile", "list", "--json")
	if code != 0 {
		t.Fatalf("profile list failed (code=%d): %s", code, errOut)
	}
	var listResp struct {
		Profiles []profileJSONItem `json:"profiles"`
	}
	if err := json.Unmarshal([]byte(out), &listResp); err != nil {
		t.Fatalf("unmarshal profile list: %v", err)
	}
	if len(listResp.Profiles) != 3 {
		t.Fatalf("expected 3 built-in profiles, got %d", len(listResp.Profiles))
	}

	// 2. Reject invalid slug, protected process, and overwriting built-in profile
	if c, _, _ := execCLI("profile", "create", "Bad_Slug!"); c == 0 {
		t.Errorf("expected invalid uppercase/punctuation slug to fail")
	}
	if c, _, _ := execCLI("profile", "create", "study", "--minutes", "30"); c == 0 {
		t.Errorf("expected overwriting built-in study profile to fail")
	}
	if c, _, _ := execCLI("profile", "create", "unsafe", "--apps", "explorer.exe"); c == 0 {
		t.Errorf("expected protected process explorer.exe to be rejected")
	}
	if c, _, _ := execCLI("profile", "create", "badhost", "--sites", "localhost"); c == 0 {
		t.Errorf("expected reserved domain localhost to be rejected")
	}

	// 3. Create custom profile cloned from study
	code, out, errOut = execCLI(
		"profile", "create", "writer-mode",
		"--from", "study",
		"--minutes", "45",
		"--break", "10",
		"--sites", "https://www.Medium.com/feed,news.ycombinator.com",
		"--apps", "Slack,zoom.exe",
		"--desc", "Distraction-free writing",
		"--json",
	)
	if code != 0 {
		t.Fatalf("profile create failed (code=%d): %s", code, errOut)
	}
	var createResp struct {
		Created bool            `json:"created"`
		Profile profileJSONItem `json:"profile"`
	}
	if err := json.Unmarshal([]byte(out), &createResp); err != nil {
		t.Fatalf("unmarshal createResp: %v", err)
	}
	if !createResp.Created || createResp.Profile.Name != "writer-mode" || createResp.Profile.BuiltIn {
		t.Fatalf("unexpected created profile: %+v", createResp)
	}
	if createResp.Profile.DefaultWorkMinutes != 45 || createResp.Profile.DefaultBreakMinutes != 10 {
		t.Errorf("unexpected profile minutes: %+v", createResp.Profile)
	}

	// 4. Show custom profile
	code, out, errOut = execCLI("profile", "show", "writer-mode", "--json")
	if code != 0 {
		t.Fatalf("profile show failed (code=%d): %s", code, errOut)
	}
	var shown profileJSONItem
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatalf("unmarshal shown: %v", err)
	}
	foundMedium := false
	for _, d := range shown.BlockedDomains {
		if d == "medium.com" {
			foundMedium = true
		}
	}
	if !foundMedium {
		t.Errorf("expected medium.com in writer-mode domains: %v", shown.BlockedDomains)
	}

	// 5. Refuse deleting built-in profile, allow deleting custom profile
	if c, _, _ := execCLI("profile", "delete", "dev"); c == 0 {
		t.Errorf("expected deleting built-in dev profile to fail")
	}
	code, _, errOut = execCLI("profile", "delete", "writer-mode", "--json")
	if code != 0 {
		t.Fatalf("profile delete failed (code=%d): %s", code, errOut)
	}
	if c, _, _ := execCLI("profile", "show", "writer-mode"); c == 0 {
		t.Errorf("expected writer-mode to be absent after deletion")
	}
}

func TestCLI_DailyGoalStatusAndStats(t *testing.T) {
	setupHermeticCLIEnv(t)

	// Default daily goal is 120m
	code, out, errOut := execCLI("goal", "--json")
	if code != 0 {
		t.Fatalf("goal --json failed: %s", errOut)
	}
	var gOut goalJSONOutput
	if err := json.Unmarshal([]byte(out), &gOut); err != nil {
		t.Fatalf("unmarshal goal: %v", err)
	}
	if gOut.DailyGoalMinutes != 120 || gOut.TodayFocusMinutes != 0 || gOut.Updated {
		t.Fatalf("unexpected initial goal output: %+v", gOut)
	}

	// Reject out-of-bounds --minutes
	if c, _, _ := execCLI("goal", "--minutes", "4"); c == 0 {
		t.Errorf("expected --minutes 4 to fail")
	}
	if c, _, _ := execCLI("goal", "--minutes", "1441"); c == 0 {
		t.Errorf("expected --minutes 1441 to fail")
	}

	// Set daily goal to 100m
	code, out, errOut = execCLI("goal", "--minutes", "100", "--json")
	if code != 0 {
		t.Fatalf("goal --minutes 100 failed: %s", errOut)
	}
	if err := json.Unmarshal([]byte(out), &gOut); err != nil {
		t.Fatalf("unmarshal updated goal: %v", err)
	}
	if !gOut.Updated || gOut.DailyGoalMinutes != 100 {
		t.Fatalf("expected updated goal 100, got %+v", gOut)
	}

	// Record a 50-minute completed session today
	store, err := storage.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	now := time.Now().UTC()
	if err := store.FinalizeSession("g-1", "dev", 50, true, "completed", 2, now.Add(-50*time.Minute), now); err != nil {
		t.Fatalf("FinalizeSession: %v", err)
	}

	// Verify human output in status & stats shows "Today: 50 / 100 min [50%]"
	code, statusHuman, _ := execCLI("status")
	if code != 0 || !strings.Contains(statusHuman, "Today: 50 / 100 min [50%]") {
		t.Errorf("expected daily goal progress in status output, got:\n%s", statusHuman)
	}
	code, statsHuman, _ := execCLI("stats")
	if code != 0 || !strings.Contains(statsHuman, "Today: 50 / 100 min [50%]") {
		t.Errorf("expected daily goal progress in stats output, got:\n%s", statsHuman)
	}

	// Verify JSON fields in status --json and stats --json
	_, statusRaw, _ := execCLI("status", "--json")
	var statusJSON statusJSONOutput
	if err := json.Unmarshal([]byte(statusRaw), &statusJSON); err != nil {
		t.Fatalf("unmarshal status JSON: %v", err)
	}
	if statusJSON.TodayFocusMinutes != 50 || statusJSON.DailyGoalMinutes != 100 || statusJSON.DailyGoalProgressPct != 50.0 {
		t.Errorf("unexpected status JSON goal fields: %+v", statusJSON)
	}

	_, statsRaw, _ := execCLI("stats", "--json")
	var statsJSON storage.AnalyticsSummary
	if err := json.Unmarshal([]byte(statsRaw), &statsJSON); err != nil {
		t.Fatalf("unmarshal stats JSON: %v", err)
	}
	if statsJSON.TodayFocusMinutes != 50 || statsJSON.DailyGoalMinutes != 100 || statsJSON.DailyGoalProgressPct != 50.0 {
		t.Errorf("unexpected stats JSON goal fields: %+v", statsJSON)
	}
}

func TestCLI_MultiCyclePomodoroAndInterruptRollback(t *testing.T) {
	_, hostsPath := setupHermeticCLIEnv(t)

	// Invalid cycle bounds
	if c, _, _ := execCLI("start", "--cycles", "0"); c == 0 {
		t.Errorf("expected --cycles 0 to fail")
	}
	if c, _, _ := execCLI("start", "--cycles", "25"); c == 0 {
		t.Errorf("expected --cycles 25 to fail")
	}

	// 1. Complete 2 Pomodoro cycles with 1s work + 1s break in standard mode
	var outBuf, errBuf bytes.Buffer
	sigChan := make(chan os.Signal, 1)
	code := runStartWithSignals(
		[]string{"--profile", "study", "--minutes", "25", "--seconds", "1", "--cycles", "2", "--break", "5", "--break-seconds", "1"},
		&outBuf,
		&errBuf,
		sigChan,
	)
	if code != 0 {
		t.Fatalf("multi-cycle start failed (code=%d): %s", code, errBuf.String())
	}
	outStr := outBuf.String()
	if !strings.Contains(outStr, "[CYCLE 1/2]") || !strings.Contains(outStr, "[CYCLE 2/2]") {
		t.Errorf("expected [CYCLE 1/2] and [CYCLE 2/2] in output:\n%s", outStr)
	}
	if !strings.Contains(outStr, "hosts shield temporarily lifted") {
		t.Errorf("expected hosts shield lifted during standard break:\n%s", outStr)
	}

	hm := engine.NewHostsManager()
	if hm.IsActive() {
		t.Errorf("expected hosts sinkhole rolled back after multi-cycle completion")
	}

	store, err := storage.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	sessions, err := store.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions: %v", err)
	}
	if len(sessions) != 2 || !sessions[0].Completed || !sessions[1].Completed {
		t.Fatalf("expected 2 completed sprint records, got %+v", sessions)
	}

	// 2. Hardcore mode keeps lockdown continuous during break, and SIGINT during break rolls back cleanly
	outBuf.Reset()
	errBuf.Reset()
	sigBreakChan := make(chan os.Signal, 1)
	go func() {
		time.Sleep(1500 * time.Millisecond)
		sigBreakChan <- os.Interrupt
	}()
	interruptCode := runStartWithSignals(
		[]string{"--profile", "hardcore", "--strict", "hardcore", "--seconds", "1", "--cycles", "2", "--break-seconds", "4"},
		&outBuf,
		&errBuf,
		sigBreakChan,
	)
	if interruptCode != 130 {
		t.Fatalf("expected exit code 130 on SIGINT, got %d", interruptCode)
	}
	if !strings.Contains(outBuf.String(), "hardcore lockdown continuous") {
		t.Errorf("expected hardcore continuous lockdown message during break:\n%s", outBuf.String())
	}
	rawHosts, _ := os.ReadFile(hostsPath)
	if engine.HasManagedMarkers(string(rawHosts)) {
		t.Errorf("expected hosts markers cleaned up after SIGINT during hardcore break:\n%s", string(rawHosts))
	}
}
