use serde::{Deserialize, Serialize};
use std::fs::{self, File};
use std::io::{self, BufReader, BufWriter};
use std::path::PathBuf;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SessionRecord {
    pub profile: String,
    pub start_time: String,
    pub duration_minutes: u32,
    pub completed: bool,
    pub blocked_count: u32,
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct MetricsStore {
    pub sessions: Vec<SessionRecord>,
}

pub struct Database {
    db_path: Option<PathBuf>,
    in_memory: Option<std::sync::Mutex<MetricsStore>>,
}

impl Database {
    pub fn open() -> io::Result<Self> {
        let db_dir = Self::get_db_dir();
        fs::create_dir_all(&db_dir)?;
        let db_path = db_dir.join("sessions.json");

        if !db_path.exists() {
            let initial = MetricsStore::default();
            let writer = BufWriter::new(File::create(&db_path)?);
            serde_json::to_writer_pretty(writer, &initial)?;
        }

        Ok(Self {
            db_path: Some(db_path),
            in_memory: None,
        })
    }

    pub fn open_in_memory() -> io::Result<Self> {
        Ok(Self {
            db_path: None,
            in_memory: Some(std::sync::Mutex::new(MetricsStore::default())),
        })
    }

    fn get_db_dir() -> PathBuf {
        if let Some(app_data) = std::env::var_os("APPDATA") {
            PathBuf::from(app_data).join("FocusFlow")
        } else {
            PathBuf::from(".focusflow")
        }
    }

    fn load(&self) -> io::Result<MetricsStore> {
        if let Some(ref mem) = self.in_memory {
            let guard = mem.lock().unwrap();
            return Ok(guard.clone());
        }

        let path = self.db_path.as_ref().unwrap();
        if !path.exists() {
            return Ok(MetricsStore::default());
        }

        let reader = BufReader::new(File::open(path)?);
        let store = serde_json::from_reader(reader).unwrap_or_default();
        Ok(store)
    }

    fn save(&self, store: &MetricsStore) -> io::Result<()> {
        if let Some(ref mem) = self.in_memory {
            let mut guard = mem.lock().unwrap();
            *guard = store.clone();
            return Ok(());
        }

        let path = self.db_path.as_ref().unwrap();
        let writer = BufWriter::new(File::create(path)?);
        serde_json::to_writer_pretty(writer, store)?;
        Ok(())
    }

    pub fn record_session(
        &self,
        profile: &str,
        duration_minutes: u32,
        completed: bool,
        blocked_count: u32,
    ) -> io::Result<()> {
        let mut store = self.load()?;
        let start_time = chrono::Local::now().to_rfc3339();

        store.sessions.push(SessionRecord {
            profile: profile.to_string(),
            start_time,
            duration_minutes,
            completed,
            blocked_count,
        });

        self.save(&store)
    }

    pub fn get_total_stats(&self) -> io::Result<(u32, u32, u32)> {
        let store = self.load()?;
        let mut total_sessions: u32 = 0;
        let mut total_minutes: u32 = 0;
        let mut total_blocked: u32 = 0;

        for s in &store.sessions {
            if s.completed {
                total_sessions += 1;
                total_minutes += s.duration_minutes;
                total_blocked += s.blocked_count;
            }
        }

        Ok((total_sessions, total_minutes, total_blocked))
    }
}
