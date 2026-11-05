package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func GetConfigDir() string {
	appData := os.Getenv("APPDATA")
	if appData != "" {
		return filepath.Join(appData, "FocusFlow")
	}
	return ".focusflow"
}

func GetConfigPath() string {
	return filepath.Join(GetConfigDir(), "config.json")
}

func LoadConfig() (*ConfigStore, error) {
	path := GetConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		// Return default config
		cfg := &ConfigStore{
			CustomDomains:   []string{},
			CustomProcesses: []string{},
			Profiles: map[string]*FocusProfile{
				"dev":      DevProfile(),
				"study":    StudyProfile(),
				"hardcore": HardcoreProfile(),
			},
		}
		_ = SaveConfig(cfg)
		return cfg, nil
	}

	var cfg ConfigStore
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
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
	return &cfg, nil
}

func SaveConfig(cfg *ConfigStore) error {
	dir := GetConfigDir()
	_ = os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(GetConfigPath(), data, 0644)
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
		Description: "Academic & Exam Preparation Mode (YKS/KPSS/LGS - Zero entertainment)",
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
	cfg, err := LoadConfig()
	if err == nil {
		lower := strings.ToLower(strings.TrimSpace(name))
		if p, ok := cfg.Profiles[lower]; ok {
			// Merge global custom domains & processes
			pCopy := *p
			pCopy.BlockedDomains = append(pCopy.BlockedDomains, cfg.CustomDomains...)
			pCopy.BlockedProcesses = append(pCopy.BlockedProcesses, cfg.CustomProcesses...)
			return &pCopy
		}
	}

	switch strings.ToLower(strings.TrimSpace(name)) {
	case "dev", "coding":
		return DevProfile()
	case "hardcore", "lockdown":
		return HardcoreProfile()
	default:
		return StudyProfile()
	}
}
