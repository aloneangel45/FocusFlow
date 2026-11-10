package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/aloneangel45/focusflow/pkg/engine"
	"github.com/aloneangel45/focusflow/pkg/profile"
	"github.com/aloneangel45/focusflow/pkg/storage"
	"github.com/aloneangel45/focusflow/pkg/tui"
)

// Version is set at build time via -ldflags "-X main.Version=..."
var Version = "2.1.0"

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func stripElevateFlag(args []string) ([]string, bool) {
	cleaned := make([]string, 0, len(args))
	elevate := false
	for _, a := range args {
		if a == "--elevate" || a == "-elevate" {
			elevate = true
			continue
		}
		cleaned = append(cleaned, a)
	}
	return cleaned, elevate
}

func runStartupAutoRecovery() (*engine.RecoveryResult, error) {
	hosts := engine.NewHostsManager()
	return hosts.RecoverIfOrphaned(func(sessionID string) {
		if store, err := storage.NewStore(); err == nil {
			_, _ = store.MarkOrphanedSession(sessionID, time.Now().UTC())
		}
	})
}

func runCLI(rawArgs []string, stdin io.Reader, stdout, stderr io.Writer) int {
	args, wantElevate := stripElevateFlag(rawArgs)
	if wantElevate && !engine.IsElevated() {
		if err := engine.ElevateSelf(args); err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Elevation failed: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintln(stdout, "[OK] Launched elevated FocusFlow process.")
		return 0
	}

	if len(args) == 0 {
		_, _ = runStartupAutoRecovery()
		return handleInteractiveMenu(stdin, stdout, stderr)
	}

	command := strings.ToLower(strings.TrimSpace(args[0]))
	subArgs := args[1:]

	// Run automatic orphan journal recovery before regular commands
	if command != "version" && command != "--version" && command != "-v" &&
		command != "help" && command != "--help" && command != "-h" &&
		command != "doctor" && command != "recover" {
		_, _ = runStartupAutoRecovery()
	}

	switch command {
	case "start":
		return handleStart(subArgs, stdout, stderr)
	case "stop":
		return handleStop(stdout, stderr)
	case "status":
		return handleStatus(subArgs, stdout, stderr)
	case "flush":
		return handleFlush(stdout, stderr)
	case "stats":
		return handleStats(subArgs, stdout, stderr)
	case "export":
		return handleExport(subArgs, stdout, stderr)
	case "doctor":
		return handleDoctor(subArgs, stdout, stderr)
	case "recover":
		return handleRecover(subArgs, stdout, stderr)
	case "menu":
		return handleInteractiveMenu(stdin, stdout, stderr)
	case "list":
		return handleListConfig(subArgs, stdout, stderr)
	case "add":
		return handleAdd(subArgs, stdout, stderr)
	case "remove", "rm":
		return handleRemove(subArgs, stdout, stderr)
	case "version", "--version", "-v":
		_, _ = fmt.Fprintf(stdout, "FocusFlow v%s (%s/%s)\n", Version, runtime.GOOS, runtime.GOARCH)
		return 0
	case "help", "--help", "-h":
		tui.PrintBannerTo(stdout, Version)
		printUsage(stdout)
		return 0
	default:
		tui.PrintBannerTo(stdout, Version)
		_, _ = fmt.Fprintf(stderr, "[ERR] Unknown command: %s\n\n", command)
		printUsage(stdout)
		return 1
	}
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: focusflow [--elevate] <command> [options]")
	_, _ = fmt.Fprintln(w, "\nAvailable Commands:")
	_, _ = fmt.Fprintln(w, "  menu                            Launch interactive configuration & session dashboard")
	_, _ = fmt.Fprintln(w, "  start                           Start a deep work sprint with crash-safe blocking")
	_, _ = fmt.Fprintln(w, "  stop                            Rollback hosts file and restore unrestricted access")
	_, _ = fmt.Fprintln(w, "  status [--json]                 Inspect shield state, privileges, and active journal")
	_, _ = fmt.Fprintln(w, "  list [--json]                   List all active profiles, custom blocked sites & apps")
	_, _ = fmt.Fprintln(w, "  add site <domain>               Add validated RFC-1123 custom domain to blocklist")
	_, _ = fmt.Fprintln(w, "  add app <proc.exe>              Add custom process to blocklist (denylist protected)")
	_, _ = fmt.Fprintln(w, "  remove site <domain>            Remove custom domain from blocklist")
	_, _ = fmt.Fprintln(w, "  remove app <proc.exe>           Remove custom process from blocklist")
	_, _ = fmt.Fprintln(w, "  stats [--days 7] [--json]       Display focus metrics, streak, completion rate & chart")
	_, _ = fmt.Fprintln(w, "  export [--format json|csv]      Export session telemetry (--out <file>)")
	_, _ = fmt.Fprintln(w, "  doctor [--json]                 Audit privileges, hosts integrity, and journal health")
	_, _ = fmt.Fprintln(w, "  recover [--force] [--json]      Recover orphaned crash/reboot hosts state")
	_, _ = fmt.Fprintln(w, "  flush                           Purge local DNS resolver cache immediately")
	_, _ = fmt.Fprintln(w, "  version                         Print application version")
	_, _ = fmt.Fprintln(w, "\nGlobal / Start Options:")
	_, _ = fmt.Fprintln(w, "  --elevate                       Request Administrator/sudo privilege elevation")
	_, _ = fmt.Fprintln(w, "  --profile <name>                Profile: study, dev, hardcore (default: study)")
	_, _ = fmt.Fprintln(w, "  --minutes <n>                   Duration in minutes (default: profile default)")
	_, _ = fmt.Fprintln(w, "  --strict <level>                Strictness: relaxed, standard, hardcore (default: standard)")
}

func shieldModeString(elevated bool) string {
	if elevated {
		return "FULL SHIELD [HOSTS + PROC]"
	}
	return "PROCESS-ONLY [UNPRIVILEGED]"
}

func handleInteractiveMenu(stdin io.Reader, stdout, stderr io.Writer) int {
	tui.PrintBannerTo(stdout, Version)
	reader := bufio.NewReader(stdin)

	for {
		elevated := engine.IsElevated()
		_, _ = fmt.Fprintf(stdout, "  [ FOCUSFLOW INTERACTIVE DASHBOARD | %s ]\n", shieldModeString(elevated))
		_, _ = fmt.Fprintln(stdout, "  1. [START] Sprint (Study Mode - 25 min)")
		_, _ = fmt.Fprintln(stdout, "  2. [START] Sprint (Dev Mode - 50 min)")
		_, _ = fmt.Fprintln(stdout, "  3. [START] Sprint (Hardcore Aegis Lockdown)")
		_, _ = fmt.Fprintln(stdout, "  4. [LIST]  Blocked Sites & Applications")
		_, _ = fmt.Fprintln(stdout, "  5. [ADD]   Custom Blocked Website (e.g. twitter.com)")
		_, _ = fmt.Fprintln(stdout, "  6. [ADD]   Custom Blocked Application (e.g. discord.exe)")
		_, _ = fmt.Fprintln(stdout, "  7. [REM]   Remove Custom Blocked Item")
		_, _ = fmt.Fprintln(stdout, "  8. [STOP]  Emergency Stop / Rollback All Blocks")
		_, _ = fmt.Fprintln(stdout, "  9. [STATS] View Productivity Analytics & 7-Day Chart")
		_, _ = fmt.Fprintln(stdout, "  0. [EXIT]  Quit")
		_, _ = fmt.Fprint(stdout, "\n  Select an option [0-9]: ")

		input, err := reader.ReadString('\n')
		if err != nil && len(input) == 0 {
			return 0
		}
		choice := strings.TrimSpace(input)

		switch choice {
		case "1":
			return handleStart([]string{"--profile", "study", "--minutes", "25"}, stdout, stderr)
		case "2":
			return handleStart([]string{"--profile", "dev", "--minutes", "50"}, stdout, stderr)
		case "3":
			return handleStart([]string{"--profile", "hardcore", "--strict", "hardcore"}, stdout, stderr)
		case "4":
			_ = handleListConfig(nil, stdout, stderr)
		case "5":
			_, _ = fmt.Fprint(stdout, "  Enter domain to block (e.g. reddit.com): ")
			domain, _ := reader.ReadString('\n')
			domain = strings.TrimSpace(domain)
			if domain != "" {
				_ = handleAdd([]string{"site", domain}, stdout, stderr)
			}
		case "6":
			_, _ = fmt.Fprint(stdout, "  Enter application executable name (e.g. steam.exe): ")
			app, _ := reader.ReadString('\n')
			app = strings.TrimSpace(app)
			if app != "" {
				_ = handleAdd([]string{"app", app}, stdout, stderr)
			}
		case "7":
			_, _ = fmt.Fprint(stdout, "  Enter type to remove ('site' or 'app'): ")
			itemType, _ := reader.ReadString('\n')
			itemType = strings.TrimSpace(itemType)
			_, _ = fmt.Fprint(stdout, "  Enter name to remove: ")
			name, _ := reader.ReadString('\n')
			name = strings.TrimSpace(name)
			if itemType != "" && name != "" {
				_ = handleRemove([]string{itemType, name}, stdout, stderr)
			}
		case "8":
			_ = handleStop(stdout, stderr)
		case "9":
			_ = handleStats(nil, stdout, stderr)
		case "0", "exit", "q":
			_, _ = fmt.Fprintln(stdout, "  [OK] Session closed.")
			return 0
		default:
			_, _ = fmt.Fprintln(stdout, "  [WARN] Invalid option. Enter 0-9.")
		}
		_, _ = fmt.Fprintln(stdout, "\n"+strings.Repeat("─", 68)+"\n")
	}
}

type listJSONOutput struct {
	ConfigPath      string                           `json:"config_path"`
	StateDir        string                           `json:"state_dir"`
	CustomDomains   []string                         `json:"custom_domains"`
	CustomProcesses []string                         `json:"custom_processes"`
	Profiles        map[string]*profile.FocusProfile `json:"profiles"`
}

func handleListConfig(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "Output configuration as JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	cfg, err := profile.LoadConfig()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "[ERR] Error reading config: %v\n", err)
		return 1
	}

	if *asJSON {
		out := listJSONOutput{
			ConfigPath:      profile.GetConfigPath(),
			StateDir:        profile.GetConfigDir(),
			CustomDomains:   cfg.CustomDomains,
			CustomProcesses: cfg.CustomProcesses,
			Profiles:        cfg.Profiles,
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}

	_, _ = fmt.Fprintln(stdout, "\n=== FocusFlow Configuration & Rules ===")
	_, _ = fmt.Fprintf(stdout, "State Dir:   %s\n", profile.GetConfigDir())
	_, _ = fmt.Fprintf(stdout, "Config File: %s\n\n", profile.GetConfigPath())

	_, _ = fmt.Fprintln(stdout, "Custom Blocked Domains:")
	if len(cfg.CustomDomains) == 0 {
		_, _ = fmt.Fprintln(stdout, "  (None. Use 'focusflow add site <domain>' to add)")
	} else {
		for _, d := range cfg.CustomDomains {
			_, _ = fmt.Fprintf(stdout, "  - %s\n", d)
		}
	}

	_, _ = fmt.Fprintln(stdout, "\nCustom Blocked Applications:")
	if len(cfg.CustomProcesses) == 0 {
		_, _ = fmt.Fprintln(stdout, "  (None. Use 'focusflow add app <proc.exe>' to add)")
	} else {
		for _, p := range cfg.CustomProcesses {
			_, _ = fmt.Fprintf(stdout, "  - %s\n", p)
		}
	}

	_, _ = fmt.Fprintln(stdout, "\nProfiles Available:")
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p := cfg.Profiles[name]
		_, _ = fmt.Fprintf(stdout, "  - [%s] %s (%d min work / %d min break)\n",
			name, p.Description, p.DefaultWorkMinutes, p.DefaultBreakMinutes)
	}
	_, _ = fmt.Fprintln(stdout)
	return 0
}

func handleAdd(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintln(stderr, "Usage: focusflow add site <domain>  OR  focusflow add app <proc.exe>")
		return 1
	}

	itemType := strings.ToLower(strings.TrimSpace(args[0]))
	rawValue := args[1]

	cfg, err := profile.LoadConfig()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "[ERR] Error loading config: %v\n", err)
		return 1
	}

	switch itemType {
	case "site", "domain", "web":
		cleanDomain, err := engine.SanitizeDomain(rawValue)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Invalid domain %q: %v\n", rawValue, err)
			return 1
		}
		for _, d := range cfg.CustomDomains {
			if d == cleanDomain {
				_, _ = fmt.Fprintf(stdout, "[INFO] Domain '%s' is already in custom blocklist.\n", cleanDomain)
				return 0
			}
		}
		cfg.CustomDomains = append(cfg.CustomDomains, cleanDomain)
		if err := profile.SaveConfig(cfg); err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Error saving config: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "[OK] Added '%s' to custom blocked websites.\n", cleanDomain)
		return 0

	case "app", "proc", "process", "exe":
		cleanProc, err := engine.NormalizeProcessName(rawValue)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Cannot block process %q: %v\n", rawValue, err)
			return 1
		}
		for _, p := range cfg.CustomProcesses {
			if p == cleanProc {
				_, _ = fmt.Fprintf(stdout, "[INFO] Application '%s' is already in custom blocklist.\n", cleanProc)
				return 0
			}
		}
		cfg.CustomProcesses = append(cfg.CustomProcesses, cleanProc)
		if err := profile.SaveConfig(cfg); err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Error saving config: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "[OK] Added '%s' to custom blocked applications.\n", cleanProc)
		return 0

	default:
		_, _ = fmt.Fprintln(stderr, "[ERR] Invalid item type. Use 'site' or 'app'.")
		return 1
	}
}

func handleRemove(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintln(stderr, "Usage: focusflow remove site <domain>  OR  focusflow remove app <proc.exe>")
		return 1
	}

	itemType := strings.ToLower(strings.TrimSpace(args[0]))
	rawValue := strings.TrimSpace(args[1])

	cfg, err := profile.LoadConfig()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "[ERR] Error loading config: %v\n", err)
		return 1
	}

	switch itemType {
	case "site", "domain", "web":
		target := strings.ToLower(rawValue)
		if clean, err := engine.SanitizeDomain(rawValue); err == nil {
			target = clean
		}
		var updated []string
		found := false
		for _, d := range cfg.CustomDomains {
			if d == target {
				found = true
			} else {
				updated = append(updated, d)
			}
		}
		if !found {
			_, _ = fmt.Fprintf(stderr, "[ERR] Domain '%s' was not found in custom blocklist.\n", target)
			return 1
		}
		cfg.CustomDomains = updated
		if err := profile.SaveConfig(cfg); err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Error saving config: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "[OK] Removed '%s' from custom blocked websites.\n", target)
		return 0

	case "app", "proc", "process", "exe":
		target := strings.ToLower(rawValue)
		if !strings.HasSuffix(target, ".exe") {
			target += ".exe"
		}
		var updated []string
		found := false
		for _, p := range cfg.CustomProcesses {
			if p == target {
				found = true
			} else {
				updated = append(updated, p)
			}
		}
		if !found {
			_, _ = fmt.Fprintf(stderr, "[ERR] Application '%s' was not found in custom blocklist.\n", target)
			return 1
		}
		cfg.CustomProcesses = updated
		if err := profile.SaveConfig(cfg); err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Error saving config: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "[OK] Removed '%s' from custom blocked applications.\n", target)
		return 0

	default:
		_, _ = fmt.Fprintln(stderr, "[ERR] Invalid item type. Use 'site' or 'app'.")
		return 1
	}
}

func handleStart(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profileName := fs.String("profile", "study", "Profile name")
	minutes := fs.Int("minutes", 0, "Duration in minutes")
	secondsOverride := fs.Int("seconds", 0, "Duration override in seconds (for rapid testing)")
	strictness := fs.String("strict", "", "Strictness level: relaxed, standard, hardcore")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	tui.PrintBannerTo(stdout, Version)

	prof := profile.LoadOrDefault(*profileName)
	durationMin := prof.DefaultWorkMinutes
	if *minutes > 0 {
		durationMin = *minutes
	}
	totalSeconds := durationMin * 60
	if *secondsOverride > 0 {
		totalSeconds = *secondsOverride
		if durationMin <= 0 {
			durationMin = 1
		}
	}

	strictLevel := engine.StrictnessStandard
	if prof.Name == "hardcore" && *strictness == "" {
		strictLevel = engine.StrictnessHardcore
	}
	switch strings.ToLower(strings.TrimSpace(*strictness)) {
	case "hardcore":
		strictLevel = engine.StrictnessHardcore
	case "relaxed":
		strictLevel = engine.StrictnessRelaxed
	case "standard":
		strictLevel = engine.StrictnessStandard
	}

	startTime := time.Now().UTC()
	expectedEnd := startTime.Add(time.Duration(totalSeconds) * time.Second)
	sessionID := storage.GenerateSessionID()

	store, storeErr := storage.NewStore()
	if storeErr == nil {
		_ = store.StartSession(sessionID, prof.Name, durationMin, startTime)
	}

	_, _ = fmt.Fprintf(stdout, "  [ACTIVE] Starting '%s' sprint (%d min | Strictness: %s | Session: %s)\n",
		prof.Name, durationMin, strictLevel, sessionID)

	hosts := engine.NewHostsManager()
	dns := &engine.DnsEngine{}
	procSup := engine.NewProcessSupervisor(prof.BlockedProcesses)
	antiCheat := engine.NewAntiCheatGuard(strictLevel)

	allDomains := append([]string{}, prof.BlockedDomains...)
	allDomains = append(allDomains, dns.GetDoHBypassDomains()...)

	sessionMeta := engine.SessionMeta{
		SessionID:     sessionID,
		PID:           os.Getpid(),
		StartedAt:     startTime,
		ExpectedEndAt: expectedEnd,
		Profile:       prof.Name,
	}

	hostsEngaged := false
	_, _ = fmt.Fprint(stdout, "  [INFO] Engaging dual-stack atomic hosts sinkhole... ")
	if err := hosts.ApplyBlocklistWithSession(allDomains, sessionMeta); err != nil {
		_, _ = fmt.Fprintf(stdout, "SKIPPED (%v)\n", err)
		_, _ = fmt.Fprintln(stdout, "  [WARN] Shield Mode: PROCESS-ONLY [UNPRIVILEGED] (re-run with --elevate for hosts sinkhole)")
	} else {
		hostsEngaged = true
		_, _ = fmt.Fprintln(stdout, "[OK] FULL SHIELD [HOSTS + PROC] (SHA-256 journaled)")
		_, _ = fmt.Fprint(stdout, "  [INFO] Purging DNS resolver cache (Anti-DoH)... ")
		_ = dns.FlushResolverCache()
		_, _ = fmt.Fprintln(stdout, "[OK]")
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	totalBlocked := 0
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	deadline := time.Now().Add(time.Duration(totalSeconds) * time.Second)
	tickCount := 0

	for {
		select {
		case <-sigChan:
			elapsed := int(time.Since(startTime).Seconds())
			if elapsed < 0 {
				elapsed = 0
			}
			_, _ = fmt.Fprintln(stdout, "\n\n  [WARN] Interrupt signal received. Rolling back system state...")
			if hostsEngaged {
				_ = hosts.Rollback()
				_ = dns.FlushResolverCache()
			}
			if store != nil {
				_ = store.FinalizeSession(sessionID, prof.Name, elapsed/60, false, "aborted", totalBlocked, startTime, time.Now().UTC())
			}
			_, _ = fmt.Fprintln(stdout, "  [OK] Hosts cleanly restored; session recorded as aborted.")
			return 130

		case <-ticker.C:
			tickCount++
			rem := time.Until(deadline)
			elapsedSeconds := totalSeconds - int(rem.Seconds())
			if elapsedSeconds < 0 {
				elapsedSeconds = 0
			}
			if elapsedSeconds > totalSeconds {
				elapsedSeconds = totalSeconds
			}

			terminatedProcs := procSup.ScanAndTerminate()
			antiCheatProcs := antiCheat.Enforce()

			hits := len(terminatedProcs) + len(antiCheatProcs)
			if hits > 0 {
				totalBlocked += hits
				for _, p := range append(terminatedProcs, antiCheatProcs...) {
					_, _ = fmt.Fprintf(stdout, "\n  [BLOCKED] Terminated distraction: %s\n", p)
				}
			}

			// Mid-session hosts tamper watchdog for standard and hardcore modes
			if hostsEngaged && (strictLevel == engine.StrictnessStandard || strictLevel == engine.StrictnessHardcore) && tickCount%5 == 0 {
				if tampered, err := hosts.VerifyOrReapply(allDomains, sessionMeta); err == nil && tampered {
					totalBlocked++
					dns.FlushResolverCacheAsync()
					_, _ = fmt.Fprintln(stdout, "\n  [BLOCKED] Hosts tamper detected; sinkhole re-applied.")
				}
			}

			_, _ = fmt.Fprint(stdout, tui.FormatProgress(elapsedSeconds, totalSeconds, prof.Name, totalBlocked))

			if rem <= 0 {
				_, _ = fmt.Fprintln(stdout, "\n\n  [OK] Sprint complete.")
				if hostsEngaged {
					_ = hosts.Rollback()
					_ = dns.FlushResolverCache()
				}
				if store != nil {
					_ = store.FinalizeSession(sessionID, prof.Name, durationMin, true, "completed", totalBlocked, startTime, time.Now().UTC())
				}
				_, _ = fmt.Fprintln(stdout, "  [OK] All system protections rolled back cleanly.")
				return 0
			}
		}
	}
}

func handleStop(stdout, stderr io.Writer) int {
	_, _ = fmt.Fprintln(stdout, "  [INFO] Rolling back hosts file and flushing DNS...")
	hosts := engine.NewHostsManager()
	dns := &engine.DnsEngine{}

	var journalSessionID string
	if j, err := hosts.LoadJournal(); err == nil && j != nil {
		journalSessionID = j.SessionID
	}

	if err := hosts.Rollback(); err != nil {
		_, _ = fmt.Fprintf(stderr, "  [ERR] Rollback failed: %v (re-run with --elevate if permission denied)\n", err)
		return 1
	}

	if journalSessionID != "" {
		if store, err := storage.NewStore(); err == nil {
			_, _ = store.MarkOrphanedSession(journalSessionID, time.Now().UTC())
		}
	}

	_ = dns.FlushResolverCache()
	_, _ = fmt.Fprintln(stdout, "  [OK] Clean rollback complete. Unrestricted access restored.")
	return 0
}

type statusJSONOutput struct {
	Version    string               `json:"version"`
	Active     bool                 `json:"active"`
	Elevated   bool                 `json:"elevated"`
	ShieldMode string               `json:"shield_mode"`
	HostsPath  string               `json:"hosts_path"`
	StateDir   string               `json:"state_dir"`
	Journal    *engine.HostsJournal `json:"journal,omitempty"`
}

func handleStatus(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "Output status as JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	hosts := engine.NewHostsManager()
	active := hosts.IsActive()
	elevated := engine.IsElevated()
	journal, _ := hosts.LoadJournal()

	if *asJSON {
		out := statusJSONOutput{
			Version:    Version,
			Active:     active,
			Elevated:   elevated,
			ShieldMode: shieldModeString(elevated),
			HostsPath:  hosts.HostsPath(),
			StateDir:   storage.GetStateDir(),
			Journal:    journal,
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}

	if active {
		_, _ = fmt.Fprintf(stdout, "  [ACTIVE] FocusFlow sinkhole is ENGAGED | %s\n", shieldModeString(elevated))
		if journal != nil {
			_, _ = fmt.Fprintf(stdout, "    Profile: %s | Session: %s | Owner PID: %d | Ends: %s\n",
				journal.Profile, journal.SessionID, journal.PID, journal.ExpectedEndAt)
		}
	} else {
		_, _ = fmt.Fprintf(stdout, "  [INACTIVE] FocusFlow guard is STANDBY | %s\n", shieldModeString(elevated))
	}
	return 0
}

func handleFlush(stdout, stderr io.Writer) int {
	_, _ = fmt.Fprint(stdout, "  [INFO] Purging DNS resolver cache... ")
	dns := &engine.DnsEngine{}
	if err := dns.FlushResolverCache(); err != nil {
		_, _ = fmt.Fprintf(stderr, "[ERR] FAILED (%v)\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "[OK]")
	return 0
}

func handleStats(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	fs.SetOutput(stderr)
	days := fs.Int("days", 7, "Number of days for historical bar chart")
	asJSON := fs.Bool("json", false, "Output statistics as JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	store, err := storage.NewStore()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "[ERR] Could not access metrics store: %v\n", err)
		return 1
	}

	summary, err := store.ComputeAnalytics(*days, time.Now())
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "[ERR] Could not compute stats: %v\n", err)
		return 1
	}

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(summary)
		return 0
	}

	_, _ = fmt.Fprintln(stdout, "  [STATS] FocusFlow Historical Telemetry")
	_, _ = fmt.Fprintf(stdout, "    - Completed Sprints:   %d / %d (%.1f%% completion)\n",
		summary.CompletedSprints, summary.AttemptedSprints, summary.CompletionRate)
	_, _ = fmt.Fprintf(stdout, "    - Deep Work Total:     %.1f hrs (%d min)\n",
		summary.TotalHours, summary.TotalMinutes)
	_, _ = fmt.Fprintf(stdout, "    - Active Streak:       %d days\n", summary.ActiveStreakDays)
	_, _ = fmt.Fprintf(stdout, "    - Distractions Killed: %d\n\n", summary.TotalBlocked)
	_, _ = fmt.Fprintf(stdout, "  Last %d Days Focus Chart:\n", len(summary.Daily))
	_, _ = fmt.Fprint(stdout, tui.FormatDailyChart(summary.Daily))
	return 0
}

func handleExport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "json", "Export format: json or csv")
	outPath := fs.String("out", "", "Output file path (defaults to stdout)")
	days := fs.Int("days", 7, "Summary window in days for JSON export")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	store, err := storage.NewStore()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "[ERR] Could not open session store: %v\n", err)
		return 1
	}

	var dest io.Writer = stdout
	var outFile *os.File
	if strings.TrimSpace(*outPath) != "" {
		cleanOut := filepath.Clean(*outPath)
		if err := os.MkdirAll(filepath.Dir(cleanOut), 0755); err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Cannot create export directory: %v\n", err)
			return 1
		}
		outFile, err = os.Create(cleanOut)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Cannot create export file: %v\n", err)
			return 1
		}
		defer outFile.Close()
		dest = outFile
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		if err := store.ExportJSON(dest, *days, time.Now()); err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] JSON export failed: %v\n", err)
			return 1
		}
	case "csv":
		if err := store.ExportCSV(dest); err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] CSV export failed: %v\n", err)
			return 1
		}
	default:
		_, _ = fmt.Fprintf(stderr, "[ERR] Unsupported format %q (expected 'json' or 'csv')\n", *format)
		return 1
	}

	if outFile != nil {
		_, _ = fmt.Fprintf(stdout, "[OK] Exported %s report to %s\n", strings.ToUpper(*format), outFile.Name())
	}
	return 0
}

type doctorReport struct {
	Version        string                 `json:"version"`
	OS             string                 `json:"os"`
	Arch           string                 `json:"arch"`
	Elevated       bool                   `json:"elevated"`
	ShieldMode     string                 `json:"shield_mode"`
	StateDir       string                 `json:"state_dir"`
	ConfigPath     string                 `json:"config_path"`
	SessionsPath   string                 `json:"sessions_path"`
	HostsPath      string                 `json:"hosts_path"`
	HostsReadable  bool                   `json:"hosts_readable"`
	HostsWritable  bool                   `json:"hosts_writable"`
	HostsActive    bool                   `json:"hosts_active"`
	JournalPresent bool                   `json:"journal_present"`
	Recovery       *engine.RecoveryResult `json:"recovery,omitempty"`
	Healthy        bool                   `json:"healthy"`
}

func checkHostsWritable(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func handleDoctor(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "Output diagnostic report as JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	hosts := engine.NewHostsManager()
	recRes, recErr := runStartupAutoRecovery()

	_, readErr := os.ReadFile(hosts.HostsPath())
	readable := readErr == nil
	writable := checkHostsWritable(hosts.HostsPath())
	elevated := engine.IsElevated()
	_, jErr := hosts.LoadJournal()

	storePath := filepath.Join(storage.GetStateDir(), "sessions.json")
	report := doctorReport{
		Version:        Version,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		Elevated:       elevated,
		ShieldMode:     shieldModeString(elevated),
		StateDir:       storage.GetStateDir(),
		ConfigPath:     profile.GetConfigPath(),
		SessionsPath:   storePath,
		HostsPath:      hosts.HostsPath(),
		HostsReadable:  readable,
		HostsWritable:  writable,
		HostsActive:    hosts.IsActive(),
		JournalPresent: jErr == nil,
		Recovery:       recRes,
		Healthy:        readable && recErr == nil,
	}

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
		if !report.Healthy {
			return 1
		}
		return 0
	}

	_, _ = fmt.Fprintln(stdout, "=== FocusFlow System Doctor ===")
	_, _ = fmt.Fprintf(stdout, "  Version:        v%s (%s/%s)\n", report.Version, report.OS, report.Arch)
	_, _ = fmt.Fprintf(stdout, "  Shield Mode:    %s\n", report.ShieldMode)
	_, _ = fmt.Fprintf(stdout, "  State Dir:      %s\n", report.StateDir)
	_, _ = fmt.Fprintf(stdout, "  Hosts Path:     %s (readable=%v, writable=%v)\n",
		report.HostsPath, report.HostsReadable, report.HostsWritable)
	_, _ = fmt.Fprintf(stdout, "  Hosts Engaged:  %v\n", report.HostsActive)
	if recRes != nil && recRes.Recovered {
		_, _ = fmt.Fprintf(stdout, "  Orphan Recovery:[OK] Restored clean hosts (reason=%s, fallback=%v)\n",
			recRes.Reason, recRes.UsedFallback)
	} else if recErr != nil {
		_, _ = fmt.Fprintf(stderr, "  Orphan Recovery:[ERR] %v (re-run 'focusflow recover --elevate')\n", recErr)
		return 1
	} else {
		_, _ = fmt.Fprintf(stdout, "  Journal State:  [OK] (%s)\n", recRes.Reason)
	}
	return 0
}

func handleRecover(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("recover", flag.ContinueOnError)
	fs.SetOutput(stderr)
	force := fs.Bool("force", false, "Force rollback even if owning PID appears alive")
	asJSON := fs.Bool("json", false, "Output recovery result as JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	hosts := engine.NewHostsManager()
	if *force {
		var orphanID string
		if j, err := hosts.LoadJournal(); err == nil && j != nil {
			orphanID = j.SessionID
		}
		if err := hosts.Rollback(); err != nil {
			if *asJSON {
				_ = json.NewEncoder(stdout).Encode(map[string]any{"recovered": false, "error": err.Error()})
			} else {
				_, _ = fmt.Fprintf(stderr, "[ERR] Force recovery failed: %v\n", err)
			}
			return 1
		}
		if store, err := storage.NewStore(); err == nil {
			_, _ = store.MarkOrphanedSession(orphanID, time.Now().UTC())
		}
		dns := &engine.DnsEngine{}
		dns.FlushResolverCacheAsync()
		res := &engine.RecoveryResult{
			Recovered:       true,
			OrphanSessionID: orphanID,
			Reason:          "forced_rollback",
		}
		if *asJSON {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(res)
		} else {
			_, _ = fmt.Fprintln(stdout, "[OK] Forced hosts recovery completed.")
		}
		return 0
	}

	res, err := runStartupAutoRecovery()
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		if err != nil {
			return 1
		}
		return 0
	}

	if err != nil {
		_, _ = fmt.Fprintf(stderr, "[ERR] Orphan recovery failed: %v (try 'focusflow recover --elevate')\n", err)
		return 1
	}
	if res.Recovered {
		_, _ = fmt.Fprintf(stdout, "[OK] Recovered orphaned hosts state (reason=%s, fallback=%v)\n",
			res.Reason, res.UsedFallback)
	} else {
		_, _ = fmt.Fprintf(stdout, "[OK] No orphaned hosts state detected (%s)\n", res.Reason)
	}
	return 0
}
