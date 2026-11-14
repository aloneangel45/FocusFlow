package storage

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestStore_LifecycleStreakAnalyticsAndExport(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FOCUSFLOW_HOME", tmpDir)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Empty store has 0 streak, 0 hours, 0% completion (zero fake data)
	emptySummary, err := store.ComputeAnalytics(7, time.Now())
	if err != nil {
		t.Fatalf("ComputeAnalytics empty: %v", err)
	}
	if emptySummary.ActiveStreakDays != 0 || emptySummary.TotalMinutes != 0 || emptySummary.CompletionRate != 0 {
		t.Fatalf("expected all-zero initial analytics, got %+v", emptySummary)
	}

	now := time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC)
	day2 := now.AddDate(0, 0, -2)
	day1 := now.AddDate(0, 0, -1)

	// 3 consecutive completed days + 1 aborted session + 1 running session that gets orphaned
	_ = store.FinalizeSession("s1", "study", 30, true, "completed", 2, day2, day2.Add(30*time.Minute))
	_ = store.FinalizeSession("s2", "dev", 60, true, "completed", 4, day1, day1.Add(60*time.Minute))
	_ = store.StartSession("s3", "study", 30, now.Add(-45*time.Minute))
	_ = store.FinalizeSession("s3", "study", 30, true, "completed", 1, now.Add(-45*time.Minute), now.Add(-15*time.Minute))
	_ = store.FinalizeSession("s4", "hardcore", 10, false, "aborted", 3, now.Add(-12*time.Minute), now.Add(-2*time.Minute))

	// Orphan session lifecycle
	_ = store.StartSession("orphan-99", "dev", 50, now.Add(-5*time.Minute))
	marked, err := store.MarkOrphanedSession("orphan-99", now)
	if err != nil || marked != 1 {
		t.Fatalf("expected 1 orphaned session marked, got marked=%d err=%v", marked, err)
	}

	summary, err := store.ComputeAnalytics(7, now)
	if err != nil {
		t.Fatalf("ComputeAnalytics failed: %v", err)
	}
	if summary.CompletedSprints != 3 {
		t.Errorf("CompletedSprints = %d, want 3", summary.CompletedSprints)
	}
	if summary.AttemptedSprints != 5 || summary.AbortedSprints != 2 {
		t.Errorf("Attempted=%d Aborted=%d, want 5 and 2", summary.AttemptedSprints, summary.AbortedSprints)
	}
	if summary.TotalMinutes != 120 || summary.TotalHours != 2.0 {
		t.Errorf("TotalMinutes=%d TotalHours=%.1f, want 120 and 2.0", summary.TotalMinutes, summary.TotalHours)
	}
	if summary.ActiveStreakDays != 3 {
		t.Errorf("ActiveStreakDays = %d, want 3", summary.ActiveStreakDays)
	}
	if summary.TotalBlocked != 10 {
		t.Errorf("TotalBlocked = %d, want 10", summary.TotalBlocked)
	}
	if len(summary.Daily) != 7 {
		t.Errorf("len(Daily) = %d, want 7", len(summary.Daily))
	}
	if summary.TodayFocusMinutes != 30 || summary.DailyGoalMinutes != 120 || summary.DailyGoalProgressPct != 25.0 {
		t.Errorf("unexpected daily goal summary: today=%d goal=%d pct=%.2f",
			summary.TodayFocusMinutes, summary.DailyGoalMinutes, summary.DailyGoalProgressPct)
	}

	goalProg, err := store.ComputeDailyGoalProgress(60, now)
	if err != nil {
		t.Fatalf("ComputeDailyGoalProgress: %v", err)
	}
	if goalProg.TodayFocusMinutes != 30 || goalProg.DailyGoalMinutes != 60 || goalProg.DailyGoalProgressPct != 50.0 {
		t.Errorf("unexpected explicit goal progress: %+v", goalProg)
	}

	// JSON export
	var jsonBuf bytes.Buffer
	if err := store.ExportJSON(&jsonBuf, 7, now); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	var payload ExportPayload
	if err := json.Unmarshal(jsonBuf.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal export JSON: %v", err)
	}
	if len(payload.Sessions) != 5 || payload.Summary.ActiveStreakDays != 3 {
		t.Errorf("unexpected exported JSON payload: %+v", payload.Summary)
	}

	// CSV export
	var csvBuf bytes.Buffer
	if err := store.ExportCSV(&csvBuf); err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}
	csvStr := csvBuf.String()
	if !strings.Contains(csvStr, "id,profile,start_time,end_time,duration_minutes") || !strings.Contains(csvStr, "orphan-99") {
		t.Errorf("unexpected CSV output:\n%s", csvStr)
	}
}

func TestStore_UTF8BOMZeroByteAndCorruptBackupRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FOCUSFLOW_HOME", tmpDir)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// 1. UTF-8 BOM sessions.json must preserve existing history on read & subsequent write
	bomSessions := "\xef\xbb\xbf{\n  \"sessions\": [\n    {\n      \"id\": \"hist-1\",\n      \"profile\": \"dev\",\n      \"start_time\": \"2026-06-15T10:00:00Z\",\n      \"duration_minutes\": 50,\n      \"completed\": true,\n      \"status\": \"completed\",\n      \"blocked_count\": 3\n    }\n  ]\n}\n"
	if err := os.WriteFile(store.Path(), []byte(bomSessions), 0644); err != nil {
		t.Fatalf("write BOM sessions: %v", err)
	}
	loaded, err := store.LoadSessions()
	if err != nil || len(loaded) != 1 || loaded[0].ID != "hist-1" {
		t.Fatalf("expected hist-1 loaded from BOM sessions.json, got len=%d err=%v", len(loaded), err)
	}
	startAt := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	if err := store.StartSession("hist-2", "study", 25, startAt); err != nil {
		t.Fatalf("StartSession after BOM: %v", err)
	}
	loadedAfter, _ := store.LoadSessions()
	if len(loadedAfter) != 2 {
		t.Fatalf("expected 2 sessions preserved after appending to BOM file, got %d", len(loadedAfter))
	}

	// 2. 0-byte sessions.json returns empty without .corrupt.bak
	if err := os.WriteFile(store.Path(), []byte{}, 0644); err != nil {
		t.Fatalf("write 0-byte sessions: %v", err)
	}
	emptyLoaded, err := store.LoadSessions()
	if err != nil || len(emptyLoaded) != 0 {
		t.Fatalf("expected empty sessions on 0-byte file, got %v (err=%v)", emptyLoaded, err)
	}
	if _, statErr := os.Stat(store.Path() + ".corrupt.bak"); !os.IsNotExist(statErr) {
		t.Errorf("did not expect .corrupt.bak for 0-byte sessions.json")
	}

	// 3. Corrupted sessions.json creates .corrupt.bak and self-heals
	corruptSessions := `{"sessions": [{"id": "broken"`
	if err := os.WriteFile(store.Path(), []byte(corruptSessions), 0644); err != nil {
		t.Fatalf("write corrupt sessions: %v", err)
	}
	healed, err := store.LoadSessions()
	if err != nil || len(healed) != 0 {
		t.Fatalf("expected healed empty slice on corrupt sessions.json, got %v (err=%v)", healed, err)
	}
	bakData, err := os.ReadFile(store.Path() + ".corrupt.bak")
	if err != nil {
		t.Fatalf("expected sessions.json.corrupt.bak to exist: %v", err)
	}
	if string(bakData) != corruptSessions {
		t.Errorf("corrupt backup = %q, want %q", string(bakData), corruptSessions)
	}
}

func TestStore_AbortActiveSessionsAndNoCompletedOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FOCUSFLOW_HOME", tmpDir)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	t0 := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	if err := store.StartSession("unpriv-run-1", "study", 25, t0); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if !store.IsSessionRunning("unpriv-run-1") {
		t.Fatalf("expected IsSessionRunning(unpriv-run-1) == true")
	}

	// External stop without journal ID aborts all running sessions and computes elapsed minutes
	aborted, err := store.AbortActiveSessions("", t0.Add(11*time.Minute+30*time.Second))
	if err != nil || aborted != 1 {
		t.Fatalf("expected 1 aborted session, got aborted=%d err=%v", aborted, err)
	}
	if store.IsSessionRunning("unpriv-run-1") {
		t.Fatalf("expected IsSessionRunning(unpriv-run-1) == false after abort")
	}

	// Late FinalizeSession(completed=true) from racing process must NOT overwrite aborted status
	_ = store.FinalizeSession("unpriv-run-1", "study", 25, true, "completed", 0, t0, t0.Add(25*time.Minute))

	sessions, _ := store.LoadSessions()
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session record, got %d", len(sessions))
	}
	if sessions[0].Completed || sessions[0].Status != "aborted" || sessions[0].DurationMinutes != 11 {
		t.Fatalf("expected aborted 11m record preserved, got %+v", sessions[0])
	}
}
