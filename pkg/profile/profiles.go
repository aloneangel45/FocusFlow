package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/aloneangel45/focusflow/pkg/engine"
	"github.com/aloneangel45/focusflow/pkg/storage"
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
	CustomDomains   []string                 `json:"custom_domains"`
	CustomProcesses []string                 `json:"custom_processes"`
	Profiles        map[string]*FocusProfile `json:"profiles"`
}

type rawConfigCompat struct {
	CustomDomains   []string                 `json:"custom_domains"`
	CustomProcesses []string                 `json:"custom_processes"`
	CustomSites     []string                 `json:"custom_sites"`
	CustomApps      []string                 `json:"custom_apps"`
	Profiles        map[string]*FocusProfile `json:"profiles"`
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

func LoadConfig() (*ConfigStore, error) {
	path := GetConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := &ConfigStore{
				CustomDomains:   []string{},
				CustomProcesses: []string{},
				Profiles:        DefaultProfiles(),
			}
			_ = SaveConfig(cfg)
			return cfg, nil
		}
		return nil, err
	}

	var compat rawConfigCompat
	if err := json.Unmarshal(data, &compat); err != nil {
		return nil, err
	}

	mergedDomains := append([]string{}, compat.CustomDomains...)
	mergedDomains = append(mergedDomains, compat.CustomSites...)

	mergedProcs := append([]string{}, compat.CustomProcesses...)
	mergedProcs = append(mergedProcs, compat.CustomApps...)

	cfg := &ConfigStore{
		CustomDomains:   engine.SanitizeDomains(mergedDomains),
		CustomProcesses: sanitizeProcessList(mergedProcs),
		Profiles:        compat.Profiles,
	}

	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]*FocusProfile)
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

	for key, p := range cfg.Profiles {
		if p == nil {
			continue
		}
		if p.Name == "" {
			p.Name = key
		}
		p.BlockedDomains = engine.SanitizeDomains(p.BlockedDomains)
		p.BlockedProcesses = sanitizeProcessList(p.BlockedProcesses)
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

func LoadOrDefault(name string) *FocusProfile {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "coding" {
		lower = "dev"
	} else if lower == "lockdown" {
		lower = "hardcore"
	}

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
