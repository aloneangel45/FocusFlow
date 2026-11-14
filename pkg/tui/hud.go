package tui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aloneangel45/focusflow/pkg/storage"
)

const DefaultVersion = "2.2.0"

func FormatBanner(version string) string {
	if strings.TrimSpace(version) == "" {
		version = DefaultVersion
	}
	banner := `
  ███████╗ ██████╗  ██████╗██╗   ██╗███████╗███████╗██╗      ██████╗ ██╗    ██╗
  ██╔════╝██╔═══██╗██╔════╝██║   ██║██╔════╝██╔════╝██║     ██╔═══██╗██║    ██║
  █████╗  ██║   ██║██║     ██║   ██║███████╗█████╗  ██║     ██║   ██║██║ █╗ ██║
  ██╔══╝  ██║   ██║██║     ██║   ██║╚════██║██╔══╝  ██║     ██║   ██║██║███╗██║
  ██║     ╚██████╔╝╚██████╗╚██████╔╝███████║██║     ███████╗╚██████╔╝╚███╔███╔╝
  ╚═╝      ╚═════╝  ╚═════╝ ╚═════╝ ╚══════╝╚═╝     ╚══════╝ ╚═════╝  ╚══╝╚══╝`
	return fmt.Sprintf("%s\n\n  [FOCUSFLOW] Native Deep Work & Distraction Shield  v%s\n  %s\n\n",
		banner, version, strings.Repeat("─", 72))
}

func PrintBanner() {
	PrintBannerTo(os.Stdout, DefaultVersion)
}

func PrintBannerWithVersion(version string) {
	PrintBannerTo(os.Stdout, version)
}

func PrintBannerTo(w io.Writer, version string) {
	_, _ = fmt.Fprint(w, FormatBanner(version))
}

func buildBar(elapsedSeconds, totalSeconds, barWidth int) (string, int) {
	percent := 0
	if totalSeconds > 0 {
		percent = (elapsedSeconds * 100) / totalSeconds
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := (barWidth * percent) / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	return bar, percent
}

func FormatProgress(elapsedSeconds, totalSeconds int, profileName string, blockedCount int) string {
	bar, percent := buildBar(elapsedSeconds, totalSeconds, 30)

	elapsedMin := elapsedSeconds / 60
	elapsedSec := elapsedSeconds % 60
	totalMin := totalSeconds / 60
	totalSec := totalSeconds % 60

	return fmt.Sprintf("\r  [%02d:%02d/%02d:%02d] [%s] %3d%% | Profile: %-8s | Blocked: %d ",
		elapsedMin, elapsedSec, totalMin, totalSec, bar, percent, profileName, blockedCount)
}

func FormatCycleProgress(elapsedSeconds, totalSeconds int, profileName string, blockedCount int, cycle, totalCycles int) string {
	if totalCycles <= 1 {
		return FormatProgress(elapsedSeconds, totalSeconds, profileName, blockedCount)
	}
	bar, percent := buildBar(elapsedSeconds, totalSeconds, 24)

	elapsedMin := elapsedSeconds / 60
	elapsedSec := elapsedSeconds % 60
	totalMin := totalSeconds / 60
	totalSec := totalSeconds % 60

	return fmt.Sprintf("\r  [CYCLE %d/%d] [%02d:%02d/%02d:%02d] [%s] %3d%% | Profile: %-8s | Blocked: %d ",
		cycle, totalCycles, elapsedMin, elapsedSec, totalMin, totalSec, bar, percent, profileName, blockedCount)
}

func FormatBreakProgress(elapsedSeconds, totalSeconds int, cycle, totalCycles int, continuousLockdown bool) string {
	bar, percent := buildBar(elapsedSeconds, totalSeconds, 24)

	elapsedMin := elapsedSeconds / 60
	elapsedSec := elapsedSeconds % 60
	totalMin := totalSeconds / 60
	totalSec := totalSeconds % 60

	shieldStatus := "PAUSED [BREAK]"
	if continuousLockdown {
		shieldStatus = "CONTINUOUS [HARDCORE]"
	}

	return fmt.Sprintf("\r  [CYCLE %d/%d] [BREAK %02d:%02d/%02d:%02d] [%s] %3d%% | Shield: %s ",
		cycle, totalCycles, elapsedMin, elapsedSec, totalMin, totalSec, bar, percent, shieldStatus)
}

func FormatDailyGoalLine(todayMinutes, goalMinutes int, progressPct float64) string {
	return fmt.Sprintf("Today: %d / %d min [%.0f%%]", todayMinutes, goalMinutes, progressPct)
}

func RenderProgress(elapsedSeconds, totalSeconds int, profileName string, blockedCount int) {
	_, _ = fmt.Fprint(os.Stdout, FormatProgress(elapsedSeconds, totalSeconds, profileName, blockedCount))
}

// FormatDailyChart renders a clean ASCII bar chart for focus minutes across N days.
func FormatDailyChart(daily []storage.DailyStat) string {
	if len(daily) == 0 {
		return "    (No daily history available)\n"
	}
	maxMin := 0
	for _, d := range daily {
		if d.Minutes > maxMin {
			maxMin = d.Minutes
		}
	}

	const maxWidth = 28
	var b strings.Builder
	for _, d := range daily {
		filled := 0
		if maxMin > 0 && d.Minutes > 0 {
			filled = (d.Minutes * maxWidth) / maxMin
			if filled == 0 {
				filled = 1
			}
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", maxWidth-filled)
		b.WriteString(fmt.Sprintf("    %s | %s | %4d min (%d sprints)\n", d.Date, bar, d.Minutes, d.Sprints))
	}
	return b.String()
}
