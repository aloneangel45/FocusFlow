package tui

import (
	"fmt"
	"strings"
)

func PrintBanner() {
	banner := `
  ███████╗ ██████╗  ██████╗██╗   ██╗███████╗███████╗██╗      ██████╗ ██╗    ██╗
  ██╔════╝██╔═══██╗██╔════╝██║   ██║██╔════╝██╔════╝██║     ██╔═══██╗██║    ██║
  █████╗  ██║   ██║██║     ██║   ██║███████╗█████╗  ██║     ██║   ██║██║ █╗ ██║
  ██╔══╝  ██║   ██║██║     ██║   ██║╚════██║██╔══╝  ██║     ██║   ██║██║███╗██║
  ██║     ╚██████╔╝╚██████╗╚██████╔╝███████║██║     ███████╗╚██████╔╝╚███╔███╔╝
  ╚═╝      ╚═════╝  ╚═════╝ ╚═════╝ ╚══════╝╚═╝     ╚══════╝ ╚═════╝  ╚══╝╚══╝ 
`
	fmt.Println(banner)
	fmt.Println("  🛡️  Kernel-Level Deep Work & Distraction Shield  v2.0.0")
	fmt.Println("  " + strings.Repeat("─", 68) + "\n")
}

func RenderProgress(elapsedSeconds, totalSeconds int, profileName string, blockedCount int) {
	percent := 0
	if totalSeconds > 0 {
		percent = (elapsedSeconds * 100) / totalSeconds
	}
	if percent > 100 {
		percent = 100
	}

	barWidth := 30
	filled := (barWidth * percent) / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	elapsedMin := elapsedSeconds / 60
	elapsedSec := elapsedSeconds % 60
	totalMin := totalSeconds / 60
	totalSec := totalSeconds % 60

	fmt.Printf("\r  [%02d:%02d/%02d:%02d] [%s] %3d%% | Profile: %-8s | Blocked Distractions: %d ",
		elapsedMin, elapsedSec, totalMin, totalSec, bar, percent, profileName, blockedCount)
}
