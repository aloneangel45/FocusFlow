package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/aloneangel45/focusflow/pkg/engine"
	"github.com/aloneangel45/focusflow/pkg/storage"
)

const (
	DefaultDailyGoalMinutes = storage.DefaultDailyGoalMinutes
	MinDailyGoalMinutes     = storage.MinDailyGoalMinutes
	MaxDailyGoalMinutes     = storage.MaxDailyGoalMinutes
)

var (
	ErrInvalidProfileSlug  = errors.New("invalid profile slug (must match ^[a-z0-9][a-z0-9_-]{0,31}$)")
	ErrBuiltInProfile      = errors.New("built-in profile cannot be overwritten or deleted")
	ErrProfileNotFound     = errors.New("profile not found")
	ErrInvalidWorkMinutes  = errors.New("work minutes must be between 1 and 720")
	ErrInvalidBreakMinutes = errors.New("break minutes must be between 0 and 120")

	profileSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	builtInProfileSet  = map[string]struct{}{
		"study":    {},
		"dev":      {},
		"hardcore": {},
	}
)

type FocusProfile struct {
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	BlockedDomains      []string `json:"blocked_domains"`
	BlockedProcesses    []string `json:"blocked_processes"`
	DefaultWorkMinutes  int      `json:"default_work_minutes"`
	DefaultBreakMinutes int      `json:"default_break_minutes"`
}

type ConfigStore struct {
	CustomDomains    []string                 `json:"custom_domains"`
	CustomProcesses  []string                 `json:"custom_processes"`
	DailyGoalMinutes int                      `json:"daily_goal_minutes"`
	Profiles         map[string]*FocusProfile `json:"profiles"`
}

type rawConfigCompat struct {
	CustomDomains    []string                 `json:"custom_domains"`
	CustomProcesses  []string                 `json:"custom_processes"`
	CustomSites      []string                 `json:"custom_sites"`
	CustomApps       []string                 `json:"custom_apps"`
	DailyGoalMinutes int                      `json:"daily_goal_minutes"`
	Profiles         map[string]*FocusProfile `json:"profiles"`
}

type CustomProfileInput struct {
	Name         string
	FromProfile  string
	Description  *string
	WorkMinutes  *int
	BreakMinutes *int
	Domains      *[]string
	Processes    *[]string
}

func NormalizeDailyGoalMinutes(minutes int) int {
	return storage.NormalizeDailyGoalMinutes(minutes)
}

func IsBuiltInProfile(name string) bool {
	_, ok := builtInProfileSet[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func ValidateProfileSlug(name string) (string, error) {
	slug := strings.TrimSpace(name)
	if !profileSlugPattern.MatchString(slug) {
		return "", fmt.Errorf("%w: %q", ErrInvalidProfileSlug, name)
	}
	return slug, nil
}

func ValidateAndSanitizeDomains(rawDomains []string) ([]string, error) {
	cleaned := make([]string, 0, len(rawDomains))
	for _, raw := range rawDomains {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		d, err := engine.SanitizeDomain(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid domain %q: %w", raw, err)
		}
		cleaned = append(cleaned, d)
	}
	return engine.SanitizeDomains(cleaned), nil
}

func ValidateAndSanitizeProcesses(rawProcs []string) ([]string, error) {
	seen := make(map[string]struct{}, len(rawProcs))
	out := make([]string, 0, len(rawProcs))
	for _, raw := range rawProcs {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		norm, err := engine.NormalizeProcessName(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid process %q: %w", raw, err)
		}
		if _, exists := seen[norm]; !exists {
			seen[norm] = struct{}{}
			out = append(out, norm)
		}
	}
	return out, nil
}

func GetConfigDir() string {
	return storage.GetStateDir()
}

func GetConfigPath() string {
	return filepath.Join(GetConfigDir(), "config.json")
}

func DefaultProfiles() map[string]*FocusProfile {
	return map[string]*FocusProfile{
		"dev":      DevProfile(),
		"study":    StudyProfile(),
		"hardcore": HardcoreProfile(),
	}
}

func sanitizeProcessList(procs []string) []string {
	seen := make(map[string]struct{}, len(procs))
	out := make([]string, 0, len(procs))
	for _, raw := range procs {
		norm, err := engine.NormalizeProcessName(raw)
		if err != nil {
			continue
		}
		if _, exists := seen[norm]; !exists {
			seen[norm] = struct{}{}
			out = append(out, norm)
		}
	}
	return out
}

func newDefaultConfigStore() *ConfigStore {
	return &ConfigStore{
		CustomDomains:    []string{},
		CustomProcesses:  []string{},
		DailyGoalMinutes: DefaultDailyGoalMinutes,
		Profiles:         DefaultProfiles(),
	}
}

func LoadConfig() (*ConfigStore, error) {
	path := GetConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := newDefaultConfigStore()
			_ = SaveConfig(cfg)
			return cfg, nil
		}
		return nil, err
	}

	clean := bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	trimmed := bytes.TrimSpace(clean)
	if len(trimmed) == 0 {
		cfg := newDefaultConfigStore()
		_ = SaveConfig(cfg)
		return cfg, nil
	}

	var compat rawConfigCompat
	if trimmed[0] != '{' || json.Unmarshal(trimmed, &compat) != nil {
		_ = os.WriteFile(path+".corrupt.bak", data, 0644)
		cfg := newDefaultConfigStore()
		_ = SaveConfig(cfg)
		return cfg, nil
	}

	mergedDomains := append([]string{}, compat.CustomDomains...)
	mergedDomains = append(mergedDomains, compat.CustomSites...)

	mergedProcs := append([]string{}, compat.CustomProcesses...)
	mergedProcs = append(mergedProcs, compat.CustomApps...)

	normalizedProfiles := make(map[string]*FocusProfile)
	for rawKey, p := range compat.Profiles {
		if p == nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(rawKey))
		if key == "" {
			continue
		}
		pCopy := *p
		pCopy.Name = key
		pCopy.BlockedDomains = engine.SanitizeDomains(p.BlockedDomains)
		pCopy.BlockedProcesses = sanitizeProcessList(p.BlockedProcesses)
		if pCopy.DefaultWorkMinutes <= 0 {
			pCopy.DefaultWorkMinutes = 25
		} else if pCopy.DefaultWorkMinutes > 720 {
			pCopy.DefaultWorkMinutes = 720
		}
		if pCopy.DefaultBreakMinutes < 0 {
			pCopy.DefaultBreakMinutes = 5
		} else if pCopy.DefaultBreakMinutes > 120 {
			pCopy.DefaultBreakMinutes = 120
		}
		normalizedProfiles[key] = &pCopy
	}

	cfg := &ConfigStore{
		CustomDomains:    engine.SanitizeDomains(mergedDomains),
		CustomProcesses:  sanitizeProcessList(mergedProcs),
		DailyGoalMinutes: NormalizeDailyGoalMinutes(compat.DailyGoalMinutes),
		Profiles:         normalizedProfiles,
	}

	if _, ok := cfg.Profiles["dev"]; !ok {
		cfg.Profiles["dev"] = DevProfile()
	}
	if _, ok := cfg.Profiles["study"]; !ok {
		cfg.Profiles["study"] = StudyProfile()
	}
	if _, ok := cfg.Profiles["hardcore"]; !ok {
		cfg.Profiles["hardcore"] = HardcoreProfile()
	}

	return cfg, nil
}

func SaveConfig(cfg *ConfigStore) error {
	dir := GetConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	if cfg.CustomDomains == nil {
		cfg.CustomDomains = []string{}
	} else {
		cfg.CustomDomains = engine.SanitizeDomains(cfg.CustomDomains)
	}
	if cfg.CustomProcesses == nil {
		cfg.CustomProcesses = []string{}
	} else {
		cfg.CustomProcesses = sanitizeProcessList(cfg.CustomProcesses)
	}
	cfg.DailyGoalMinutes = NormalizeDailyGoalMinutes(cfg.DailyGoalMinutes)
	if cfg.Profiles == nil {
		cfg.Profiles = DefaultProfiles()
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	cfgPath := GetConfigPath()
	tmpFile, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	_ = tmpFile.Sync()
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	_ = os.Chmod(tmpPath, 0644)
	if err := os.Rename(tmpPath, cfgPath); err != nil {
		if wErr := os.WriteFile(cfgPath, data, 0644); wErr != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		_ = os.Remove(tmpPath)
	}
	return nil
}

func DevProfile() *FocusProfile {
	return &FocusProfile{
		Name:        "dev",
		Description: "Developer Deep Work (Keeps GitHub & Docs open; blocks social media & gaming)",
		BlockedDomains: []string{
			"twitter.com",
			"x.com",
			"instagram.com",
			"tiktok.com",
			"reddit.com",
			"facebook.com",
			"netflix.com",
			"twitch.tv",
		},
		BlockedProcesses: []string{
			"discord.exe",
			"spotify.exe",
			"steam.exe",
			"epicgameslauncher.exe",
			"riotclientservices.exe",
		},
		DefaultWorkMinutes:  50,
		DefaultBreakMinutes: 10,
	}
}

func StudyProfile() *FocusProfile {
	return &FocusProfile{
		Name:        "study",
		Description: "Academic & Exam Preparation Mode (Zero entertainment)",
		BlockedDomains: []string{
			"youtube.com",
			"instagram.com",
			"tiktok.com",
			"twitter.com",
			"x.com",
			"twitch.tv",
			"kick.com",
			"netflix.com",
			"disneyplus.com",
			"reddit.com",
		},
		BlockedProcesses: []string{
			"steam.exe",
			"discord.exe",
			"spotify.exe",
			"telegram.exe",
			"whatsapp.exe",
			"riotclientservices.exe",
			"leagueclient.exe",
			"valorant.exe",
		},
		DefaultWorkMinutes:  25,
		DefaultBreakMinutes: 5,
	}
}

func HardcoreProfile() *FocusProfile {
	p := StudyProfile()
	p.Name = "hardcore"
	p.Description = "Aegis Lockdown - Maximum discipline with Anti-Cheat task suppression"
	return p
}

func resolveAlias(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "coding" {
		return "dev"
	}
	if lower == "lockdown" {
		return "hardcore"
	}
	return lower
}

func (cfg *ConfigStore) SortedProfileNames() []string {
	builtIns := []string{"study", "dev", "hardcore"}
	out := make([]string, 0, len(cfg.Profiles))
	for _, b := range builtIns {
		if _, ok := cfg.Profiles[b]; ok {
			out = append(out, b)
		}
	}
	var custom []string
	for k := range cfg.Profiles {
		if !IsBuiltInProfile(k) {
			custom = append(custom, k)
		}
	}
	sort.Strings(custom)
	out = append(out, custom...)
	return out
}

func (cfg *ConfigStore) GetProfile(name string) (*FocusProfile, bool) {
	if cfg == nil || cfg.Profiles == nil {
		return nil, false
	}
	key := resolveAlias(name)
	p, ok := cfg.Profiles[key]
	if !ok || p == nil {
		return nil, false
	}
	return p, true
}

func (cfg *ConfigStore) UpsertCustomProfile(in CustomProfileInput) (*FocusProfile, bool, error) {
	slug, err := ValidateProfileSlug(in.Name)
	if err != nil {
		return nil, false, err
	}
	if IsBuiltInProfile(slug) {
		return nil, false, fmt.Errorf("%w: %s", ErrBuiltInProfile, slug)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = DefaultProfiles()
	}

	existing, existed := cfg.Profiles[slug]
	workMin := 25
	breakMin := 5
	desc := "Custom focus profile"
	domains := []string{}
	procs := []string{}

	fromKey := resolveAlias(in.FromProfile)
	if fromKey != "" {
		src, ok := cfg.Profiles[fromKey]
		if !ok || src == nil {
			return nil, false, fmt.Errorf("source %w: %s", ErrProfileNotFound, in.FromProfile)
		}
		workMin = src.DefaultWorkMinutes
		breakMin = src.DefaultBreakMinutes
		desc = src.Description
		domains = append([]string{}, src.BlockedDomains...)
		procs = append([]string{}, src.BlockedProcesses...)
	} else if existed && existing != nil {
		workMin = existing.DefaultWorkMinutes
		breakMin = existing.DefaultBreakMinutes
		desc = existing.Description
		domains = append([]string{}, existing.BlockedDomains...)
		procs = append([]string{}, existing.BlockedProcesses...)
	}

	if in.WorkMinutes != nil {
		if *in.WorkMinutes < 1 || *in.WorkMinutes > 720 {
			return nil, false, ErrInvalidWorkMinutes
		}
		workMin = *in.WorkMinutes
	}
	if in.BreakMinutes != nil {
		if *in.BreakMinutes < 0 || *in.BreakMinutes > 120 {
			return nil, false, ErrInvalidBreakMinutes
		}
		breakMin = *in.BreakMinutes
	}
	if in.Description != nil {
		trimmedDesc := strings.TrimSpace(*in.Description)
		if trimmedDesc != "" {
			desc = trimmedDesc
		} else {
			desc = "Custom focus profile"
		}
	}

	if in.Domains != nil {
		cleanDomains, err := ValidateAndSanitizeDomains(*in.Domains)
		if err != nil {
			return nil, false, err
		}
		if fromKey != "" {
			domains = engine.SanitizeDomains(append(domains, cleanDomains...))
		} else {
			domains = cleanDomains
		}
	}

	if in.Processes != nil {
		cleanProcs, err := ValidateAndSanitizeProcesses(*in.Processes)
		if err != nil {
			return nil, false, err
		}
		if fromKey != "" {
			procs = sanitizeProcessList(append(procs, cleanProcs...))
		} else {
			procs = cleanProcs
		}
	}

	prof := &FocusProfile{
		Name:                slug,
		Description:         desc,
		BlockedDomains:      domains,
		BlockedProcesses:    procs,
		DefaultWorkMinutes:  workMin,
		DefaultBreakMinutes: breakMin,
	}
	cfg.Profiles[slug] = prof
	return prof, !existed, nil
}

func (cfg *ConfigStore) DeleteCustomProfile(name string) error {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return ErrProfileNotFound
	}
	if IsBuiltInProfile(key) {
		return fmt.Errorf("%w: %s", ErrBuiltInProfile, key)
	}
	if cfg.Profiles == nil {
		return fmt.Errorf("%w: %s", ErrProfileNotFound, key)
	}
	if _, ok := cfg.Profiles[key]; !ok {
		return fmt.Errorf("%w: %s", ErrProfileNotFound, key)
	}
	delete(cfg.Profiles, key)
	return nil
}

func LoadOrDefault(name string) *FocusProfile {
	lower := resolveAlias(name)

	cfg, err := LoadConfig()
	if err == nil {
		if p, ok := cfg.Profiles[lower]; ok && p != nil {
			pCopy := *p
			domains := append([]string{}, p.BlockedDomains...)
			domains = append(domains, cfg.CustomDomains...)
			pCopy.BlockedDomains = engine.SanitizeDomains(domains)

			procs := append([]string{}, p.BlockedProcesses...)
			procs = append(procs, cfg.CustomProcesses...)
			pCopy.BlockedProcesses = sanitizeProcessList(procs)
			return &pCopy
		}
	}

	var fallback *FocusProfile
	switch lower {
	case "dev":
		fallback = DevProfile()
	case "hardcore":
		fallback = HardcoreProfile()
	default:
		fallback = StudyProfile()
	}

	if err == nil {
		fallback.BlockedDomains = engine.SanitizeDomains(append(fallback.BlockedDomains, cfg.CustomDomains...))
		fallback.BlockedProcesses = sanitizeProcessList(append(fallback.BlockedProcesses, cfg.CustomProcesses...))
	}
	return fallback
}
