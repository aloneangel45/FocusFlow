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
	localNow := time.Now()
	todayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, localNow.Location())
	if err := store.FinalizeSession("g-1", "dev", 50, true, "completed", 2, todayStart, localNow); err != nil {
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

func TestCLI_SubcommandFlagOrderingAndPositionalValidation(t *testing.T) {
	setupHermeticCLIEnv(t)

	// 1. profile create with flags before, between, and after positional <name>
	code, out, errOut := execCLI("profile", "create", "myprof", "--minutes", "35", "--break", "7", "--json")
	if code != 0 {
		t.Fatalf("profile create myprof --minutes 35 failed: %s", errOut)
	}
	var cResp struct {
		Created bool            `json:"created"`
		Profile profileJSONItem `json:"profile"`
	}
	if err := json.Unmarshal([]byte(out), &cResp); err != nil {
		t.Fatalf("unmarshal create JSON: %v", err)
	}
	if cResp.Profile.Name != "myprof" || cResp.Profile.DefaultWorkMinutes != 35 || cResp.Profile.DefaultBreakMinutes != 7 {
		t.Fatalf("unexpected profile create result: %+v", cResp.Profile)
	}

	// 2. profile show <name> --json and profile show --json <name>
	for _, args := range [][]string{
		{"profile", "show", "myprof", "--json"},
		{"profile", "show", "--json", "myprof"},
	} {
		c, o, e := execCLI(args...)
		if c != 0 {
			t.Fatalf("%v failed: %s", args, e)
		}
		var item profileJSONItem
		if err := json.Unmarshal([]byte(o), &item); err != nil || item.Name != "myprof" {
			t.Fatalf("%v returned unexpected JSON: %s (err=%v)", args, o, err)
		}
	}

	// 3. Unknown flag on profile create must fail rather than swallowing positional <name>
	if c, _, _ := execCLI("profile", "create", "--unknown-opt", "swallowed"); c == 0 {
		t.Errorf("expected unknown flag on profile create to fail")
	}

	// 4. add / remove with --json and path basename normalization
	cAddSite, addSiteOut, addSiteErr := execCLI("add", "site", "https://www.Reddit.com:443/r/golang", "--json")
	if cAddSite != 0 {
		t.Fatalf("add site --json failed: %s", addSiteErr)
	}
	var addSiteResp map[string]any
	if err := json.Unmarshal([]byte(addSiteOut), &addSiteResp); err != nil || addSiteResp["added"] != true || addSiteResp["value"] != "reddit.com" {
		t.Fatalf("unexpected add site JSON: %+v (err=%v)", addSiteResp, err)
	}

	cRemSite, remSiteOut, remSiteErr := execCLI("remove", "--json", "site", "https://reddit.com:443/test")
	if cRemSite != 0 {
		t.Fatalf("remove --json site failed: %s", remSiteErr)
	}
	var remSiteResp map[string]any
	if err := json.Unmarshal([]byte(remSiteOut), &remSiteResp); err != nil || remSiteResp["removed"] != true || remSiteResp["value"] != "reddit.com" {
		t.Fatalf("unexpected remove site JSON: %+v (err=%v)", remSiteResp, err)
	}

	cAddApp, addAppOut, addAppErr := execCLI("add", "app", "Steam", "--json")
	if cAddApp != 0 {
		t.Fatalf("add app --json failed: %s", addAppErr)
	}
	var addAppResp map[string]any
	if err := json.Unmarshal([]byte(addAppOut), &addAppResp); err != nil || addAppResp["added"] != true || addAppResp["value"] != "steam.exe" {
		t.Fatalf("unexpected add app JSON: %+v (err=%v)", addAppResp, err)
	}
	cRemApp, remAppOut, remAppErr := execCLI("remove", "app", "/opt/games/Steam.exe", "--json")
	if cRemApp != 0 {
		t.Fatalf("remove app --json failed: %s", remAppErr)
	}
	var remAppResp map[string]any
	if err := json.Unmarshal([]byte(remAppOut), &remAppResp); err != nil || remAppResp["removed"] != true || remAppResp["value"] != "steam.exe" {
		t.Fatalf("unexpected remove app JSON: %+v (err=%v)", remAppResp, err)
	}

	// 5. goal <positional> must fail fast rather than silently ignoring the argument
	if c, _, _ := execCLI("goal", "90"); c == 0 {
		t.Errorf("expected 'focusflow goal 90' without --minutes to return non-zero error")
	}

	// 6. start <profile> --minutes <n> (positional profile before flags)
	var outBuf, errBuf bytes.Buffer
	sigChan := make(chan os.Signal, 1)
	startCode := runStartWithSignals([]string{"dev", "--minutes", "33", "--seconds", "1"}, &outBuf, &errBuf, sigChan)
	if startCode != 0 {
		t.Fatalf("start dev --minutes 33 failed: %s", errBuf.String())
	}
	store, _ := storage.NewStore()
	sessions, _ := store.LoadSessions()
	if len(sessions) != 1 || sessions[0].Profile != "dev" || sessions[0].DurationMinutes != 33 || !sessions[0].Completed {
		t.Fatalf("expected completed 33m dev session, got %+v", sessions)
	}

	// 7. profile delete <name> --json
	if c, _, e := execCLI("profile", "delete", "myprof", "--json"); c != 0 {
		t.Fatalf("profile delete myprof --json failed: %s", e)
	}
}

func TestCLI_StopAbortsRunningSessionAndConcurrentStartExitsCleanly(t *testing.T) {
	_, hostsPath := setupHermeticCLIEnv(t)

	store, err := storage.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Part A: Unprivileged running session (no hosts journal) is finalized as aborted on `focusflow stop`
	t0 := time.Now().UTC().Add(-8 * time.Minute)
	if err := store.StartSession("ghost-unpriv-1", "study", 25, t0); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	code, stopOut, stopErr := execCLI("stop", "--json")
	if code != 0 {
		t.Fatalf("stop --json failed: %s", stopErr)
	}
	var sResp stopJSONOutput
	if err := json.Unmarshal([]byte(stopOut), &sResp); err != nil {
		t.Fatalf("unmarshal stop JSON: %v", err)
	}
	if !sResp.Stopped || sResp.AbortedSessions != 1 {
		t.Fatalf("expected Stopped=true AbortedSessions=1, got %+v", sResp)
	}
	afterStop, _ := store.LoadSessions()
	if len(afterStop) != 1 || afterStop[0].Status != "aborted" || afterStop[0].Completed {
		t.Fatalf("expected ghost-unpriv-1 aborted, got %+v", afterStop)
	}

	// Part B: Concurrent `focusflow start` + external `focusflow stop` must NOT re-apply hosts or double-count
	var startOut, startErr bytes.Buffer
	sigChan := make(chan os.Signal, 1)
	doneCh := make(chan int, 1)
	go func() {
		doneCh <- runStartWithSignals(
			[]string{"--profile", "study", "--minutes", "25", "--seconds", "8", "--strict", "hardcore"},
			&startOut,
			&startErr,
			sigChan,
		)
	}()

	// Wait until the new running session appears in sessions.json
	deadline := time.Now().Add(3 * time.Second)
	sawRunning := false
	for time.Now().Before(deadline) {
		cur, _ := store.LoadSessions()
		if len(cur) == 2 && cur[1].Status == "running" {
			sawRunning = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !sawRunning {
		t.Fatalf("timed out waiting for concurrent start session to enter running state")
	}

	// Invoke external `focusflow stop`
	if c, _, e := execCLI("stop"); c != 0 {
		t.Fatalf("external stop failed: %s", e)
	}

	select {
	case exitCode := <-doneCh:
		if exitCode != 0 {
			t.Fatalf("expected concurrent start to exit cleanly with 0 after external stop, got %d (stderr=%s)", exitCode, startErr.String())
		}
	case <-time.After(4 * time.Second):
		sigChan <- os.Interrupt
		t.Fatalf("concurrent start loop did not exit promptly after external stop")
	}

	rawHosts, _ := os.ReadFile(hostsPath)
	if engine.HasManagedMarkers(string(rawHosts)) {
		t.Fatalf("expected hosts file clean (not re-applied) after external stop:\n%s", string(rawHosts))
	}

	finalSessions, _ := store.LoadSessions()
	if len(finalSessions) != 2 {
		t.Fatalf("expected exactly 2 session records, got %d: %+v", len(finalSessions), finalSessions)
	}
	for _, rec := range finalSessions {
		if rec.Status == "running" || rec.Completed {
			t.Fatalf("expected no running or completed records after external stop, got %+v", rec)
		}
	}
}
