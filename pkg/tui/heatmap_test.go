package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/aloneangel45/focusflow/pkg/storage"
)

func TestHeatmapIntensityLevels(t *testing.T) {
	goal := 100
	if got := HeatmapIntensity(0, goal); got != 0 {
		t.Errorf("0 min -> level %d, want 0", got)
	}
	if got := HeatmapIntensity(20, goal); got != 1 {
		t.Errorf("20 min -> level %d, want 1", got)
	}
	if got := HeatmapIntensity(50, goal); got != 2 {
		t.Errorf("50 min -> level %d, want 2", got)
	}
	if got := HeatmapIntensity(80, goal); got != 3 {
		t.Errorf("80 min -> level %d, want 3", got)
	}
	if got := HeatmapIntensity(100, goal); got != 4 {
		t.Errorf("100 min -> level %d, want 4", got)
	}
	if got := HeatmapIntensity(180, goal); got != 4 {
		t.Errorf("180 min -> level %d, want 4", got)
	}
}

func TestFormatHeatmap_ANSIAndASCIIDegradation(t *testing.T) {
	now := time.Date(2026, 11, 20, 15, 0, 0, 0, time.Local)
	daily := make([]storage.DailyStat, 0, 90)
	for i := 89; i >= 0; i-- {
		d := now.AddDate(0, 0, -i).Format("2006-01-02")
		mins := 0
		switch i % 5 {
		case 1:
			mins = 20
		case 2:
			mins = 50
		case 3:
			mins = 90
		case 4:
			mins = 140
		}
		daily = append(daily, storage.DailyStat{Date: d, Minutes: mins, Sprints: 1})
	}

	// 1. ANSI color output
	ansiOut := FormatHeatmap(daily, 120, 4, 12, now, true)
	if !strings.Contains(ansiOut, "\x1b[38;5;") {
		t.Errorf("expected ANSI color sequences in color heatmap:\n%s", ansiOut)
	}
	if !strings.Contains(ansiOut, "90-Day Focus Contribution Heatmap") {
		t.Errorf("missing 90-day header:\n%s", ansiOut)
	}
	if !strings.Contains(ansiOut, "Active 4 days") || !strings.Contains(ansiOut, "Longest 12 days") {
		t.Errorf("missing streak summary:\n%s", ansiOut)
	}
	if containsEmoji(ansiOut) {
		t.Errorf("heatmap must remain zero-emoji:\n%s", ansiOut)
	}

	// 2. ASCII degradation (--no-color / NO_COLOR)
	asciiOut := FormatHeatmap(daily, 120, 4, 12, now, false)
	if strings.Contains(asciiOut, "\x1b[") {
		t.Errorf("ASCII heatmap must NOT contain ANSI escape codes:\n%s", asciiOut)
	}
	for _, sym := range []string{"[ ]", "[.]", "[o]", "[O]", "[#]"} {
		if !strings.Contains(asciiOut, sym) {
			t.Errorf("expected ASCII legend symbol %q in output:\n%s", sym, asciiOut)
		}
	}
	if !strings.Contains(asciiOut, "Mon|") || !strings.Contains(asciiOut, "Sun|") {
		t.Errorf("expected weekday row labels in ASCII output:\n%s", asciiOut)
	}
	if containsEmoji(asciiOut) {
		t.Errorf("ASCII heatmap must remain zero-emoji:\n%s", asciiOut)
	}

	// 3. NO_COLOR env var disables color automatically
	t.Setenv("NO_COLOR", "1")
	if ShouldUseColor(false) {
		t.Errorf("expected ShouldUseColor(false) == false when NO_COLOR=1")
	}
	t.Setenv("NO_COLOR", "")
	if ShouldUseColor(true) {
		t.Errorf("expected ShouldUseColor(true) == false when --no-color flag is set")
	}
	if !ShouldUseColor(false) {
		t.Errorf("expected ShouldUseColor(false) == true when NO_COLOR is unset")
	}
}

func TestFormatHeatmap_DSTEmptyAndDuplicateDates(t *testing.T) {
	// 1. Empty daily slice still renders a valid 90-day grid with 0/90 active days
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	emptyOut := FormatHeatmap(nil, 120, 0, 0, now, false)
	if !strings.Contains(emptyOut, "90-Day Focus Contribution Heatmap") || !strings.Contains(emptyOut, "Active Days: 0/90 (0 min)") {
		t.Errorf("unexpected empty heatmap output:\n%s", emptyOut)
	}

	// 2. Duplicate dates sum cleanly and match footer totals (ignoring out-of-window dates)
	daily := []storage.DailyStat{
		{Date: "2026-04-14", Minutes: 30, Sprints: 1},
		{Date: "2026-04-15", Minutes: 40, Sprints: 1},
		{Date: "2026-04-15", Minutes: 25, Sprints: 1},
	}
	dupOut := FormatHeatmap(daily, 100, 2, 2, now, false)
	if !strings.Contains(dupOut, "Active Days: 2/3 (95 min)") {
		t.Errorf("expected summed duplicate dates 2/3 (95 min), got:\n%s", dupOut)
	}

	// 3. DST spring-forward window (if America/New_York tzdata is available) renders cleanly without week drift
	if nyLoc, err := time.LoadLocation("America/New_York"); err == nil {
		springEnd := time.Date(2026, 4, 15, 12, 0, 0, 0, nyLoc)
		dstOut := FormatHeatmap(nil, 120, 0, 0, springEnd, false)
		if !strings.Contains(dstOut, "90-Day Focus Contribution Heatmap (14 weeks") &&
			!strings.Contains(dstOut, "90-Day Focus Contribution Heatmap (13 weeks") {
			t.Errorf("unexpected DST heatmap week count:\n%s", dstOut)
		}
	}
}
