package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aloneangel45/focusflow/pkg/engine"
	"github.com/aloneangel45/focusflow/pkg/profile"
	"github.com/aloneangel45/focusflow/pkg/storage"
	"github.com/aloneangel45/focusflow/pkg/tui"
)

const Version = "2.0.0"

func main() {
	if len(os.Args) < 2 {
		tui.PrintBanner()
		printUsage()
		return
	}

	command := strings.ToLower(os.Args[1])

	switch command {
	case "start":
		handleStart(os.Args[2:])
	case "stop":
		handleStop()
	case "status":
		handleStatus()
	case "flush":
		handleFlush()
	case "stats":
		handleStats()
	case "version", "--version", "-v":
		fmt.Printf("FocusFlow v%s (Windows Win32 Native)\n", Version)
	default:
		tui.PrintBanner()
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Usage: focusflow <command> [options]")
	fmt.Println("\nAvailable Commands:")
	fmt.Println("  start     Start a deep work sprint with atomic blocking active")
	fmt.Println("  stop      Rollback hosts file and restore unrestricted system access")
	fmt.Println("  status    Inspect whether FocusFlow active blocklist is currently engaged")
	fmt.Println("  flush     Purge local DNS resolver cache immediately")
	fmt.Println("  stats     Display lifetime focus session metrics and streak summary")
	fmt.Println("  version   Print application version")
	fmt.Println("\nStart Options:")
	fmt.Println("  --profile    Profile to activate: dev, study, hardcore (default: study)")
	fmt.Println("  --minutes    Duration in minutes (default: profile default)")
	fmt.Println("  --strict     Strictness level: relaxed, standard, hardcore (default: standard)")
}

func handleStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	profileName := fs.String("profile", "study", "Profile name")
	minutes := fs.Int("minutes", 0, "Duration in minutes")
	strictness := fs.String("strict", "standard", "Strictness level")
	_ = fs.Parse(args)

	tui.PrintBanner()

	prof := profile.LoadOrDefault(*profileName)
	durationMin := prof.DefaultWorkMinutes
	if *minutes > 0 {
		durationMin = *minutes
	}
	totalSeconds := durationMin * 60

	strictLevel := engine.StrictnessStandard
	switch strings.ToLower(*strictness) {
	case "hardcore":
		strictLevel = engine.StrictnessHardcore
	case "relaxed":
		strictLevel = engine.StrictnessRelaxed
	}

	fmt.Printf("  ▶ Starting %s session (%d minutes | Strictness: %s)\n",
		prof.Name, durationMin, strictLevel)

	hosts := engine.NewHostsManager()
	dns := &engine.DnsEngine{}
	procSup := engine.NewProcessSupervisor(prof.BlockedProcesses)
	antiCheat := engine.NewAntiCheatGuard(strictLevel)

	// Step 1: Engage Hosts Blocklist
	fmt.Print("  • Engaging atomic hosts blocklist... ")
	allDomains := append([]string{}, prof.BlockedDomains...)
	allDomains = append(allDomains, dns.GetDoHBypassDomains()...)

	if err := hosts.ApplyBlocklist(allDomains); err != nil {
		fmt.Printf("FAILED (%v)\n", err)
		fmt.Println("  ⚠ Please run terminal as Administrator to modify system hosts.")
	} else {
		fmt.Println("OK (Backup secured with SHA-256)")
	}

	// Step 2: Flush DNS resolver cache
	fmt.Print("  • Purging DNS resolver cache (Anti-DoH)... ")
	_ = dns.FlushResolverCache()
	fmt.Println("OK")

	// Setup graceful Ctrl+C interrupt handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n\n  ⚠ Interrupt signal received! Rolling back hosts...")
		_ = hosts.Rollback()
		_ = dns.FlushResolverCache()
		fmt.Println("  ✔ System access cleanly restored. Exiting.")
		os.Exit(0)
	}()

	// Step 3: Run interactive loop
	elapsedSeconds := 0
	totalBlocked := 0

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for elapsedSeconds < totalSeconds {
		<-ticker.C
		elapsedSeconds++

		// Process surveillance scan
		terminatedProcs := procSup.ScanAndTerminate()
		antiCheatProcs := antiCheat.Enforce()

		hits := len(terminatedProcs) + len(antiCheatProcs)
		if hits > 0 {
			totalBlocked += hits
			for _, p := range append(terminatedProcs, antiCheatProcs...) {
				fmt.Printf("\n  ✘ Terminated distraction: %s\n", p)
			}
		}

		tui.RenderProgress(elapsedSeconds, totalSeconds, prof.Name, totalBlocked)
	}

	fmt.Println("\n\n  🎉 Sprint Complete! Take a well-deserved break.")

	// Clean rollback
	_ = hosts.Rollback()
	_ = dns.FlushResolverCache()

	// Record into storage
	if store, err := storage.NewStore(); err == nil {
		_ = store.RecordSession(prof.Name, durationMin, true, totalBlocked)
	}

	fmt.Println("  ✔ All system protections rolled back cleanly.")
}

func handleStop() {
	fmt.Println("  • Rolling back hosts file and flushing DNS...")
	hosts := engine.NewHostsManager()
	dns := &engine.DnsEngine{}

	if err := hosts.Rollback(); err != nil {
		fmt.Printf("  ✘ Rollback failed: %v\n", err)
	} else {
		_ = dns.FlushResolverCache()
		fmt.Println("  ✔ Clean rollback complete. Unrestricted access restored.")
	}
}

func handleStatus() {
	hosts := engine.NewHostsManager()
	if hosts.IsActive() {
		fmt.Println("  • FocusFlow guard is currently ACTIVE (Lockdown Engaged).")
	} else {
		fmt.Println("  • FocusFlow guard is INACTIVE (Unrestricted).")
	}
}

func handleFlush() {
	fmt.Print("  • Purging DNS resolver cache... ")
	dns := &engine.DnsEngine{}
	if err := dns.FlushResolverCache(); err != nil {
		fmt.Printf("FAILED (%v)\n", err)
	} else {
		fmt.Println("OK")
	}
}

func handleStats() {
	fmt.Println("  📊 FocusFlow Historical Metrics:")
	store, err := storage.NewStore()
	if err != nil {
		fmt.Printf("  ✘ Could not access metrics store: %v\n", err)
		return
	}

	sessions, minutes, blocked, err := store.GetTotalStats()
	if err != nil {
		fmt.Printf("  ✘ Could not read stats: %v\n", err)
		return
	}

	hours := float64(minutes) / 60.0
	fmt.Printf("    • Total Completed Sprints: %d\n", sessions)
	fmt.Printf("    • Total Deep Work Hours:   %.1f hrs\n", hours)
	fmt.Printf("    • Distractions Blocked:    %d\n", blocked)
}
