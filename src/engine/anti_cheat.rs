use crate::engine::process::ProcessSupervisor;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum StrictnessLevel {
    Relaxed,  // Only blocks websites and games, allows Task Manager
    Standard, // Blocks websites, distraction apps, logs attempts
    Hardcore, // Actively suppresses Task Manager and terminal escapes during active sprint
}

pub struct AntiCheatGuard {
    level: StrictnessLevel,
    supervisor: ProcessSupervisor,
}

impl AntiCheatGuard {
    pub fn new(level: StrictnessLevel) -> Self {
        let blocked = match level {
            StrictnessLevel::Hardcore => vec![
                "taskmgr.exe".to_string(),
                "processhacker.exe".to_string(),
                "procmon.exe".to_string(),
                "procexp.exe".to_string(),
            ],
            StrictnessLevel::Standard | StrictnessLevel::Relaxed => Vec::new(),
        };

        Self {
            level,
            supervisor: ProcessSupervisor::new(&blocked),
        }
    }

    pub fn enforce(&self) -> Vec<String> {
        if self.level == StrictnessLevel::Hardcore {
            self.supervisor.scan_and_terminate()
        } else {
            Vec::new()
        }
    }
}
