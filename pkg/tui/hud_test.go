package tui

import (
	"strings"
	"testing"

	"github.com/aloneangel45/focusflow/pkg/storage"
)

func containsEmoji(s string) bool {
	for _, r := range s {
		if (r >= 0x1F000 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) || r == 0xFE0F {
			return true
		}
	}
	return false
}

func TestHUD_BannerProgressAndDailyChart_ZeroEmojis(t *testing.T) {
	banner := FormatBanner("2.2.0")
	if !strings.Contains(banner, "v2.2.0") {
		t.Errorf("expected version in banner")
	}
	if containsEmoji(banner) {
		t.Errorf("banner contains forbidden emoji/dingbat characters:\n%s", banner)
	}

	prog := FormatProgress(300, 600, "dev", 4)
	if !strings.Contains(prog, "50%") || !strings.Contains(prog, "05:00/10:00") || containsEmoji(prog) {
		t.Errorf("unexpected progress string: %q", prog)
	}

	cycleProg := FormatCycleProgress(60, 120, "study", 2, 2, 4)
	if !strings.Contains(cycleProg, "[CYCLE 2/4]") || !strings.Contains(cycleProg, "50%") || containsEmoji(cycleProg) {
		t.Errorf("unexpected cycle progress string: %q", cycleProg)
	}

	breakProg := FormatBreakProgress(30, 60, 1, 3, false)
	if !strings.Contains(breakProg, "[CYCLE 1/3]") || !strings.Contains(breakProg, "PAUSED [BREAK]") || containsEmoji(breakProg) {
		t.Errorf("unexpected break progress string: %q", breakProg)
	}

	goalLine := FormatDailyGoalLine(90, 120, 75.0)
	if goalLine != "Today: 90 / 120 min [75%]" || containsEmoji(goalLine) {
		t.Errorf("unexpected daily goal line: %q", goalLine)
	}

	chart := FormatDailyChart([]storage.DailyStat{
		{Date: "2026-11-09", Minutes: 0, Sprints: 0},
		{Date: "2026-11-10", Minutes: 50, Sprints: 1},
	})
	if !strings.Contains(chart, "2026-11-10") || !strings.Contains(chart, "50 min") || containsEmoji(chart) {
		t.Errorf("unexpected chart output:\n%s", chart)
	}
}
