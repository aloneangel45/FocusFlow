package schedule

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalidRuleName = errors.New("schedule name must be 1..64 printable characters")
	ErrInvalidDays     = errors.New("invalid schedule days specification")
	ErrInvalidTime     = errors.New("invalid time format (expected HH:MM in 24-hour clock)")
	ErrZeroLength      = errors.New("schedule start_time and end_time cannot be identical")
	ErrInvalidProfile  = errors.New("schedule profile cannot be empty")
)

// ScheduleRule defines a recurring local-time focus window persisted in config.json.
type ScheduleRule struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Days      []string `json:"days"`
	StartTime string   `json:"start_time"`
	EndTime   string   `json:"end_time"`
	Profile   string   `json:"profile"`
	Enabled   bool     `json:"enabled"`
}

// Window represents a concrete resolved local-time focus window for a ScheduleRule.
type Window struct {
	Rule  ScheduleRule `json:"rule"`
	Start time.Time    `json:"start"`
	End   time.Time    `json:"end"`
}

var canonicalDayOrder = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

var dayAliasToIndex = map[string]int{
	"mon": 0, "monday": 0,
	"tue": 1, "tues": 1, "tuesday": 1,
	"wed": 2, "weds": 2, "wednesday": 2,
	"thu": 3, "thur": 3, "thurs": 3, "thursday": 3,
	"fri": 4, "friday": 4,
	"sat": 5, "saturday": 5,
	"sun": 6, "sunday": 6,
}

var weekdayToCanonical = map[time.Weekday]string{
	time.Monday:    "mon",
	time.Tuesday:   "tue",
	time.Wednesday: "wed",
	time.Thursday:  "thu",
	time.Friday:    "fri",
	time.Saturday:  "sat",
	time.Sunday:    "sun",
}

// GenerateRuleID generates a unique random schedule identifier.
func GenerateRuleID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("sch-%x", time.Now().UnixNano()&0xffffffff)
	}
	return "sch-" + hex.EncodeToString(b)
}

// ParseDays parses specifications like "mon-fri", "mon,wed,fri", "fri-mon", "daily",
// "weekdays", or "weekends" into a canonical deduplicated slice of "mon".."sun".
func ParseDays(spec string) ([]string, error) {
	trimmed := strings.TrimSpace(strings.ToLower(spec))
	if trimmed == "" {
		return nil, fmt.Errorf("%w: empty days", ErrInvalidDays)
	}

	present := make([]bool, 7)
	parts := strings.Split(trimmed, ",")
	for _, rawPart := range parts {
		tok := strings.TrimSpace(rawPart)
		if tok == "" {
			continue
		}
		switch tok {
		case "daily", "all", "everyday", "*":
			for i := 0; i < 7; i++ {
				present[i] = true
			}
			continue
		case "weekdays", "weekday":
			for i := 0; i < 5; i++ {
				present[i] = true
			}
			continue
		case "weekends", "weekend":
			present[5] = true
			present[6] = true
			continue
		}

		if strings.Contains(tok, "-") {
			bounds := strings.Split(tok, "-")
			if len(bounds) != 2 {
				return nil, fmt.Errorf("%w: %q", ErrInvalidDays, tok)
			}
			startIdx, okStart := dayAliasToIndex[strings.TrimSpace(bounds[0])]
			endIdx, okEnd := dayAliasToIndex[strings.TrimSpace(bounds[1])]
			if !okStart || !okEnd {
				return nil, fmt.Errorf("%w: %q", ErrInvalidDays, tok)
			}
			idx := startIdx
			for {
				present[idx] = true
				if idx == endIdx {
					break
				}
				idx = (idx + 1) % 7
			}
			continue
		}

		idx, ok := dayAliasToIndex[tok]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrInvalidDays, tok)
		}
		present[idx] = true
	}

	out := make([]string, 0, 7)
	for i, ok := range present {
		if ok {
			out = append(out, canonicalDayOrder[i])
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: no days selected", ErrInvalidDays)
	}
	return out, nil
}

// NormalizeDays validates and deduplicates a slice of day tokens into canonical "mon".."sun" order.
func NormalizeDays(days []string) ([]string, error) {
	if len(days) == 0 {
		return nil, fmt.Errorf("%w: empty days", ErrInvalidDays)
	}
	return ParseDays(strings.Join(days, ","))
}

// ParseClockTime parses a 24-hour clock string ("HH:MM" or "H:MM") and returns
// hour (0..23), minute (0..59), and canonical zero-padded "HH:MM".
func ParseClockTime(raw string) (int, int, string, error) {
	s := strings.TrimSpace(raw)
	parts := strings.Split(s, ":")
	if len(parts) != 2 || len(parts[0]) == 0 || len(parts[0]) > 2 || len(parts[1]) != 2 {
		return 0, 0, "", fmt.Errorf("%w: %q", ErrInvalidTime, raw)
	}
	for _, p := range parts {
		for _, ch := range p {
			if ch < '0' || ch > '9' {
				return 0, 0, "", fmt.Errorf("%w: %q", ErrInvalidTime, raw)
			}
		}
	}
	hour, err1 := strconv.Atoi(parts[0])
	min, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || hour < 0 || hour > 23 || min < 0 || min > 59 {
		return 0, 0, "", fmt.Errorf("%w: %q", ErrInvalidTime, raw)
	}
	return hour, min, fmt.Sprintf("%02d:%02d", hour, min), nil
}

// ValidateRule validates and normalizes a ScheduleRule.
func ValidateRule(r ScheduleRule) (ScheduleRule, error) {
	name := strings.TrimSpace(r.Name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return ScheduleRule{}, fmt.Errorf("%w: %q", ErrInvalidRuleName, r.Name)
	}
	for _, ch := range name {
		if ch < 32 || ch == 127 {
			return ScheduleRule{}, fmt.Errorf("%w: control characters not allowed", ErrInvalidRuleName)
		}
	}

	days, err := NormalizeDays(r.Days)
	if err != nil {
		return ScheduleRule{}, err
	}

	sh, sm, startNorm, err := ParseClockTime(r.StartTime)
	if err != nil {
		return ScheduleRule{}, fmt.Errorf("start_time: %w", err)
	}
	eh, em, endNorm, err := ParseClockTime(r.EndTime)
	if err != nil {
		return ScheduleRule{}, fmt.Errorf("end_time: %w", err)
	}
	if sh*60+sm == eh*60+em {
		return ScheduleRule{}, ErrZeroLength
	}

	prof := strings.ToLower(strings.TrimSpace(r.Profile))
	if prof == "" {
		return ScheduleRule{}, ErrInvalidProfile
	}
	if prof == "coding" {
		prof = "dev"
	} else if prof == "lockdown" {
		prof = "hardcore"
	}

	id := strings.TrimSpace(r.ID)
	if id == "" {
		id = GenerateRuleID()
	}

	return ScheduleRule{
		ID:        id,
		Name:      name,
		Days:      days,
		StartTime: startNorm,
		EndTime:   endNorm,
		Profile:   prof,
		Enabled:   r.Enabled,
	}, nil
}

// SanitizeRules normalizes a slice of rules from persisted storage, skipping invalid entries.
func SanitizeRules(rules []ScheduleRule) []ScheduleRule {
	if len(rules) == 0 {
		return []ScheduleRule{}
	}
	seenID := make(map[string]struct{}, len(rules))
	out := make([]ScheduleRule, 0, len(rules))
	for _, r := range rules {
		clean, err := ValidateRule(r)
		if err != nil {
			continue
		}
		if _, dup := seenID[clean.ID]; dup {
			clean.ID = GenerateRuleID()
		}
		seenID[clean.ID] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func containsDay(days []string, target string) bool {
	for _, d := range days {
		if d == target {
			return true
		}
	}
	return false
}

// WindowForDate resolves the concrete local start/end interval for a rule starting on the given calendar date.
// Overnight rules (end_time < start_time) extend cleanly into the following calendar day.
func WindowForDate(rule ScheduleRule, date time.Time) (time.Time, time.Time, bool) {
	if !rule.Enabled {
		return time.Time{}, time.Time{}, false
	}
	normDays, err := NormalizeDays(rule.Days)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	dayName := weekdayToCanonical[date.Weekday()]
	if !containsDay(normDays, dayName) {
		return time.Time{}, time.Time{}, false
	}
	sh, sm, _, err1 := ParseClockTime(rule.StartTime)
	eh, em, _, err2 := ParseClockTime(rule.EndTime)
	if err1 != nil || err2 != nil || sh*60+sm == eh*60+em {
		return time.Time{}, time.Time{}, false
	}

	loc := date.Location()
	start := time.Date(date.Year(), date.Month(), date.Day(), sh, sm, 0, 0, loc)
	endDate := date
	if eh*60+em < sh*60+sm {
		endDate = date.AddDate(0, 0, 1)
	}
	end := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), eh, em, 0, 0, loc)
	if !end.After(start) {
		return time.Time{}, time.Time{}, false
	}
	return start, end, true
}

// IsActiveAt reports whether a single ScheduleRule is active at `now` (in `now`'s timezone),
// including overnight windows that started on the previous calendar day.
func IsActiveAt(rule ScheduleRule, now time.Time) bool {
	if !rule.Enabled {
		return false
	}
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	for _, dayOffset := range []int{-1, 0} {
		candidateDate := today.AddDate(0, 0, dayOffset)
		start, end, ok := WindowForDate(rule, candidateDate)
		if ok && !now.Before(start) && now.Before(end) {
			return true
		}
	}
	return false
}

func windowPrecedes(cand, best *Window) bool {
	if best == nil {
		return true
	}
	if !cand.Start.Equal(best.Start) {
		return cand.Start.Before(best.Start)
	}
	if !cand.End.Equal(best.End) {
		return cand.End.Before(best.End)
	}
	if cand.Rule.Name != best.Rule.Name {
		return cand.Rule.Name < best.Rule.Name
	}
	return cand.Rule.ID < best.Rule.ID
}

// ActiveWindow returns the currently active enabled schedule window at `now` (in `now`'s timezone),
// properly handling overnight windows that began on the previous day. Returns nil if no rule is active.
func ActiveWindow(rules []ScheduleRule, now time.Time) *Window {
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	var best *Window
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		for _, dayOffset := range []int{-1, 0} {
			candidateDate := today.AddDate(0, 0, dayOffset)
			start, end, ok := WindowForDate(r, candidateDate)
			if !ok {
				continue
			}
			if !now.Before(start) && now.Before(end) {
				cand := &Window{Rule: r, Start: start, End: end}
				if windowPrecedes(cand, best) {
					best = cand
				}
			}
		}
	}
	return best
}

// NextWindow computes the earliest upcoming enabled schedule window starting strictly after `now`
// (in `now`'s timezone), supporting weekday wrap-around across weeks and overnight windows.
// Returns nil if no enabled valid rules exist.
func NextWindow(rules []ScheduleRule, now time.Time) *Window {
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	var best *Window
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		for dayOffset := 0; dayOffset <= 7; dayOffset++ {
			candidateDate := today.AddDate(0, 0, dayOffset)
			start, end, ok := WindowForDate(r, candidateDate)
			if !ok {
				continue
			}
			if start.After(now) {
				cand := &Window{Rule: r, Start: start, End: end}
				if windowPrecedes(cand, best) {
					best = cand
				}
				break // earliest future occurrence for this rule found
			}
		}
	}
	return best
}

// FormatDurationShort returns a compact human-readable duration like "2d 3h 15m", "45m", or "<1m".
func FormatDurationShort(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	if d < time.Minute {
		return "<1m"
	}
	totalMin := int(d / time.Minute)
	days := totalMin / (24 * 60)
	rem := totalMin % (24 * 60)
	hours := rem / 60
	mins := rem % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if mins > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%dm", mins))
	}
	return strings.Join(parts, " ")
}
