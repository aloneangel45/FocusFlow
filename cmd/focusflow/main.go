package main

import (
	"bufio"
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
		handleInteractiveMenu()
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
	case "menu":
		handleInteractiveMenu()
	case "list":
		handleListConfig()
	case "add":
		handleAdd(os.Args[2:])
	case "remove", "rm":
		handleRemove(os.Args[2:])
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
	fmt.Println("  menu                Launch interactive configuration & session menu")
	fmt.Println("  start               Start a deep work sprint with atomic blocking active")
	fmt.Println("  stop                Rollback hosts file and restore unrestricted system access")
	fmt.Println("  status              Inspect whether FocusFlow active blocklist is currently engaged")
	fmt.Println("  list                List all active profiles, custom blocked sites & apps")
	fmt.Println("  add site <domain>   Add custom domain to blocklist")
	fmt.Println("  add app <proc.exe>  Add custom process to blocklist")
	fmt.Println("  remove site <dom>   Remove custom domain from blocklist")
	fmt.Println("  remove app <proc>   Remove custom process from blocklist")
	fmt.Println("  flush               Purge local DNS resolver cache immediately")
	fmt.Println("  stats               Display lifetime focus session metrics and streak summary")
	fmt.Println("  version             Print application version")
	fmt.Println("\nStart Options:")
	fmt.Println("  --profile    Profile to activate: dev, study, hardcore (default: study)")
	fmt.Println("  --minutes    Duration in minutes (default: profile default)")
	fmt.Println("  --strict     Strictness level: relaxed, standard, hardcore (default: standard)")
}

func handleInteractiveMenu() {
	tui.PrintBanner()
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("  [ FOCUSFLOW INTERACTIVE DASHBOARD ]")
		fmt.Println("  1. ▶ Start Sprint (Study Mode - 25 min)")
		fmt.Println("  2. ▶ Start Sprint (Dev Mode - 50 min)")
		fmt.Println("  3. ▶ Start Sprint (Hardcore Aegis Lockdown)")
		fmt.Println("  4. 📋 List Blocked Sites & Applications")
		fmt.Println("  5. ➕ Add Custom Blocked Website (e.g. twitter.com)")
		fmt.Println("  6. ➕ Add Custom Blocked Application (e.g. discord.exe)")
		fmt.Println("  7. ➖ Remove Custom Blocked Item")
		fmt.Println("  8. 🛑 Emergency Stop / Rollback All Blocks")
		fmt.Println("  9. 📊 View Lifetime Productivity Statistics")
		fmt.Println("  0. 🚪 Exit")
		fmt.Print("\n  Select an option [0-9]: ")

		input, _ := reader.ReadString('\n')
		choice := strings.TrimSpace(input)

		switch choice {
		case "1":
			handleStart([]string{"--profile", "study", "--minutes", "25"})
			return
		case "2":
			handleStart([]string{"--profile", "dev", "--minutes", "50"})
			return
		case "3":
			handleStart([]string{"--profile", "hardcore", "--strict", "hardcore"})
			return
		case "4":
			handleListConfig()
		case "5":
			fmt.Print("  Enter domain to block (e.g. reddit.com): ")
			domain, _ := reader.ReadString('\n')
			domain = strings.TrimSpace(domain)
			if domain != "" {
				handleAdd([]string{"site", domain})
			}
		case "6":
			fmt.Print("  Enter application executable name (e.g. steam.exe): ")
			app, _ := reader.ReadString('\n')
			app = strings.TrimSpace(app)
			if app != "" {
				handleAdd([]string{"app", app})
			}
		case "7":
			fmt.Print("  Enter type to remove ('site' or 'app'): ")
			itemType, _ := reader.ReadString('\n')
			itemType = strings.TrimSpace(itemType)
			fmt.Print("  Enter name to remove: ")
			name, _ := reader.ReadString('\n')
			name = strings.TrimSpace(name)
			if itemType != "" && name != "" {
				handleRemove([]string{itemType, name})
			}
		case "8":
			handleStop()
		case "9":
			handleStats()
		case "0", "exit", "q":
			fmt.Println("  Goodbye! Stay focused.")
			return
		default:
			fmt.Println("  Invalid option. Please enter 0-9.")
		}
		fmt.Println("\n" + strings.Repeat("─", 60) + "\n")
	}
}

func handleListConfig() {
	cfg, err := profile.LoadConfig()
	if err != nil {
		fmt.Printf("Error reading config: %v\n", err)
		return
	}

	fmt.Println("\n=== FocusFlow Custom User Rules ===")
	fmt.Printf("Config File: %s\n\n", profile.GetConfigPath())

	fmt.Println("Custom Blocked Domains:")
	if len(cfg.CustomDomains) == 0 {
		fmt.Println("  (None. Use 'focusflow add site <domain>' to add)")
	} else {
		for _, d := range cfg.CustomDomains {
			fmt.Printf("  • %s\n", d)
		}
	}

	fmt.Println("\nCustom Blocked Applications:")
	if len(cfg.CustomProcesses) == 0 {
		fmt.Println("  (None. Use 'focusflow add app <proc.exe>' to add)")
	} else {
		for _, p := range cfg.CustomProcesses {
			fmt.Printf("  • %s\n", p)
		}
	}

	fmt.Println("\nStandard Profiles Available:")
	for name, p := range cfg.Profiles {
		fmt.Printf("  • [%s] %s (%d mins work / %d mins break)\n",
			name, p.Description, p.DefaultWorkMinutes, p.DefaultBreakMinutes)
	}
	fmt.Println()
}

func handleAdd(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: focusflow add site <domain>  OR  focusflow add app <proc.exe>")
		return
	}

	itemType := strings.ToLower(args[0])
	itemValue := strings.ToLower(strings.TrimSpace(args[1]))

	cfg, err := profile.LoadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	switch itemType {
	case "site", "domain", "web":
		for _, d := range cfg.CustomDomains {
			if d == itemValue {
				fmt.Printf("Domain '%s' is already in custom blocklist.\n", itemValue)
				return
			}
		}
		cfg.CustomDomains = append(cfg.CustomDomains, itemValue)
		if err := profile.SaveConfig(cfg); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
		} else {
			fmt.Printf("✔ Added '%s' to custom blocked websites.\n", itemValue)
		}
	case "app", "proc", "process", "exe":
		if !strings.HasSuffix(itemValue, ".exe") {
			itemValue = itemValue + ".exe"
		}
		for _, p := range cfg.CustomProcesses {
			if p == itemValue {
				fmt.Printf("Application '%s' is already in custom blocklist.\n", itemValue)
				return
			}
		}
		cfg.CustomProcesses = append(cfg.CustomProcesses, itemValue)
		if err := profile.SaveConfig(cfg); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
		} else {
			fmt.Printf("✔ Added '%s' to custom blocked applications.\n", itemValue)
		}
	default:
		fmt.Println("Invalid item type. Use 'site' or 'app'.")
	}
}

func handleRemove(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: focusflow remove site <domain>  OR  focusflow remove app <proc.exe>")
		return
	}

	itemType := strings.ToLower(args[0])
	itemValue := strings.ToLower(strings.TrimSpace(args[1]))

	cfg, err := profile.LoadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	switch itemType {
	case "site", "domain", "web":
		var updated []string
		found := false
		for _, d := range cfg.CustomDomains {
			if d == itemValue {
				found = true
			} else {
				updated = append(updated, d)
			}
		}
		if !found {
			fmt.Printf("Domain '%s' was not found in custom blocklist.\n", itemValue)
			return
		}
		cfg.CustomDomains = updated
		_ = profile.SaveConfig(cfg)
		fmt.Printf("✔ Removed '%s' from custom blocked websites.\n", itemValue)
	case "app", "proc", "process", "exe":
		if !strings.HasSuffix(itemValue, ".exe") {
			itemValue = itemValue + ".exe"
		}
		var updated []string
		found := false
		for _, p := range cfg.CustomProcesses {
			if p == itemValue {
				found = true
			} else {
				updated = append(updated, p)
			}
		}
		if !found {
			fmt.Printf("Application '%s' was not found in custom blocklist.\n", itemValue)
			return
		}
		cfg.CustomProcesses = updated
		_ = profile.SaveConfig(cfg)
		fmt.Printf("✔ Removed '%s' from custom blocked applications.\n", itemValue)
	default:
		fmt.Println("Invalid item type. Use 'site' or 'app'.")
	}
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
