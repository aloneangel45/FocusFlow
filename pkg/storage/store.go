package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type SessionRecord struct {
	Profile         string `json:"profile"`
	StartTime       string `json:"start_time"`
	DurationMinutes int    `json:"duration_minutes"`
	Completed       bool   `json:"completed"`
	BlockedCount    int    `json:"blocked_count"`
}

type MetricsStore struct {
	Sessions []SessionRecord `json:"sessions"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore() (*Store, error) {
	appData := os.Getenv("APPDATA")
	var dir string
	if appData != "" {
		dir = filepath.Join(appData, "FocusFlow")
	} else {
		dir = ".focusflow"
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	path := filepath.Join(dir, "sessions.json")
	return &Store{path: path}, nil
}

func (s *Store) RecordSession(profile string, durationMin int, completed bool, blockedCount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var data MetricsStore
	if raw, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(raw, &data)
	}

	record := SessionRecord{
		Profile:         profile,
		StartTime:       time.Now().Format(time.RFC3339),
		DurationMinutes: durationMin,
		Completed:       completed,
		BlockedCount:    blockedCount,
	}

	data.Sessions = append(data.Sessions, record)

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, out, 0644)
}

func (s *Store) GetTotalStats() (int, int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var data MetricsStore
	if raw, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(raw, &data)
	}

	var totalSessions, totalMinutes, totalBlocked int
	for _, rec := range data.Sessions {
		if rec.Completed {
			totalSessions++
			totalMinutes += rec.DurationMinutes
			totalBlocked += rec.BlockedCount
		}
	}

	return totalSessions, totalMinutes, totalBlocked, nil
}
