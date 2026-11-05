use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FocusProfile {
    pub name: String,
    pub description: String,
    pub blocked_domains: Vec<String>,
    pub blocked_processes: Vec<String>,
    pub default_work_minutes: u32,
    pub default_break_minutes: u32,
}

impl FocusProfile {
    pub fn dev_profile() -> Self {
        Self {
            name: "dev".to_string(),
            description: "Developer Deep Work (Keeps GitHub & Docs open; blocks social & gaming)".to_string(),
            blocked_domains: vec![
                "twitter.com".to_string(),
                "x.com".to_string(),
                "instagram.com".to_string(),
                "tiktok.com".to_string(),
                "reddit.com".to_string(),
                "facebook.com".to_string(),
                "netflix.com".to_string(),
                "twitch.tv".to_string(),
            ],
            blocked_processes: vec![
                "discord.exe".to_string(),
                "spotify.exe".to_string(),
                "steam.exe".to_string(),
                "epicgameslauncher.exe".to_string(),
                "riotclientservices.exe".to_string(),
            ],
            default_work_minutes: 50,
            default_break_minutes: 10,
        }
    }

    pub fn study_profile() -> Self {
        Self {
            name: "study".to_string(),
            description: "Academic & Exam Preparation Mode (YKS/KPSS/LGS - Zero entertainment)".to_string(),
            blocked_domains: vec![
                "youtube.com".to_string(),
                "instagram.com".to_string(),
                "tiktok.com".to_string(),
                "twitter.com".to_string(),
                "x.com".to_string(),
                "twitch.tv".to_string(),
                "kick.com".to_string(),
                "netflix.com".to_string(),
                "disneyplus.com".to_string(),
                "reddit.com".to_string(),
            ],
            blocked_processes: vec![
                "steam.exe".to_string(),
                "discord.exe".to_string(),
                "spotify.exe".to_string(),
                "telegram.exe".to_string(),
                "whatsapp.exe".to_string(),
                "riotclientservices.exe".to_string(),
                "leagueclient.exe".to_string(),
                "valorant.exe".to_string(),
            ],
            default_work_minutes: 25,
            default_break_minutes: 5,
        }
    }

    pub fn hardcore_profile() -> Self {
        let mut p = Self::study_profile();
        p.name = "hardcore".to_string();
        p.description = "Aegis Lockdown - Maximum discipline with Anti-Cheat guardrails".to_string();
        p
    }

    pub fn load_or_default(profile_name: &str) -> Self {
        match profile_name.to_lowercase().as_str() {
            "dev" | "coding" => Self::dev_profile(),
            "hardcore" | "lockdown" => Self::hardcore_profile(),
            _ => Self::study_profile(),
        }
    }
}
