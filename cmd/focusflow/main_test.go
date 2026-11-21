package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aloneangel45/focusflow/pkg/engine"
	"github.com/aloneangel45/focusflow/pkg/profile"
	"github.com/aloneangel45/focusflow/pkg/schedule"
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

type lockedBuffer struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (lb *lockedBuffer) Write(p []byte) (int, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.buf.Write(p)
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
	var syncOutMu sync.Mutex
	var syncOutBuf bytes.Buffer
	syncOut := &lockedBuffer{mu: &syncOutMu, buf: &syncOutBuf}
	errBuf.Reset()
	sigBreakChan := make(chan os.Signal, 1)
	doneBreak := make(chan int, 1)
	go func() {
		doneBreak <- runStartWithSignals(
			[]string{"--profile", "hardcore", "--strict", "hardcore", "--seconds", "1", "--cycles", "2", "--break-seconds", "8"},
			syncOut,
			&errBuf,
			sigBreakChan,
		)
	}()
	breakWaitDeadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(breakWaitDeadline) {
		syncOutMu.Lock()
		inBreak := strings.Contains(syncOutBuf.String(), "hardcore lockdown continuous")
		syncOutMu.Unlock()
		if inBreak {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	sigBreakChan <- os.Interrupt
	interruptCode := <-doneBreak
	if interruptCode != 130 {
		t.Fatalf("expected exit code 130 on SIGINT, got %d", interruptCode)
	}
	syncOutMu.Lock()
	breakOutStr := syncOutBuf.String()
	syncOutMu.Unlock()
	if !strings.Contains(breakOutStr, "hardcore lockdown continuous") {
		t.Errorf("expected hardcore continuous lockdown message during break:\n%s", breakOutStr)
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

func TestCLI_ExportToSessionsJSONDoesNotSelfTruncate(t *testing.T) {
	stateDir, _ := setupHermeticCLIEnv(t)

	store, err := storage.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	now := time.Now().UTC()
	if err := store.FinalizeSession("exp-1", "dev", 50, true, "completed", 4, now.Add(-50*time.Minute), now); err != nil {
		t.Fatalf("FinalizeSession: %v", err)
	}

	sessionsPath := filepath.Join(stateDir, "sessions.json")
	code, _, errOut := execCLI("export", "--format", "json", "--out", sessionsPath)
	if code != 0 {
		t.Fatalf("export --out sessions.json failed: %s", errOut)
	}

	var exported storage.ExportPayload
	raw, err := os.ReadFile(sessionsPath)
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}
	if err := json.Unmarshal(raw, &exported); err != nil {
		t.Fatalf("unmarshal exported payload: %v", err)
	}
	if len(exported.Sessions) != 1 || exported.Sessions[0].ID != "exp-1" || exported.Summary.TotalMinutes != 50 {
		t.Fatalf("export --out <sessions.json> self-truncated session history! got: %+v", exported)
	}
}

func TestCLI_ReadOnlyCommandsSkipStartupAutoRecovery(t *testing.T) {
	_, hostsPath := setupHermeticCLIEnv(t)

	store, err := storage.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	now := time.Now().UTC()
	if err := store.StartSession("orphan-ro-1", "hardcore", 25, now.Add(-10*time.Minute)); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	hm := engine.NewHostsManager()
	if err := hm.ApplyBlocklistWithSession([]string{"youtube.com"}, engine.SessionMeta{
		SessionID:     "orphan-ro-1",
		PID:           99999999, // Dead PID
		StartedAt:     now.Add(-10 * time.Minute),
		ExpectedEndAt: now.Add(15 * time.Minute),
		Profile:       "hardcore",
	}); err != nil {
		t.Fatalf("ApplyBlocklistWithSession: %v", err)
	}

	for _, roArgs := range [][]string{
		{"status", "--json"},
		{"stats", "--json"},
		{"list", "--json"},
		{"export", "--format", "json"},
	} {
		code, _, errOut := execCLI(roArgs...)
		if code != 0 {
			t.Fatalf("read-only command %v failed: %s", roArgs, errOut)
		}
		if !hm.IsActive() {
			t.Fatalf("read-only command %v unexpectedly mutated/rolled back hosts!", roArgs)
		}
		if !store.IsSessionRunning("orphan-ro-1") {
			t.Fatalf("read-only command %v unexpectedly mutated sessions.json status!", roArgs)
		}
	}

	// Explicit diagnostic `doctor` command MUST still perform orphan recovery
	docCode, _, docErr := execCLI("doctor", "--json")
	if docCode != 0 {
		t.Fatalf("doctor --json failed: %s", docErr)
	}
	rawHosts, _ := os.ReadFile(hostsPath)
	if engine.HasManagedMarkers(string(rawHosts)) {
		t.Fatalf("expected doctor to recover orphaned hosts markers")
	}
	if store.IsSessionRunning("orphan-ro-1") {
		t.Fatalf("expected doctor to mark orphan-ro-1 as orphaned")
	}
}

func TestCLI_HardcoreMultiCycleBreakRefreshesJournalLease(t *testing.T) {
	setupHermeticCLIEnv(t)

	hm := engine.NewHostsManager()
	sigChan := make(chan os.Signal, 1)
	var outBuf, errBuf bytes.Buffer
	doneCh := make(chan int, 1)

	startedWall := time.Now().UTC()
	go func() {
		doneCh <- runStartWithSignals(
			[]string{"--profile", "hardcore", "--strict", "hardcore", "--seconds", "1", "--cycles", "2", "--break-seconds", "8"},
			&outBuf,
			&errBuf,
			sigChan,
		)
	}()

	// Wait until sprint 1 finishes and break begins (process scan + DNS flush takes ~2-3s on Windows)
	deadline := time.Now().Add(10 * time.Second)
	refreshed := false
	for time.Now().Before(deadline) {
		if j, err := hm.LoadJournal(); err == nil && j != nil {
			if exp, pErr := time.Parse(time.RFC3339, j.ExpectedEndAt); pErr == nil {
				// Initial work lease was startedWall + 1s; break refresh pushes ExpectedEndAt out by ~1m08s!
				if exp.After(startedWall.Add(30 * time.Second)) {
					refreshed = true
					break
				}
			}
		}
		time.Sleep(80 * time.Millisecond)
	}

	sigChan <- os.Interrupt
	<-doneCh

	if !refreshed {
		t.Fatalf("expected hardcore break to refresh hosts journal ExpectedEndAt lease! output:\n%s", outBuf.String())
	}
}

func TestCLI_ScheduleCRUDNextAndDaemonLifecycle(t *testing.T) {
	_, hostsPath := setupHermeticCLIEnv(t)
	t.Setenv("FOCUSFLOW_NO_NOTIFY", "1")

	// 1. Initially empty schedule list
	code, out, errOut := execCLI("schedule", "list", "--json")
	if code != 0 {
		t.Fatalf("schedule list failed: %s", errOut)
	}
	var listResp struct {
		Schedules []schedule.ScheduleRule `json:"schedules"`
	}
	if err := json.Unmarshal([]byte(out), &listResp); err != nil || len(listResp.Schedules) != 0 {
		t.Fatalf("expected empty schedules, got %s (err=%v)", out, err)
	}

	// 2. Reject invalid schedule inputs
	if c, _, _ := execCLI("schedule", "add", "--name", "Bad", "--days", "funday", "--from", "09:00", "--to", "12:00", "--profile", "dev"); c == 0 {
		t.Errorf("expected invalid days to fail")
	}
	if c, _, _ := execCLI("schedule", "add", "--name", "Bad2", "--days", "mon-fri", "--from", "09:00", "--to", "09:00", "--profile", "dev"); c == 0 {
		t.Errorf("expected zero-length schedule to fail")
	}
	if c, _, _ := execCLI("schedule", "add", "--name", "Bad3", "--days", "mon-fri", "--from", "09:00", "--to", "12:00", "--profile", "nonexistent"); c == 0 {
		t.Errorf("expected unknown profile schedule to fail")
	}

	// 3. Add valid weekday schedule
	code, out, errOut = execCLI(
		"schedule", "add",
		"--name", "Morning Deep Work",
		"--days", "mon-fri",
		"--from", "09:00",
		"--to", "12:00",
		"--profile", "dev",
		"--json",
	)
	if code != 0 {
		t.Fatalf("schedule add failed: %s", errOut)
	}
	var addResp struct {
		Added    bool                  `json:"added"`
		Schedule schedule.ScheduleRule `json:"schedule"`
	}
	if err := json.Unmarshal([]byte(out), &addResp); err != nil || !addResp.Added || addResp.Schedule.Profile != "dev" {
		t.Fatalf("unexpected schedule add response: %s (err=%v)", out, err)
	}

	// Reject duplicate name
	if c, _, _ := execCLI("schedule", "add", "--name", "morning deep work", "--days", "mon", "--from", "10:00", "--to", "11:00", "--profile", "study"); c == 0 {
		t.Errorf("expected duplicate schedule name to fail")
	}

	// 4. `schedule next --json` computes next window
	code, out, errOut = execCLI("schedule", "next", "--json")
	if code != 0 {
		t.Fatalf("schedule next --json failed: %s", errOut)
	}
	var nextResp scheduleNextJSONOutput
	if err := json.Unmarshal([]byte(out), &nextResp); err != nil || nextResp.Next == nil {
		t.Fatalf("expected non-nil next window in JSON, got %s (err=%v)", out, err)
	}
	if nextResp.Next.Profile != "dev" || nextResp.Next.StartsInSeconds <= 0 {
		t.Fatalf("unexpected next window: %+v", nextResp.Next)
	}

	// 5. Daemon enters an active window, engages hosts sinkhole, and cleanly releases at window end
	cfg, err := profile.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.Schedules = []schedule.ScheduleRule{
		{
			ID:        "sch-active-test",
			Name:      "Active Window Test",
			Days:      []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"},
			StartTime: "10:00",
			EndTime:   "10:05",
			Profile:   "study",
			Enabled:   true,
		},
	}
	if err := profile.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	var mu sync.Mutex
	simNow := time.Date(2026, 11, 20, 10, 4, 59, 0, time.Local)
	nowFn := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return simNow
	}

	var dOut, dErr bytes.Buffer
	sigChan := make(chan os.Signal, 1)
	doneCh := make(chan int, 1)
	go func() {
		doneCh <- runScheduleDaemonWithSignals(
			[]string{"--check-interval", "50ms", "--no-notify"},
			&dOut,
			&dErr,
			sigChan,
			nowFn,
		)
	}()

	// Wait until hosts sinkhole is engaged by the daemon
	deadline := time.Now().Add(5 * time.Second)
	engaged := false
	for time.Now().Before(deadline) {
		rawHosts, _ := os.ReadFile(hostsPath)
		if engine.HasManagedMarkers(string(rawHosts)) {
			engaged = true
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if !engaged {
		sigChan <- os.Interrupt
		<-doneCh
		t.Fatalf("scheduler daemon did not engage hosts sinkhole inside active window; out=%s err=%s", dOut.String(), dErr.String())
	}

	// Advance simulated clock past 10:05 so daemon releases window cleanly
	mu.Lock()
	simNow = time.Date(2026, 11, 20, 10, 5, 1, 0, time.Local)
	mu.Unlock()

	deadline = time.Now().Add(5 * time.Second)
	released := false
	for time.Now().Before(deadline) {
		rawHosts, _ := os.ReadFile(hostsPath)
		if !engine.HasManagedMarkers(string(rawHosts)) {
			released = true
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	sigChan <- os.Interrupt
	exitCode := <-doneCh

	if !released {
		t.Fatalf("scheduler daemon did not release hosts sinkhole at window end; out=%s", dOut.String())
	}
	if exitCode != 0 {
		t.Fatalf("expected daemon graceful shutdown code 0, got %d (err=%s)", exitCode, dErr.String())
	}

	// 6. Remove schedule by name/id
	code, out, errOut = execCLI("schedule", "remove", "Active Window Test", "--json")
	if code != 0 {
		t.Fatalf("schedule remove failed: %s", errOut)
	}
	if !strings.Contains(out, `"removed": true`) {
		t.Fatalf("unexpected remove output: %s", out)
	}
}

func TestCLI_StatsHeatmapANSIAndASCIIDegradation(t *testing.T) {
	setupHermeticCLIEnv(t)

	store, err := storage.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	now := time.Now()
	// Create a 3-day active streak today..today-2 and a 5-day historical longest streak 20..24 days ago
	for _, offset := range []int{0, 1, 2, 20, 21, 22, 23, 24} {
		day := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, now.Location()).AddDate(0, 0, -offset)
		_ = store.FinalizeSession(storage.GenerateSessionID(), "dev", 60, true, "completed", 1, day, day.Add(60*time.Minute))
	}

	// 1. Color heatmap
	t.Setenv("NO_COLOR", "")
	code, outColor, errOut := execCLI("stats", "--heatmap")
	if code != 0 {
		t.Fatalf("stats --heatmap failed: %s", errOut)
	}
	if !strings.Contains(outColor, "90-Day Focus Contribution Heatmap") || !strings.Contains(outColor, "\x1b[38;5;") {
		t.Fatalf("expected ANSI 90-day heatmap output, got:\n%s", outColor)
	}
	if !strings.Contains(outColor, "Active 3 days") || !strings.Contains(outColor, "Longest 5 days") {
		t.Fatalf("expected active=3 and longest=5 streaks, got:\n%s", outColor)
	}

	// 2. --no-color ASCII fallback
	code, outASCII, errOut := execCLI("stats", "--heatmap", "--no-color")
	if code != 0 {
		t.Fatalf("stats --heatmap --no-color failed: %s", errOut)
	}
	if strings.Contains(outASCII, "\x1b[") {
		t.Fatalf("--no-color heatmap must not emit ANSI escapes:\n%s", outASCII)
	}
	if !strings.Contains(outASCII, "[ ] [.] [o] [O] [#]") {
		t.Fatalf("expected ASCII legend in --no-color heatmap:\n%s", outASCII)
	}

	// 3. NO_COLOR env var fallback
	t.Setenv("NO_COLOR", "1")
	code, outNoColorEnv, errOut := execCLI("stats", "--heatmap")
	if code != 0 || strings.Contains(outNoColorEnv, "\x1b[") {
		t.Fatalf("NO_COLOR=1 heatmap must degrade cleanly to ASCII (code=%d err=%s):\n%s", code, errOut, outNoColorEnv)
	}
}

func TestCLI_LifecycleHooksAndRollbackResilience(t *testing.T) {
	stateDir, hostsPath := setupHermeticCLIEnv(t)
	t.Setenv("FOCUSFLOW_NO_NOTIFY", "1")

	markerFile := filepath.Join(stateDir, "hook_fired.txt")
	var startHook, failEndHook string
	if runtime.GOOS == "windows" {
		startHook = `echo %FOCUSFLOW_EVENT%:%FOCUSFLOW_PROFILE%:%FOCUSFLOW_MINUTES%> "` + markerFile + `"`
		failEndHook = `exit /b 9`
	} else {
		startHook = `printf '%s:%s:%s' "$FOCUSFLOW_EVENT" "$FOCUSFLOW_PROFILE" "$FOCUSFLOW_MINUTES" > "` + markerFile + `"`
		failEndHook = `exit 9`
	}

	var outBuf, errBuf bytes.Buffer
	sigChan := make(chan os.Signal, 1)
	code := runStartWithSignals(
		[]string{
			"--profile", "dev",
			"--minutes", "30",
			"--seconds", "1",
			"--no-notify",
			"--on-start", startHook,
			"--on-end", failEndHook,
		},
		&outBuf,
		&errBuf,
		sigChan,
	)
	if code != 0 {
		t.Fatalf("start with failing end hook should still succeed cleanly (code=%d): %s", code, errBuf.String())
	}

	rawMarker, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatalf("expected --on-start hook marker file to exist: %v", err)
	}
	if !strings.Contains(string(rawMarker), "start:dev:30") {
		t.Errorf("unexpected --on-start hook marker content: %q", string(rawMarker))
	}
	if !strings.Contains(errBuf.String(), "[WARN] Lifecycle hook (end) failed") {
		t.Errorf("expected non-fatal warning from failing --on-end hook, got stderr=%q", errBuf.String())
	}

	// Critical invariant: failing --on-end hook MUST NEVER prevent hosts rollback
	rawHosts, _ := os.ReadFile(hostsPath)
	if engine.HasManagedMarkers(string(rawHosts)) {
		t.Fatalf("hosts file still has managed markers after failing --on-end hook:\n%s", string(rawHosts))
	}
}

func TestCLI_ScheduleDaemonDeletedProfileSleepWakeAndInvalidStrict(t *testing.T) {
	_, hostsPath := setupHermeticCLIEnv(t)
	t.Setenv("FOCUSFLOW_NO_NOTIFY", "1")

	// 1. Invalid --strict rejected immediately
	var badOut, badErr bytes.Buffer
	sigBad := make(chan os.Signal, 1)
	if code := runScheduleDaemonWithSignals([]string{"--strict", "nonsense"}, &badOut, &badErr, sigBad, time.Now); code == 0 {
		t.Fatalf("expected invalid --strict to fail fast")
	}

	// 2. Deleted profile fallback warning + wake-from-sleep past window end releases hosts cleanly without crediting sleep
	cfg, err := profile.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.Schedules = []schedule.ScheduleRule{
		{
			ID:        "sch-deleted-prof",
			Name:      "Orphaned Profile Window",
			Days:      []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"},
			StartTime: "10:00",
			EndTime:   "10:30",
			Profile:   "deleted-custom-profile",
			Enabled:   true,
		},
	}
	if err := profile.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	var mu sync.Mutex
	simNow := time.Date(2026, 11, 20, 10, 20, 0, 0, time.Local)
	nowFn := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return simNow
	}

	var outMu sync.Mutex
	var rawOut, rawErr bytes.Buffer
	dOut := &lockedBuffer{mu: &outMu, buf: &rawOut}
	dErr := &lockedBuffer{mu: &outMu, buf: &rawErr}
	sigChan := make(chan os.Signal, 1)
	doneCh := make(chan int, 1)
	go func() {
		doneCh <- runScheduleDaemonWithSignals(
			[]string{"--check-interval", "40ms", "--no-notify"},
			dOut,
			dErr,
			sigChan,
			nowFn,
		)
	}()

	// Wait for hosts sinkhole engagement
	deadline := time.Now().Add(5 * time.Second)
	engaged := false
	for time.Now().Before(deadline) {
		rawHosts, _ := os.ReadFile(hostsPath)
		if engine.HasManagedMarkers(string(rawHosts)) {
			engaged = true
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if !engaged {
		sigChan <- os.Interrupt
		<-doneCh
		t.Fatalf("daemon did not engage hosts for fallback profile")
	}

	// Simulate laptop sleeping for 3 hours and waking long past 10:30 window end!
	mu.Lock()
	simNow = time.Date(2026, 11, 20, 13, 30, 0, 0, time.Local)
	mu.Unlock()

	deadline = time.Now().Add(5 * time.Second)
	released := false
	for time.Now().Before(deadline) {
		rawHosts, _ := os.ReadFile(hostsPath)
		if !engine.HasManagedMarkers(string(rawHosts)) {
			released = true
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	sigChan <- os.Interrupt
	exitCode := <-doneCh

	if !released || exitCode != 0 {
		t.Fatalf("expected clean hosts release on wake-from-sleep (released=%v exit=%d)", released, exitCode)
	}
	outMu.Lock()
	errText := rawErr.String()
	outText := rawOut.String()
	outMu.Unlock()
	if !strings.Contains(errText, "missing/deleted profile 'deleted-custom-profile'") {
		t.Errorf("expected missing/deleted profile warning on stderr, got: %q", errText)
	}
	if !strings.Contains(outText, "System sleep/wake detected") {
		t.Errorf("expected sleep/wake detection log, got: %q", outText)
	}

	store, _ := storage.NewStore()
	sessions, _ := store.LoadSessions()
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session record, got %+v", sessions)
	}
	if sessions[0].Completed || sessions[0].Status != "aborted" || sessions[0].Profile != "study" {
		t.Errorf("expected wake-from-sleep session finalized as aborted on fallback study profile, got %+v", sessions[0])
	}
	if sessions[0].PlannedMinutes != 10 {
		t.Errorf("expected mid-window planned_minutes=10 (10:20..10:30), got %d", sessions[0].PlannedMinutes)
	}
}
