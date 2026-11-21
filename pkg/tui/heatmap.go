package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aloneangel45/focusflow/pkg/storage"
)

const DefaultHeatmapDays = 90

var asciiHeatmapGlyphs = [5]string{" ", ".", "o", "O", "#"}

// ANSI 256-color Catppuccin-inspired green intensity ramp (levels 0..4)
var ansiHeatmapCells = [5]string{
	"\x1b[38;5;238m■\x1b[0m",
	"\x1b[38;5;22m■\x1b[0m",
	"\x1b[38;5;28m■\x1b[0m",
	"\x1b[38;5;34m■\x1b[0m",
	"\x1b[38;5;82m■\x1b[0m",
}

// ShouldUseColor returns false if --no-color was passed or NO_COLOR is set in the environment.
func ShouldUseColor(noColorFlag bool) bool {
	if noColorFlag {
		return false
	}
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	return true
}

// HeatmapIntensity maps daily focus minutes against dailyGoalMinutes to intensity levels 0..4.
func HeatmapIntensity(minutes, dailyGoalMinutes int) int {
	if minutes <= 0 {
		return 0
	}
	goal := storage.NormalizeDailyGoalMinutes(dailyGoalMinutes)
	ratio := float64(minutes) / float64(goal)
	switch {
	case ratio >= 1.0:
		return 4
	case ratio >= 0.70:
		return 3
	case ratio >= 0.35:
		return 2
	default:
		return 1
	}
}

func weekdayRowMondayFirst(wd time.Weekday) int {
	if wd == time.Sunday {
		return 6
	}
	return int(wd) - 1
}

// FormatHeatmap renders a GitHub-style contribution heatmap (default 90 days = 13 weeks x 7 days)
// of daily focus minutes vs dailyGoalMinutes, with month labels, legend, active streak, and longest streak.
// When useColor is false, it degrades cleanly to pure ASCII (" ", ".", "o", "O", "#") with zero ANSI escapes.
func FormatHeatmap(
	daily []storage.DailyStat,
	dailyGoalMinutes int,
	activeStreak int,
	longestStreak int,
	now time.Time,
	useColor bool,
) string {
	goal := storage.NormalizeDailyGoalMinutes(dailyGoalMinutes)
	loc := now.Location()
	endDay := time.Date(now.In(loc).Year(), now.In(loc).Month(), now.In(loc).Day(), 0, 0, 0, 0, loc)

	windowDays := len(daily)
	if windowDays <= 0 {
		windowDays = DefaultHeatmapDays
	}

	minutesByDate := make(map[string]int, len(daily))
	for _, stat := range daily {
		if stat.Minutes > 0 {
			minutesByDate[stat.Date] += stat.Minutes
		}
	}

	startDay := endDay.AddDate(0, 0, -(windowDays - 1))
	totalWindowMin := 0
	activeDays := 0
	for i := 0; i < windowDays; i++ {
		dKey := startDay.AddDate(0, 0, i).Format("2006-01-02")
		if mins := minutesByDate[dKey]; mins > 0 {
			totalWindowMin += mins
			activeDays++
		}
	}

	// Align grid start to the Monday on or before startDay
	gridStart := startDay.AddDate(0, 0, -weekdayRowMondayFirst(startDay.Weekday()))
	// Align grid end to the Sunday on or after endDay
	gridEnd := endDay.AddDate(0, 0, 6-weekdayRowMondayFirst(endDay.Weekday()))

	// Use pure UTC calendar midnights so DST spring-forward/fall-back transitions
	// never introduce fractional-hour drift in week counts.
	gridStartUTC := time.Date(gridStart.Year(), gridStart.Month(), gridStart.Day(), 0, 0, 0, 0, time.UTC)
	gridEndUTC := time.Date(gridEnd.Year(), gridEnd.Month(), gridEnd.Day(), 0, 0, 0, 0, time.UTC)
	totalGridDays := int(gridEndUTC.Sub(gridStartUTC)/(24*time.Hour)) + 1
	numWeeks := (totalGridDays + 6) / 7
	if numWeeks < 1 {
		numWeeks = 1
	}

	// Build month header across week columns (2 chars per column)
	monthRunes := make([]byte, numWeeks*2+4)
	for i := range monthRunes {
		monthRunes[i] = ' '
	}
	lastMonth := time.Month(0)
	lastLabelCol := -3
	for w := 0; w < numWeeks; w++ {
		weekMon := gridStart.AddDate(0, 0, w*7)
		// Find first in-window day in this week to pick accurate month transition
		refDay := weekMon
		for d := 0; d < 7; d++ {
			cand := weekMon.AddDate(0, 0, d)
			if !cand.Before(startDay) && !cand.After(endDay) {
				if cand.Day() == 1 || d == 0 {
					refDay = cand
					if cand.Day() == 1 {
						break
					}
				}
			}
		}
		if refDay.Before(startDay) {
			refDay = startDay
		}
		if refDay.Month() != lastMonth && w-lastLabelCol >= 2 {
			lbl := refDay.Format("Jan")
			pos := w * 2
			for k := 0; k < len(lbl) && pos+k < len(monthRunes); k++ {
				monthRunes[pos+k] = lbl[k]
			}
			lastMonth = refDay.Month()
			lastLabelCol = w
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("  %d-Day Focus Contribution Heatmap (%d weeks | Goal: %d min/day):\n", windowDays, numWeeks, goal))
	b.WriteString("        " + strings.TrimRight(string(monthRunes), " ") + "\n")

	rowLabels := [7]string{"Mon", "   ", "Wed", "   ", "Fri", "   ", "Sun"}
	border := strings.Repeat("-", numWeeks*2+1)
	if !useColor {
		b.WriteString("       +" + border + "+\n")
	}

	for row := 0; row < 7; row++ {
		if useColor {
			b.WriteString(fmt.Sprintf("    %s ", rowLabels[row]))
		} else {
			b.WriteString(fmt.Sprintf("    %s| ", rowLabels[row]))
		}
		for w := 0; w < numWeeks; w++ {
			cellDate := gridStart.AddDate(0, 0, w*7+row)
			if cellDate.Before(startDay) || cellDate.After(endDay) {
				b.WriteString("  ")
				continue
			}
			mins := minutesByDate[cellDate.Format("2006-01-02")]
			lvl := HeatmapIntensity(mins, goal)
			if useColor {
				b.WriteString(ansiHeatmapCells[lvl])
				b.WriteByte(' ')
			} else {
				b.WriteString(asciiHeatmapGlyphs[lvl])
				b.WriteByte(' ')
			}
		}
		if !useColor {
			b.WriteByte('|')
		}
		b.WriteByte('\n')
	}

	if !useColor {
		b.WriteString("       +" + border + "+\n")
	}

	// Legend
	if useColor {
		b.WriteString(fmt.Sprintf(
			"    Legend: Less  %s %s %s %s %s  More\n",
			ansiHeatmapCells[0], ansiHeatmapCells[1], ansiHeatmapCells[2], ansiHeatmapCells[3], ansiHeatmapCells[4],
		))
	} else {
		b.WriteString("    Legend: Less  [ ] [.] [o] [O] [#]  More\n")
	}

	b.WriteString(fmt.Sprintf(
		"    Streaks: Active %d days | Longest %d days | Active Days: %d/%d (%d min)\n",
		activeStreak, longestStreak, activeDays, windowDays, totalWindowMin,
	))
	return b.String()
}
