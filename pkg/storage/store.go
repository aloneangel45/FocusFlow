package storage

import (
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SessionRecord struct {
	ID              string `json:"id,omitempty"`
	Profile         string `json:"profile"`
	StartTime       string `json:"start_time"`
	EndTime         string `json:"end_time,omitempty"`
	DurationMinutes int    `json:"duration_minutes"`
	PlannedMinutes  int    `json:"planned_minutes,omitempty"`
	Completed       bool   `json:"completed"`
	Status          string `json:"status,omitempty"`
	BlockedCount    int    `json:"blocked_count"`
}

type MetricsStore struct {
	Sessions []SessionRecord `json:"sessions"`
}

type DailyStat struct {
	Date    string `json:"date"`
	Minutes int    `json:"minutes"`
	Sprints int    `json:"sprints"`
}

type AnalyticsSummary struct {
	CompletedSprints int         `json:"completed_sprints"`
	AttemptedSprints int         `json:"attempted_sprints"`
	AbortedSprints   int         `json:"aborted_sprints"`
	CompletionRate   float64     `json:"completion_rate"`
	TotalMinutes     int         `json:"total_minutes"`
	TotalHours       float64     `json:"total_hours"`
	TotalBlocked     int         `json:"total_blocked"`
	ActiveStreakDays int         `json:"active_streak_days"`
	Daily            []DailyStat `json:"daily"`
}

type ExportPayload struct {
	GeneratedAt string           `json:"generated_at"`
	Summary     AnalyticsSummary `json:"summary"`
	Sessions    []SessionRecord  `json:"sessions"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

// GetStateDir resolves the unified FocusFlow state directory shared by Go CLI and Python GUI.
// Priority: FOCUSFLOW_HOME -> %APPDATA%\FocusFlow (Windows) / ~/.config/focusflow (POSIX) -> .focusflow
func GetStateDir() string {
	if override := strings.TrimSpace(os.Getenv("FOCUSFLOW_HOME")); override != "" {
		return filepath.Clean(override)
	}
	if runtime.GOOS == "windows" {
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			return filepath.Join(appData, "FocusFlow")
		}
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, "AppData", "Roaming", "FocusFlow")
		}
		return ".focusflow"
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "focusflow")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "focusflow")
	}
	return ".focusflow"
}

func GenerateSessionID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("ff-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func NewStore() (*Store, error) {
	return NewStoreWithDir(GetStateDir())
}

func NewStoreWithDir(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, "sessions.json")}, nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) readLocked() MetricsStore {
	var data MetricsStore
	if raw, err := os.ReadFile(s.path); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &data)
	}
	if data.Sessions == nil {
		data.Sessions = []SessionRecord{}
	}
	return data
}

func (s *Store) writeLocked(data MetricsStore) error {
	if data.Sessions == nil {
		data.Sessions = []SessionRecord{}
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')

	tmpFile, err := os.CreateTemp(dir, "sessions-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(out); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	_ = tmpFile.Sync()
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	_ = os.Chmod(tmpPath, 0644)
	if err := os.Rename(tmpPath, s.path); err != nil {
		// Windows fallback if destination exists and Rename transiently fails
		if writeErr := os.WriteFile(s.path, out, 0644); writeErr != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		_ = os.Remove(tmpPath)
	}
	return nil
}

// StartSession records the start of a focus sprint immediately so StartTime is accurate
// and unexpected crashes can mark the session as orphaned/incomplete.
func (s *Store) StartSession(sessionID, profile string, plannedMin int, startTime time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sessionID == "" {
		sessionID = GenerateSessionID()
	}
	data := s.readLocked()
	record := SessionRecord{
		ID:              sessionID,
		Profile:         profile,
		StartTime:       startTime.UTC().Format(time.RFC3339),
		DurationMinutes: 0,
		PlannedMinutes:  plannedMin,
		Completed:       false,
		Status:          "running",
		BlockedCount:    0,
	}
	data.Sessions = append(data.Sessions, record)
	return s.writeLocked(data)
}

// FinalizeSession updates an existing running session (or appends if missing) upon completion or abort.
func (s *Store) FinalizeSession(sessionID, profile string, durationMin int, completed bool, status string, blockedCount int, startTime, endTime time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if status == "" {
		if completed {
			status = "completed"
		} else {
			status = "aborted"
		}
	}

	data := s.readLocked()
	updated := false
	if sessionID != "" {
		for i := len(data.Sessions) - 1; i >= 0; i-- {
			if data.Sessions[i].ID == sessionID {
				data.Sessions[i].DurationMinutes = durationMin
				data.Sessions[i].Completed = completed
				data.Sessions[i].Status = status
				data.Sessions[i].BlockedCount = blockedCount
				data.Sessions[i].EndTime = endTime.UTC().Format(time.RFC3339)
				if data.Sessions[i].StartTime == "" && !startTime.IsZero() {
					data.Sessions[i].StartTime = startTime.UTC().Format(time.RFC3339)
				}
				updated = true
				break
			}
		}
	}

	if !updated {
		if startTime.IsZero() {
			startTime = endTime.Add(-time.Duration(durationMin) * time.Minute)
		}
		data.Sessions = append(data.Sessions, SessionRecord{
			ID:              sessionID,
			Profile:         profile,
			StartTime:       startTime.UTC().Format(time.RFC3339),
			EndTime:         endTime.UTC().Format(time.RFC3339),
			DurationMinutes: durationMin,
			Completed:       completed,
			Status:          status,
			BlockedCount:    blockedCount,
		})
	}
	return s.writeLocked(data)
}

// RecordSession records a completed or aborted session directly.
func (s *Store) RecordSession(profile string, durationMin int, completed bool, blockedCount int) error {
	now := time.Now().UTC()
	start := now.Add(-time.Duration(durationMin) * time.Minute)
	status := "aborted"
	if completed {
		status = "completed"
	}
	return s.FinalizeSession(GenerateSessionID(), profile, durationMin, completed, status, blockedCount, start, now)
}

// MarkOrphanedSession marks an orphaned session (by sessionID, plus any stale "running" records when sessionID matches or is empty) as completed: false.
func (s *Store) MarkOrphanedSession(sessionID string, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data := s.readLocked()
	marked := 0
	for i := range data.Sessions {
		rec := &data.Sessions[i]
		if (sessionID != "" && rec.ID == sessionID) || rec.Status == "running" {
			if rec.Status == "running" || (sessionID != "" && rec.ID == sessionID && rec.Completed) || (sessionID != "" && rec.ID == sessionID && rec.Status == "") {
				rec.Completed = false
				rec.Status = "orphaned"
				if rec.EndTime == "" {
					rec.EndTime = now.UTC().Format(time.RFC3339)
				}
				marked++
			}
		}
	}
	if marked > 0 {
		if err := s.writeLocked(data); err != nil {
			return 0, err
		}
	}
	return marked, nil
}

func (s *Store) LoadSessions() ([]SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := s.readLocked()
	out := make([]SessionRecord, len(data.Sessions))
	copy(out, data.Sessions)
	return out, nil
}

func (s *Store) GetTotalStats() (int, int, int, error) {
	summary, err := s.ComputeAnalytics(7, time.Now())
	if err != nil {
		return 0, 0, 0, err
	}
	return summary.CompletedSprints, summary.TotalMinutes, summary.TotalBlocked, nil
}

func parseSessionLocalDate(ts string, loc *time.Location) (string, bool) {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return "", false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, ts); err == nil {
			if layout == "2006-01-02" {
				return t.Format("2006-01-02"), true
			}
			return t.In(loc).Format("2006-01-02"), true
		}
	}
	if len(ts) >= 10 && ts[4] == '-' && ts[7] == '-' {
		return ts[:10], true
	}
	return "", false
}

// ComputeAnalytics calculates real streak, completion rate, focus hours, and N-day chart from sessions.json.
func (s *Store) ComputeAnalytics(days int, now time.Time) (*AnalyticsSummary, error) {
	if days <= 0 {
		days = 7
	}
	sessions, err := s.LoadSessions()
	if err != nil {
		return nil, err
	}

	loc := now.Location()
	today := time.Date(now.In(loc).Year(), now.In(loc).Month(), now.In(loc).Day(), 0, 0, 0, 0, loc)

	dailyMin := make(map[string]int)
	dailySprints := make(map[string]int)
	completedDates := make(map[string]bool)

	var completedCount, attemptedCount, abortedCount, totalMinutes, totalBlocked int

	for _, rec := range sessions {
		totalBlocked += rec.BlockedCount
		if rec.Status == "running" {
			continue
		}
		attemptedCount++
		if rec.Completed {
			completedCount++
			totalMinutes += rec.DurationMinutes
			if dStr, ok := parseSessionLocalDate(rec.StartTime, loc); ok {
				dailyMin[dStr] += rec.DurationMinutes
				dailySprints[dStr]++
				if rec.DurationMinutes > 0 {
					completedDates[dStr] = true
				}
			}
		} else {
			abortedCount++
		}
	}

	var completionRate float64
	if attemptedCount > 0 {
		completionRate = (float64(completedCount) / float64(attemptedCount)) * 100.0
	}

	// Real streak calculation: consecutive calendar days ending today or yesterday
	streak := 0
	todayStr := today.Format("2006-01-02")
	yesterday := today.AddDate(0, 0, -1)
	yesterdayStr := yesterday.Format("2006-01-02")

	var cursor time.Time
	hasStart := false
	if completedDates[todayStr] {
		cursor = today
		hasStart = true
	} else if completedDates[yesterdayStr] {
		cursor = yesterday
		hasStart = true
	}
	if hasStart {
		for completedDates[cursor.Format("2006-01-02")] {
			streak++
			cursor = cursor.AddDate(0, 0, -1)
		}
	}

	daily := make([]DailyStat, 0, days)
	for i := days - 1; i >= 0; i-- {
		d := today.AddDate(0, 0, -i).Format("2006-01-02")
		daily = append(daily, DailyStat{
			Date:    d,
			Minutes: dailyMin[d],
			Sprints: dailySprints[d],
		})
	}

	return &AnalyticsSummary{
		CompletedSprints: completedCount,
		AttemptedSprints: attemptedCount,
		AbortedSprints:   abortedCount,
		CompletionRate:   completionRate,
		TotalMinutes:     totalMinutes,
		TotalHours:       float64(totalMinutes) / 60.0,
		TotalBlocked:     totalBlocked,
		ActiveStreakDays: streak,
		Daily:            daily,
	}, nil
}

func (s *Store) ExportJSON(w io.Writer, days int, now time.Time) error {
	sessions, err := s.LoadSessions()
	if err != nil {
		return err
	}
	summary, err := s.ComputeAnalytics(days, now)
	if err != nil {
		return err
	}
	payload := ExportPayload{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Summary:     *summary,
		Sessions:    sessions,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func (s *Store) ExportCSV(w io.Writer) error {
	sessions, err := s.LoadSessions()
	if err != nil {
		return err
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].StartTime < sessions[j].StartTime
	})

	cw := csv.NewWriter(w)
	header := []string{"id", "profile", "start_time", "end_time", "duration_minutes", "planned_minutes", "completed", "status", "blocked_count"}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, rec := range sessions {
		status := rec.Status
		if status == "" {
			if rec.Completed {
				status = "completed"
			} else {
				status = "aborted"
			}
		}
		row := []string{
			rec.ID,
			rec.Profile,
			rec.StartTime,
			rec.EndTime,
			strconv.Itoa(rec.DurationMinutes),
			strconv.Itoa(rec.PlannedMinutes),
			strconv.FormatBool(rec.Completed),
			status,
			strconv.Itoa(rec.BlockedCount),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
