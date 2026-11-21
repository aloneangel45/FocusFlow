package schedule

import (
	"reflect"
	"testing"
	"time"
)

func TestParseDays(t *testing.T) {
	tests := []struct {
		spec    string
		want    []string
		wantErr bool
	}{
		{"mon-fri", []string{"mon", "tue", "wed", "thu", "fri"}, false},
		{"mon,wed,fri", []string{"mon", "wed", "fri"}, false},
		{"fri-mon", []string{"mon", "fri", "sat", "sun"}, false},
		{"sat-sun", []string{"sat", "sun"}, false},
		{"daily", []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}, false},
		{"weekdays", []string{"mon", "tue", "wed", "thu", "fri"}, false},
		{"weekends", []string{"sat", "sun"}, false},
		{"Monday, Thursday", []string{"mon", "thu"}, false},
		{"", nil, true},
		{"funday", nil, true},
		{"mon-tue-wed", nil, true},
	}

	for _, tc := range tests {
		got, err := ParseDays(tc.spec)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseDays(%q) expected error, got %v", tc.spec, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDays(%q) unexpected error: %v", tc.spec, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseDays(%q) = %v, want %v", tc.spec, got, tc.want)
		}
	}
}

func TestValidateRule(t *testing.T) {
	rule, err := ValidateRule(ScheduleRule{
		Name:      "Morning Deep Work",
		Days:      []string{"mon-fri"},
		StartTime: "9:00",
		EndTime:   "12:00",
		Profile:   "Coding",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("ValidateRule failed: %v", err)
	}
	if rule.ID == "" || rule.StartTime != "09:00" || rule.EndTime != "12:00" || rule.Profile != "dev" {
		t.Fatalf("unexpected normalized rule: %+v", rule)
	}

	// Zero-length window rejected
	if _, err := ValidateRule(ScheduleRule{
		Name:      "Invalid",
		Days:      []string{"mon"},
		StartTime: "10:00",
		EndTime:   "10:00",
		Profile:   "dev",
		Enabled:   true,
	}); err == nil {
		t.Errorf("expected zero-length window to fail")
	}

	// Bad clock time rejected
	if _, err := ValidateRule(ScheduleRule{
		Name:      "Invalid",
		Days:      []string{"mon"},
		StartTime: "24:00",
		EndTime:   "12:00",
		Profile:   "dev",
		Enabled:   true,
	}); err == nil {
		t.Errorf("expected 24:00 to fail")
	}
}

func TestNextWindow_SameDayWeekdayWrapAndOvernight(t *testing.T) {
	loc := time.FixedZone("UTC+3", 3*3600)

	// 2026-11-20 is Friday
	fridayMorning := time.Date(2026, 11, 20, 8, 30, 0, 0, loc)
	fridayAfternoon := time.Date(2026, 11, 20, 14, 0, 0, 0, loc)
	saturdayNight := time.Date(2026, 11, 21, 23, 0, 0, 0, loc)

	weekdayRule := ScheduleRule{
		ID:        "r-work",
		Name:      "Workday Sprint",
		Days:      []string{"mon", "tue", "wed", "thu", "fri"},
		StartTime: "09:00",
		EndTime:   "12:00",
		Profile:   "dev",
		Enabled:   true,
	}
	disabledRule := ScheduleRule{
		ID:        "r-off",
		Name:      "Disabled Early",
		Days:      []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"},
		StartTime: "08:45",
		EndTime:   "09:15",
		Profile:   "study",
		Enabled:   false,
	}

	// 1. Same day upcoming (Fri 08:30 -> Fri 09:00..12:00), skipping disabled rule
	next := NextWindow([]ScheduleRule{disabledRule, weekdayRule}, fridayMorning)
	if next == nil {
		t.Fatal("expected next window")
	}
	wantStart := time.Date(2026, 11, 20, 9, 0, 0, 0, loc)
	wantEnd := time.Date(2026, 11, 20, 12, 0, 0, 0, loc)
	if !next.Start.Equal(wantStart) || !next.End.Equal(wantEnd) || next.Rule.ID != "r-work" {
		t.Fatalf("unexpected next window: %+v", next)
	}

	// 2. Weekday wrap-around across weekend (Fri 14:00 -> Mon 2026-11-23 09:00..12:00)
	nextWrap := NextWindow([]ScheduleRule{weekdayRule}, fridayAfternoon)
	if nextWrap == nil {
		t.Fatal("expected wrapped Monday window")
	}
	wantMonStart := time.Date(2026, 11, 23, 9, 0, 0, 0, loc)
	if !nextWrap.Start.Equal(wantMonStart) {
		t.Fatalf("expected Monday wrap %v, got %v", wantMonStart, nextWrap.Start)
	}

	// 3. Same-weekday wrap next week (only Friday rule, now is Fri 14:00 -> next Fri 2026-11-27 09:00)
	onlyFriday := ScheduleRule{
		ID:        "r-fri",
		Name:      "Friday Review",
		Days:      []string{"fri"},
		StartTime: "09:00",
		EndTime:   "11:00",
		Profile:   "study",
		Enabled:   true,
	}
	nextFri := NextWindow([]ScheduleRule{onlyFriday}, fridayAfternoon)
	if nextFri == nil || !nextFri.Start.Equal(time.Date(2026, 11, 27, 9, 0, 0, 0, loc)) {
		t.Fatalf("expected next Friday 2026-11-27 09:00, got %+v", nextFri)
	}

	// 4. Overnight rule: Sun 22:00 -> Mon 02:00
	overnight := ScheduleRule{
		ID:        "r-night",
		Name:      "Night Owl",
		Days:      []string{"sun"},
		StartTime: "22:00",
		EndTime:   "02:00",
		Profile:   "hardcore",
		Enabled:   true,
	}
	// From Sat 23:00, next is Sun 2026-11-22 22:00 -> Mon 2026-11-23 02:00
	nextNight := NextWindow([]ScheduleRule{overnight}, saturdayNight)
	if nextNight == nil {
		t.Fatal("expected overnight next window")
	}
	wantNightStart := time.Date(2026, 11, 22, 22, 0, 0, 0, loc)
	wantNightEnd := time.Date(2026, 11, 23, 2, 0, 0, 0, loc)
	if !nextNight.Start.Equal(wantNightStart) || !nextNight.End.Equal(wantNightEnd) {
		t.Fatalf("overnight window = [%v, %v], want [%v, %v]", nextNight.Start, nextNight.End, wantNightStart, wantNightEnd)
	}

	// ActiveWindow check after midnight (Mon 2026-11-23 01:15) MUST match Sunday's overnight window!
	mondayEarly := time.Date(2026, 11, 23, 1, 15, 0, 0, loc)
	active := ActiveWindow([]ScheduleRule{overnight}, mondayEarly)
	if active == nil {
		t.Fatal("expected Sunday overnight window to be active on Monday 01:15")
	}
	if !active.Start.Equal(wantNightStart) || !active.End.Equal(wantNightEnd) {
		t.Fatalf("active overnight window mismatch: %+v", active)
	}

	// Exactly at End (02:00) is NOT active anymore
	mondayEnd := time.Date(2026, 11, 23, 2, 0, 0, 0, loc)
	if ActiveWindow([]ScheduleRule{overnight}, mondayEnd) != nil {
		t.Errorf("expected window to be inactive exactly at end time")
	}
}

func TestSchedule_OvernightIsActiveAtInvalidAndOverlapPriority(t *testing.T) {
	loc := time.FixedZone("UTC+3", 3*3600)

	// 1. Friday overnight 22:00-02:00 MUST be active on Saturday 01:00 (2026-11-20 is Fri, 2026-11-21 is Sat)
	friNight := ScheduleRule{
		ID:        "r-fri-night",
		Name:      "Friday Night Deep Work",
		Days:      []string{"Fri"}, // intentionally unnormalized casing
		StartTime: "22:00",
		EndTime:   "02:00",
		Profile:   "hardcore",
		Enabled:   true,
	}
	if IsActiveAt(friNight, time.Date(2026, 11, 20, 21, 59, 0, 0, loc)) {
		t.Errorf("expected inactive at Fri 21:59")
	}
	if !IsActiveAt(friNight, time.Date(2026, 11, 20, 22, 0, 0, 0, loc)) {
		t.Errorf("expected active at Fri 22:00")
	}
	if !IsActiveAt(friNight, time.Date(2026, 11, 21, 1, 0, 0, 0, loc)) {
		t.Errorf("expected active at Sat 01:00 for Friday overnight 22:00-02:00")
	}
	if IsActiveAt(friNight, time.Date(2026, 11, 21, 2, 0, 0, 0, loc)) {
		t.Errorf("expected inactive at Sat 02:00")
	}
	if aw := ActiveWindow([]ScheduleRule{friNight}, time.Date(2026, 11, 21, 1, 0, 0, 0, loc)); aw == nil || aw.Rule.ID != "r-fri-night" {
		t.Fatalf("ActiveWindow did not match unnormalized Friday overnight rule on Sat 01:00: %+v", aw)
	}

	// 2. Invalid times, empty days, zero-length, and disabled rules rejected everywhere
	invalidTimes := []string{"24:00", "25:00", "-1:00", "12:60", "9:5", "", "abc"}
	for _, bad := range invalidTimes {
		if _, _, _, err := ParseClockTime(bad); err == nil {
			t.Errorf("ParseClockTime(%q) expected error", bad)
		}
		badRule := ScheduleRule{
			ID:        "bad",
			Name:      "Bad",
			Days:      []string{"daily"},
			StartTime: bad,
			EndTime:   "12:00",
			Profile:   "dev",
			Enabled:   true,
		}
		if IsActiveAt(badRule, time.Date(2026, 11, 20, 10, 0, 0, 0, loc)) {
			t.Errorf("IsActiveAt should be false for bad time %q", bad)
		}
		if NextWindow([]ScheduleRule{badRule}, time.Date(2026, 11, 20, 10, 0, 0, 0, loc)) != nil {
			t.Errorf("NextWindow should be nil for bad time %q", bad)
		}
	}

	emptyDaysRule := ScheduleRule{
		ID: "empty-days", Name: "Empty", Days: nil, StartTime: "09:00", EndTime: "12:00", Profile: "dev", Enabled: true,
	}
	zeroLenRule := ScheduleRule{
		ID: "zero-len", Name: "Zero", Days: []string{"mon-fri"}, StartTime: "09:00", EndTime: "09:00", Profile: "dev", Enabled: true,
	}
	disabledRule := ScheduleRule{
		ID: "disabled", Name: "Disabled", Days: []string{"daily"}, StartTime: "00:00", EndTime: "23:59", Profile: "dev", Enabled: false,
	}
	probe := time.Date(2026, 11, 20, 10, 0, 0, 0, loc)
	for _, r := range []ScheduleRule{emptyDaysRule, zeroLenRule, disabledRule} {
		if IsActiveAt(r, probe) {
			t.Errorf("expected IsActiveAt false for %s", r.ID)
		}
		if NextWindow([]ScheduleRule{r}, probe) != nil {
			t.Errorf("expected NextWindow nil for %s", r.ID)
		}
	}

	// 3. Deterministic overlapping rules priority: earlier start -> earlier end -> Name -> ID
	rLateStart := ScheduleRule{ID: "id-a", Name: "Alpha", Days: []string{"fri"}, StartTime: "09:30", EndTime: "11:00", Profile: "dev", Enabled: true}
	rEarlyLong := ScheduleRule{ID: "id-z", Name: "Zeta", Days: []string{"fri"}, StartTime: "09:00", EndTime: "12:00", Profile: "dev", Enabled: true}
	rEarlyShortB := ScheduleRule{ID: "id-2", Name: "Same", Days: []string{"fri"}, StartTime: "09:00", EndTime: "11:00", Profile: "dev", Enabled: true}
	rEarlyShortA := ScheduleRule{ID: "id-1", Name: "Same", Days: []string{"fri"}, StartTime: "09:00", EndTime: "11:00", Profile: "study", Enabled: true}

	aw := ActiveWindow([]ScheduleRule{rLateStart, rEarlyLong, rEarlyShortB, rEarlyShortA}, probe)
	if aw == nil || aw.Rule.ID != "id-1" {
		t.Fatalf("expected deterministic overlap winner id-1, got %+v", aw)
	}
}
