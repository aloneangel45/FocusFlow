package storage

import (
	"bytes"
	"encoding/json"
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
