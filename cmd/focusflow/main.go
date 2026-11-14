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
var Version = "2.2.0"

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
	case "profile", "profiles":
		return handleProfile(subArgs, stdout, stderr)
	case "goal":
		return handleGoal(subArgs, stdout, stderr)
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
	_, _ = fmt.Fprintln(w, "  start                           Start a single or multi-cycle Pomodoro sprint")
	_, _ = fmt.Fprintln(w, "  stop                            Rollback hosts file and restore unrestricted access")
	_, _ = fmt.Fprintln(w, "  status [--json]                 Inspect shield state, daily goal, and active journal")
	_, _ = fmt.Fprintln(w, "  profile <list|show|create|delete> Manage built-in and custom focus profiles")
	_, _ = fmt.Fprintln(w, "  goal [--minutes <n>] [--json]   View or set daily focus goal (5..1440 min)")
	_, _ = fmt.Fprintln(w, "  list [--json]                   List all active profiles, custom blocked sites & apps")
	_, _ = fmt.Fprintln(w, "  add site <domain>               Add validated RFC-1123 custom domain to blocklist")
	_, _ = fmt.Fprintln(w, "  add app <proc.exe>              Add custom process to blocklist (denylist protected)")
	_, _ = fmt.Fprintln(w, "  remove site <domain>            Remove custom domain from blocklist")
	_, _ = fmt.Fprintln(w, "  remove app <proc.exe>           Remove custom process from blocklist")
	_, _ = fmt.Fprintln(w, "  stats [--days 7] [--json]       Display focus metrics, daily goal, streak & chart")
	_, _ = fmt.Fprintln(w, "  export [--format json|csv]      Export session telemetry (--out <file>)")
	_, _ = fmt.Fprintln(w, "  doctor [--json]                 Audit privileges, hosts integrity, and journal health")
	_, _ = fmt.Fprintln(w, "  recover [--force] [--json]      Recover orphaned crash/reboot hosts state")
	_, _ = fmt.Fprintln(w, "  flush                           Purge local DNS resolver cache immediately")
	_, _ = fmt.Fprintln(w, "  version                         Print application version")
	_, _ = fmt.Fprintln(w, "\nGlobal / Start Options:")
	_, _ = fmt.Fprintln(w, "  --elevate                       Request Administrator/sudo privilege elevation")
	_, _ = fmt.Fprintln(w, "  --profile <name>                Profile: study, dev, hardcore, or custom (default: study)")
	_, _ = fmt.Fprintln(w, "  --minutes <n>                   Work duration in minutes (default: profile default)")
	_, _ = fmt.Fprintln(w, "  --cycles <n>                    Number of Pomodoro sprints (1..24, default: 1)")
	_, _ = fmt.Fprintln(w, "  --break <min>                   Break minutes between cycles (0..120, default: profile default)")
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
	ConfigPath       string                           `json:"config_path"`
	StateDir         string                           `json:"state_dir"`
	DailyGoalMinutes int                              `json:"daily_goal_minutes"`
	CustomDomains    []string                         `json:"custom_domains"`
	CustomProcesses  []string                         `json:"custom_processes"`
	Profiles         map[string]*profile.FocusProfile `json:"profiles"`
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
			ConfigPath:       profile.GetConfigPath(),
			StateDir:         profile.GetConfigDir(),
			DailyGoalMinutes: cfg.DailyGoalMinutes,
			CustomDomains:    cfg.CustomDomains,
			CustomProcesses:  cfg.CustomProcesses,
			Profiles:         cfg.Profiles,
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}

	_, _ = fmt.Fprintln(stdout, "\n=== FocusFlow Configuration & Rules ===")
	_, _ = fmt.Fprintf(stdout, "State Dir:        %s\n", profile.GetConfigDir())
	_, _ = fmt.Fprintf(stdout, "Config File:      %s\n", profile.GetConfigPath())
	_, _ = fmt.Fprintf(stdout, "Daily Focus Goal: %d min\n\n", cfg.DailyGoalMinutes)

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

type profileJSONItem struct {
	Name                string   `json:"name"`
	BuiltIn             bool     `json:"builtin"`
	Description         string   `json:"description"`
	DefaultWorkMinutes  int      `json:"default_work_minutes"`
	DefaultBreakMinutes int      `json:"default_break_minutes"`
	DomainCount         int      `json:"domain_count"`
	AppCount            int      `json:"app_count"`
	BlockedDomains      []string `json:"blocked_domains"`
	BlockedProcesses    []string `json:"blocked_processes"`
}

func toProfileJSONItem(p *profile.FocusProfile) profileJSONItem {
	domains := p.BlockedDomains
	if domains == nil {
		domains = []string{}
	}
	procs := p.BlockedProcesses
	if procs == nil {
		procs = []string{}
	}
	return profileJSONItem{
		Name:                p.Name,
		BuiltIn:             profile.IsBuiltInProfile(p.Name),
		Description:         p.Description,
		DefaultWorkMinutes:  p.DefaultWorkMinutes,
		DefaultBreakMinutes: p.DefaultBreakMinutes,
		DomainCount:         len(domains),
		AppCount:            len(procs),
		BlockedDomains:      domains,
		BlockedProcesses:    procs,
	}
}

func splitPositionalAndFlags(args []string, boolFlags map[string]bool) ([]string, []string) {
	var posArgs []string
	var flagArgs []string
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if tok == "--" {
			posArgs = append(posArgs, args[i+1:]...)
			break
		}
		if strings.HasPrefix(tok, "-") && tok != "-" {
			flagArgs = append(flagArgs, tok)
			if strings.Contains(tok, "=") {
				continue
			}
			flagName := strings.TrimLeft(tok, "-")
			if boolFlags[flagName] {
				continue
			}
			if i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		posArgs = append(posArgs, tok)
	}
	return posArgs, flagArgs
}

func splitCSVArg(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func handleProfile(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "Usage: focusflow profile <list|show|create|delete> [options]")
		return 1
	}

	sub := strings.ToLower(strings.TrimSpace(args[0]))
	rest := args[1:]

	switch sub {
	case "list", "ls":
		fs := flag.NewFlagSet("profile list", flag.ContinueOnError)
		fs.SetOutput(stderr)
		asJSON := fs.Bool("json", false, "Output profiles as JSON")
		if err := fs.Parse(rest); err != nil {
			return 1
		}
		cfg, err := profile.LoadConfig()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Error reading config: %v\n", err)
			return 1
		}
		names := cfg.SortedProfileNames()
		items := make([]profileJSONItem, 0, len(names))
		for _, name := range names {
			if p, ok := cfg.Profiles[name]; ok && p != nil {
				items = append(items, toProfileJSONItem(p))
			}
		}
		if *asJSON {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(map[string]any{"profiles": items})
			return 0
		}
		_, _ = fmt.Fprintln(stdout, "  [PROFILES] FocusFlow Profiles")
		for _, item := range items {
			kind := "custom"
			if item.BuiltIn {
				kind = "built-in"
			}
			_, _ = fmt.Fprintf(stdout, "    - %-12s [%-8s] %3dm work / %2dm break | %2d sites | %2d apps | %s\n",
				item.Name, kind, item.DefaultWorkMinutes, item.DefaultBreakMinutes,
				item.DomainCount, item.AppCount, item.Description)
		}
		return 0

	case "show", "info", "get":
		posArgs, flagArgs := splitPositionalAndFlags(rest, map[string]bool{"json": true, "h": true, "help": true})
		fs := flag.NewFlagSet("profile show", flag.ContinueOnError)
		fs.SetOutput(stderr)
		asJSON := fs.Bool("json", false, "Output profile details as JSON")
		if err := fs.Parse(flagArgs); err != nil {
			return 1
		}
		if len(posArgs) < 1 {
			_, _ = fmt.Fprintln(stderr, "Usage: focusflow profile show <name> [--json]")
			return 1
		}
		cfg, err := profile.LoadConfig()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Error reading config: %v\n", err)
			return 1
		}
		p, ok := cfg.GetProfile(posArgs[0])
		if !ok {
			_, _ = fmt.Fprintf(stderr, "[ERR] Profile '%s' not found.\n", posArgs[0])
			return 1
		}
		item := toProfileJSONItem(p)
		if *asJSON {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(item)
			return 0
		}
		kind := "custom"
		if item.BuiltIn {
			kind = "built-in"
		}
		_, _ = fmt.Fprintf(stdout, "  [PROFILE] %s (%s)\n", item.Name, kind)
		_, _ = fmt.Fprintf(stdout, "    Description:   %s\n", item.Description)
		_, _ = fmt.Fprintf(stdout, "    Work / Break:  %d min / %d min\n", item.DefaultWorkMinutes, item.DefaultBreakMinutes)
		_, _ = fmt.Fprintf(stdout, "    Blocked Sites (%d):\n", item.DomainCount)
		if len(item.BlockedDomains) == 0 {
			_, _ = fmt.Fprintln(stdout, "      (None)")
		} else {
			for _, d := range item.BlockedDomains {
				_, _ = fmt.Fprintf(stdout, "      - %s\n", d)
			}
		}
		_, _ = fmt.Fprintf(stdout, "    Blocked Apps (%d):\n", item.AppCount)
		if len(item.BlockedProcesses) == 0 {
			_, _ = fmt.Fprintln(stdout, "      (None)")
		} else {
			for _, proc := range item.BlockedProcesses {
				_, _ = fmt.Fprintf(stdout, "      - %s\n", proc)
			}
		}
		return 0

	case "create", "set", "save", "upsert":
		posArgs, flagArgs := splitPositionalAndFlags(rest, map[string]bool{"json": true, "h": true, "help": true})
		fs := flag.NewFlagSet("profile create", flag.ContinueOnError)
		fs.SetOutput(stderr)
		workMin := fs.Int("minutes", 25, "Default work duration in minutes (1..720)")
		breakMin := fs.Int("break", 5, "Default break duration in minutes (0..120)")
		sitesCSV := fs.String("sites", "", "Comma-separated domains to block")
		appsCSV := fs.String("apps", "", "Comma-separated process names to block")
		desc := fs.String("desc", "", "Profile description")
		fromProf := fs.String("from", "", "Clone initial settings & rules from existing profile")
		asJSON := fs.Bool("json", false, "Output created/updated profile as JSON")
		if err := fs.Parse(flagArgs); err != nil {
			return 1
		}
		if len(posArgs) < 1 {
			_, _ = fmt.Fprintln(stderr, "Usage: focusflow profile create <name> [--minutes 45] [--break 10] [--sites a.com,b.com] [--apps slack.exe] [--desc \"...\"] [--from study] [--json]")
			return 1
		}

		visited := make(map[string]bool)
		fs.Visit(func(f *flag.Flag) {
			visited[f.Name] = true
		})

		cfg, err := profile.LoadConfig()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Error loading config: %v\n", err)
			return 1
		}

		input := profile.CustomProfileInput{
			Name:        posArgs[0],
			FromProfile: *fromProf,
		}
		if visited["minutes"] {
			input.WorkMinutes = workMin
		}
		if visited["break"] {
			input.BreakMinutes = breakMin
		}
		if visited["desc"] {
			input.Description = desc
		}
		if visited["sites"] {
			parsedSites := splitCSVArg(*sitesCSV)
			input.Domains = &parsedSites
		}
		if visited["apps"] {
			parsedApps := splitCSVArg(*appsCSV)
			input.Processes = &parsedApps
		}

		prof, created, err := cfg.UpsertCustomProfile(input)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Cannot save profile: %v\n", err)
			return 1
		}
		if err := profile.SaveConfig(cfg); err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Error saving config: %v\n", err)
			return 1
		}

		item := toProfileJSONItem(prof)
		if *asJSON {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(map[string]any{
				"created": created,
				"profile": item,
			})
			return 0
		}
		verb := "Updated"
		if created {
			verb = "Created"
		}
		_, _ = fmt.Fprintf(stdout, "[OK] %s custom profile '%s' (%dm work / %dm break | %d sites | %d apps).\n",
			verb, item.Name, item.DefaultWorkMinutes, item.DefaultBreakMinutes, item.DomainCount, item.AppCount)
		return 0

	case "delete", "rm", "remove":
		posArgs, flagArgs := splitPositionalAndFlags(rest, map[string]bool{"json": true, "h": true, "help": true})
		fs := flag.NewFlagSet("profile delete", flag.ContinueOnError)
		fs.SetOutput(stderr)
		asJSON := fs.Bool("json", false, "Output deletion result as JSON")
		if err := fs.Parse(flagArgs); err != nil {
			return 1
		}
		if len(posArgs) < 1 {
			_, _ = fmt.Fprintln(stderr, "Usage: focusflow profile delete <name> [--json]")
			return 1
		}
		cfg, err := profile.LoadConfig()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Error loading config: %v\n", err)
			return 1
		}
		targetName := strings.ToLower(strings.TrimSpace(posArgs[0]))
		if err := cfg.DeleteCustomProfile(targetName); err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Cannot delete profile '%s': %v\n", posArgs[0], err)
			return 1
		}
		if err := profile.SaveConfig(cfg); err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Error saving config: %v\n", err)
			return 1
		}
		if *asJSON {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(map[string]any{
				"deleted": true,
				"name":    targetName,
			})
			return 0
		}
		_, _ = fmt.Fprintf(stdout, "[OK] Deleted custom profile '%s'.\n", targetName)
		return 0

	default:
		_, _ = fmt.Fprintf(stderr, "[ERR] Unknown profile subcommand %q. Expected list, show, create, or delete.\n", sub)
		return 1
	}
}

type goalJSONOutput struct {
	Updated              bool    `json:"updated"`
	TodayFocusMinutes    int     `json:"today_focus_minutes"`
	DailyGoalMinutes     int     `json:"daily_goal_minutes"`
	DailyGoalProgressPct float64 `json:"daily_goal_progress_pct"`
}

func handleGoal(args []string, stdout, stderr io.Writer) int {
	_, flagArgs := splitPositionalAndFlags(args, map[string]bool{"json": true, "h": true, "help": true})
	fs := flag.NewFlagSet("goal", flag.ContinueOnError)
	fs.SetOutput(stderr)
	minutes := fs.Int("minutes", 0, "Set daily focus goal in minutes (5..1440)")
	asJSON := fs.Bool("json", false, "Output daily goal status as JSON")
	if err := fs.Parse(flagArgs); err != nil {
		return 1
	}

	minutesSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "minutes" {
			minutesSet = true
		}
	})

	cfg, err := profile.LoadConfig()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "[ERR] Error loading config: %v\n", err)
		return 1
	}

	if minutesSet {
		if *minutes < profile.MinDailyGoalMinutes || *minutes > profile.MaxDailyGoalMinutes {
			_, _ = fmt.Fprintf(stderr, "[ERR] --minutes must be between %d and %d.\n",
				profile.MinDailyGoalMinutes, profile.MaxDailyGoalMinutes)
			return 1
		}
		cfg.DailyGoalMinutes = *minutes
		if err := profile.SaveConfig(cfg); err != nil {
			_, _ = fmt.Fprintf(stderr, "[ERR] Error saving config: %v\n", err)
			return 1
		}
	}

	var prog storage.DailyGoalProgress
	if store, sErr := storage.NewStore(); sErr == nil {
		prog, _ = store.ComputeDailyGoalProgress(cfg.DailyGoalMinutes, time.Now())
	} else {
		prog = storage.DailyGoalProgress{
			TodayFocusMinutes:    0,
			DailyGoalMinutes:     cfg.DailyGoalMinutes,
			DailyGoalProgressPct: 0,
		}
	}

	if *asJSON {
		out := goalJSONOutput{
			Updated:              minutesSet,
			TodayFocusMinutes:    prog.TodayFocusMinutes,
			DailyGoalMinutes:     prog.DailyGoalMinutes,
			DailyGoalProgressPct: prog.DailyGoalProgressPct,
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}

	if minutesSet {
		_, _ = fmt.Fprintf(stdout, "[OK] Daily focus goal set to %d minutes.\n", cfg.DailyGoalMinutes)
	}
	_, _ = fmt.Fprintf(stdout, "  [GOAL] %s\n",
		tui.FormatDailyGoalLine(prog.TodayFocusMinutes, prog.DailyGoalMinutes, prog.DailyGoalProgressPct))
	return 0
}

func handleStart(args []string, stdout, stderr io.Writer) int {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)
	return runStartWithSignals(args, stdout, stderr, sigChan)
}

func runStartWithSignals(args []string, stdout, stderr io.Writer, sigChan <-chan os.Signal) int {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profileName := fs.String("profile", "study", "Profile name")
	minutes := fs.Int("minutes", 0, "Work duration in minutes")
	secondsOverride := fs.Int("seconds", 0, "Work duration override in seconds (for rapid testing)")
	cycles := fs.Int("cycles", 1, "Number of Pomodoro cycles (1..24)")
	breakFlag := fs.Int("break", -1, "Break duration in minutes between cycles (0..120)")
	breakSecondsOverride := fs.Int("break-seconds", 0, "Break duration override in seconds (for rapid testing)")
	strictness := fs.String("strict", "", "Strictness level: relaxed, standard, hardcore")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *cycles < 1 || *cycles > 24 {
		_, _ = fmt.Fprintln(stderr, "[ERR] --cycles must be between 1 and 24.")
		return 1
	}
	if *minutes < 0 {
		_, _ = fmt.Fprintln(stderr, "[ERR] --minutes cannot be negative.")
		return 1
	}

	prof := profile.LoadOrDefault(*profileName)
	durationMin := prof.DefaultWorkMinutes
	if *minutes > 0 {
		durationMin = *minutes
	}
	workSeconds := durationMin * 60
	if *secondsOverride > 0 {
		workSeconds = *secondsOverride
		if durationMin <= 0 {
			durationMin = 1
		}
	}

	breakMin := prof.DefaultBreakMinutes
	breakVisited := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "break" {
			breakVisited = true
		}
	})
	if breakVisited {
		if *breakFlag < 0 || *breakFlag > 120 {
			_, _ = fmt.Fprintln(stderr, "[ERR] --break must be between 0 and 120 minutes.")
			return 1
		}
		breakMin = *breakFlag
	}
	breakSeconds := breakMin * 60
	if *breakSecondsOverride > 0 {
		breakSeconds = *breakSecondsOverride
		if breakMin <= 0 {
			breakMin = 1
		}
	}

	strictLevel := engine.StrictnessStandard
	if prof.Name == "hardcore" && *strictness == "" {
		strictLevel = engine.StrictnessHardcore
	}
	if strings.TrimSpace(*strictness) != "" {
		switch strings.ToLower(strings.TrimSpace(*strictness)) {
		case "hardcore":
			strictLevel = engine.StrictnessHardcore
		case "relaxed":
			strictLevel = engine.StrictnessRelaxed
		case "standard":
			strictLevel = engine.StrictnessStandard
		default:
			_, _ = fmt.Fprintf(stderr, "[ERR] Invalid --strict level %q (expected relaxed, standard, or hardcore).\n", *strictness)
			return 1
		}
	}

	tui.PrintBannerTo(stdout, Version)

	totalCycles := *cycles
	store, _ := storage.NewStore()
	hosts := engine.NewHostsManager()
	dns := &engine.DnsEngine{}
	procSup := engine.NewProcessSupervisor(prof.BlockedProcesses)
	antiCheat := engine.NewAntiCheatGuard(strictLevel)

	allDomains := append([]string{}, prof.BlockedDomains...)
	allDomains = append(allDomains, dns.GetDoHBypassDomains()...)

	hostsEngaged := false

	for cycle := 1; cycle <= totalCycles; cycle++ {
		startTime := time.Now().UTC()
		expectedEnd := startTime.Add(time.Duration(workSeconds) * time.Second)
		sessionID := storage.GenerateSessionID()

		if store != nil {
			_ = store.StartSession(sessionID, prof.Name, durationMin, startTime)
		}

		if totalCycles > 1 {
			_, _ = fmt.Fprintf(stdout, "  [CYCLE %d/%d] [ACTIVE] Starting '%s' sprint (%d min work / %d min break | Strictness: %s | Session: %s)\n",
				cycle, totalCycles, prof.Name, durationMin, breakMin, strictLevel, sessionID)
		} else {
			_, _ = fmt.Fprintf(stdout, "  [ACTIVE] Starting '%s' sprint (%d min | Strictness: %s | Session: %s)\n",
				prof.Name, durationMin, strictLevel, sessionID)
		}

		sessionMeta := engine.SessionMeta{
			SessionID:     sessionID,
			PID:           os.Getpid(),
			StartedAt:     startTime,
			ExpectedEndAt: expectedEnd,
			Profile:       prof.Name,
		}

		if !hostsEngaged {
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
		} else {
			// Continuous hardcore lockdown across cycles: refresh active session journal metadata
			if err := hosts.ApplyBlocklistWithSession(allDomains, sessionMeta); err != nil {
				hostsEngaged = false
			}
		}

		cycleBlocked := 0
		workTicker := time.NewTicker(1 * time.Second)
		deadline := time.Now().Add(time.Duration(workSeconds) * time.Second)
		tickCount := 0

		workFinished := false
		for !workFinished {
			select {
			case <-sigChan:
				workTicker.Stop()
				elapsed := int(time.Since(startTime).Seconds())
				if elapsed < 0 {
					elapsed = 0
				}
				_, _ = fmt.Fprintln(stdout, "\n\n  [WARN] Interrupt signal received. Rolling back system state...")
				if hostsEngaged {
					_ = hosts.Rollback()
					_ = dns.FlushResolverCache()
					hostsEngaged = false
				}
				if store != nil {
					_ = store.FinalizeSession(sessionID, prof.Name, elapsed/60, false, "aborted", cycleBlocked, startTime, time.Now().UTC())
				}
				_, _ = fmt.Fprintln(stdout, "  [OK] Hosts cleanly restored; session recorded as aborted.")
				return 130

			case <-workTicker.C:
				tickCount++
				rem := time.Until(deadline)
				elapsedSeconds := workSeconds - int(rem.Seconds())
				if elapsedSeconds < 0 {
					elapsedSeconds = 0
				}
				if elapsedSeconds > workSeconds {
					elapsedSeconds = workSeconds
				}

				terminatedProcs := procSup.ScanAndTerminate()
				antiCheatProcs := antiCheat.Enforce()

				hits := len(terminatedProcs) + len(antiCheatProcs)
				if hits > 0 {
					cycleBlocked += hits
					for _, p := range append(terminatedProcs, antiCheatProcs...) {
						_, _ = fmt.Fprintf(stdout, "\n  [BLOCKED] Terminated distraction: %s\n", p)
					}
				}

				if hostsEngaged && (strictLevel == engine.StrictnessStandard || strictLevel == engine.StrictnessHardcore) && tickCount%5 == 0 {
					if tampered, err := hosts.VerifyOrReapply(allDomains, sessionMeta); err == nil && tampered {
						cycleBlocked++
						dns.FlushResolverCacheAsync()
						_, _ = fmt.Fprintln(stdout, "\n  [BLOCKED] Hosts tamper detected; sinkhole re-applied.")
					}
				}

				_, _ = fmt.Fprint(stdout, tui.FormatCycleProgress(elapsedSeconds, workSeconds, prof.Name, cycleBlocked, cycle, totalCycles))

				if rem <= 0 {
					workTicker.Stop()
					if store != nil {
						_ = store.FinalizeSession(sessionID, prof.Name, durationMin, true, "completed", cycleBlocked, startTime, time.Now().UTC())
					}
					if totalCycles > 1 {
						_, _ = fmt.Fprintf(stdout, "\n\n  [CYCLE %d/%d] [OK] Work sprint complete and recorded.\n", cycle, totalCycles)
					} else {
						_, _ = fmt.Fprintln(stdout, "\n\n  [OK] Sprint complete.")
					}
					workFinished = true
				}
			}
		}

		// Inter-cycle Pomodoro recovery break
		if cycle < totalCycles && breakSeconds > 0 {
			continuousLockdown := strictLevel == engine.StrictnessHardcore
			if !continuousLockdown && hostsEngaged {
				_ = hosts.Rollback()
				_ = dns.FlushResolverCache()
				hostsEngaged = false
				_, _ = fmt.Fprintf(stdout, "  [CYCLE %d/%d] [BREAK] Starting %d min recovery break (hosts shield temporarily lifted)...\n",
					cycle, totalCycles, breakMin)
			} else if continuousLockdown {
				_, _ = fmt.Fprintf(stdout, "  [CYCLE %d/%d] [BREAK] Starting %d min recovery break (hardcore lockdown continuous)...\n",
					cycle, totalCycles, breakMin)
			} else {
				_, _ = fmt.Fprintf(stdout, "  [CYCLE %d/%d] [BREAK] Starting %d min recovery break...\n",
					cycle, totalCycles, breakMin)
			}

			breakTicker := time.NewTicker(1 * time.Second)
			breakDeadline := time.Now().Add(time.Duration(breakSeconds) * time.Second)
			breakTicks := 0
			breakDone := false

			for !breakDone {
				select {
				case <-sigChan:
					breakTicker.Stop()
					_, _ = fmt.Fprintln(stdout, "\n\n  [WARN] Interrupt signal received during break. Rolling back system state...")
					if hostsEngaged {
						_ = hosts.Rollback()
						_ = dns.FlushResolverCache()
						hostsEngaged = false
					}
					_, _ = fmt.Fprintln(stdout, "  [OK] Break interrupted; completed sprints preserved and hosts restored.")
					return 130

				case <-breakTicker.C:
					breakTicks++
					rem := time.Until(breakDeadline)
					elapsedBreak := breakSeconds - int(rem.Seconds())
					if elapsedBreak < 0 {
						elapsedBreak = 0
					}
					if elapsedBreak > breakSeconds {
						elapsedBreak = breakSeconds
					}

					if continuousLockdown {
						_ = procSup.ScanAndTerminate()
						_ = antiCheat.Enforce()
						if hostsEngaged && breakTicks%5 == 0 {
							if tampered, err := hosts.VerifyOrReapply(allDomains, sessionMeta); err == nil && tampered {
								dns.FlushResolverCacheAsync()
							}
						}
					}

					_, _ = fmt.Fprint(stdout, tui.FormatBreakProgress(elapsedBreak, breakSeconds, cycle, totalCycles, continuousLockdown))

					if rem <= 0 {
						breakTicker.Stop()
						_, _ = fmt.Fprintf(stdout, "\n\n  [CYCLE %d/%d] [OK] Recovery break complete.\n\n", cycle, totalCycles)
						breakDone = true
					}
				}
			}
		}
	}

	if hostsEngaged {
		_ = hosts.Rollback()
		_ = dns.FlushResolverCache()
	}
	_, _ = fmt.Fprintln(stdout, "  [OK] All system protections rolled back cleanly.")
	return 0
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
	Version              string               `json:"version"`
	Active               bool                 `json:"active"`
	Elevated             bool                 `json:"elevated"`
	ShieldMode           string               `json:"shield_mode"`
	HostsPath            string               `json:"hosts_path"`
	StateDir             string               `json:"state_dir"`
	TodayFocusMinutes    int                  `json:"today_focus_minutes"`
	DailyGoalMinutes     int                  `json:"daily_goal_minutes"`
	DailyGoalProgressPct float64              `json:"daily_goal_progress_pct"`
	Journal              *engine.HostsJournal `json:"journal,omitempty"`
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

	goalMinutes := profile.DefaultDailyGoalMinutes
	if cfg, err := profile.LoadConfig(); err == nil {
		goalMinutes = cfg.DailyGoalMinutes
	}
	goalProg := storage.DailyGoalProgress{
		TodayFocusMinutes:    0,
		DailyGoalMinutes:     goalMinutes,
		DailyGoalProgressPct: 0,
	}
	if store, err := storage.NewStore(); err == nil {
		if computed, cErr := store.ComputeDailyGoalProgress(goalMinutes, time.Now()); cErr == nil {
			goalProg = computed
		}
	}

	if *asJSON {
		out := statusJSONOutput{
			Version:              Version,
			Active:               active,
			Elevated:             elevated,
			ShieldMode:           shieldModeString(elevated),
			HostsPath:            hosts.HostsPath(),
			StateDir:             storage.GetStateDir(),
			TodayFocusMinutes:    goalProg.TodayFocusMinutes,
			DailyGoalMinutes:     goalProg.DailyGoalMinutes,
			DailyGoalProgressPct: goalProg.DailyGoalProgressPct,
			Journal:              journal,
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
	_, _ = fmt.Fprintf(stdout, "    Daily Goal: %s\n",
		tui.FormatDailyGoalLine(goalProg.TodayFocusMinutes, goalProg.DailyGoalMinutes, goalProg.DailyGoalProgressPct))
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

	goalMinutes := profile.DefaultDailyGoalMinutes
	if cfg, cfgErr := profile.LoadConfig(); cfgErr == nil {
		goalMinutes = cfg.DailyGoalMinutes
	}

	summary, err := store.ComputeAnalyticsWithGoal(*days, goalMinutes, time.Now())
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
	_, _ = fmt.Fprintf(stdout, "    - Daily Focus Goal:    %s\n",
		tui.FormatDailyGoalLine(summary.TodayFocusMinutes, summary.DailyGoalMinutes, summary.DailyGoalProgressPct))
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
