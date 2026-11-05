package profile

import "strings"

type FocusProfile struct {
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	BlockedDomains      []string `json:"blocked_domains"`
	BlockedProcesses    []string `json:"blocked_processes"`
	DefaultWorkMinutes  int      `json:"default_work_minutes"`
	DefaultBreakMinutes int      `json:"default_break_minutes"`
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
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "dev", "coding":
		return DevProfile()
	case "hardcore", "lockdown":
		return HardcoreProfile()
	default:
		return StudyProfile()
	}
}
