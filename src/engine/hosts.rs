use std::fs::{self, File, OpenOptions};
use std::io::{self, Read, Write};
use std::path::{Path, PathBuf};
use sha2::{Digest, Sha256};

pub const HOSTS_PATH: &str = r"C:\Windows\System32\drivers\etc\hosts";
pub const BACKUP_SUFFIX: &str = ".focusflow.bak";
pub const MARKER_BEGIN: &str = "# --- BEGIN FOCUSFLOW MANAGED BLOCK ---";
pub const MARKER_END: &str = "# --- END FOCUSFLOW MANAGED BLOCK ---";

#[derive(Debug)]
pub struct HostsManager {
    hosts_path: PathBuf,
    backup_path: PathBuf,
}

impl HostsManager {
    pub fn new() -> Self {
        Self::with_path(PathBuf::from(HOSTS_PATH))
    }

    pub fn with_path(hosts_path: PathBuf) -> Self {
        let mut backup_path = hosts_path.clone();
        backup_path.set_extension("focusflow.bak");
        Self {
            hosts_path,
            backup_path,
        }
    }

    pub fn compute_sha256(path: &Path) -> io::Result<String> {
        let mut file = File::open(path)?;
        let mut hasher = Sha256::new();
        let mut buffer = [0u8; 8192];
        loop {
            let bytes_read = file.read(&mut buffer)?;
            if bytes_read == 0 {
                break;
            }
            hasher.update(&buffer[..bytes_read]);
        }
        Ok(format!("{:x}", hasher.finalize()))
    }

    pub fn backup(&self) -> io::Result<String> {
        if !self.hosts_path.exists() {
            return Err(io::Error::new(
                io::ErrorKind::NotFound,
                format!("Hosts file not found at {:?}", self.hosts_path),
            ));
        }

        fs::copy(&self.hosts_path, &self.backup_path)?;
        Self::compute_sha256(&self.backup_path)
    }

    pub fn apply_blocklist(&self, domains: &[String]) -> io::Result<()> {
        if domains.is_empty() {
            return Ok(());
        }

        // Ensure backup exists first
        if !self.backup_path.exists() {
            self.backup()?;
        }

        let content = fs::read_to_string(&self.hosts_path)?;
        let clean_content = self.strip_managed_block(&content);

        let mut block_entries = String::new();
        block_entries.push_str("\n");
        block_entries.push_str(MARKER_BEGIN);
        block_entries.push_str("\n");
        block_entries.push_str("# FocusFlow V2 - Distraction-free system guard\n");

        for domain in domains {
            let trimmed = domain.trim().to_lowercase();
            if !trimmed.is_empty() && !trimmed.starts_with('#') {
                block_entries.push_str(&format!("0.0.0.0 {}\n", trimmed));
                if !trimmed.starts_with("www.") {
                    block_entries.push_str(&format!("0.0.0.0 www.{}\n", trimmed));
                }
            }
        }

        block_entries.push_str(MARKER_END);
        block_entries.push_str("\n");

        let new_content = format!("{}{}", clean_content.trim_end(), block_entries);

        // Atomic file write using temporary file in same folder
        let temp_path = self.hosts_path.with_extension("tmp");
        {
            let mut temp_file = OpenOptions::new()
                .write(true)
                .create(true)
                .truncate(true)
                .open(&temp_path)?;
            temp_file.write_all(new_content.as_bytes())?;
            temp_file.flush()?;
        }

        fs::rename(&temp_path, &self.hosts_path)?;
        Ok(())
    }

    pub fn rollback(&self) -> io::Result<()> {
        if self.backup_path.exists() {
            fs::copy(&self.backup_path, &self.hosts_path)?;
            let _ = fs::remove_file(&self.backup_path);
            return Ok(());
        }

        // Fallback: Strip managed block directly
        if self.hosts_path.exists() {
            let content = fs::read_to_string(&self.hosts_path)?;
            let clean_content = self.strip_managed_block(&content);
            fs::write(&self.hosts_path, clean_content)?;
        }

        Ok(())
    }

    fn strip_managed_block(&self, content: &str) -> String {
        let mut result = String::new();
        let mut inside_block = false;

        for line in content.lines() {
            if line.contains(MARKER_BEGIN) {
                inside_block = true;
                continue;
            }
            if line.contains(MARKER_END) {
                inside_block = false;
                continue;
            }
            if !inside_block {
                result.push_str(line);
                result.push('\n');
            }
        }

        result
    }

    pub fn is_active(&self) -> bool {
        if let Ok(content) = fs::read_to_string(&self.hosts_path) {
            content.contains(MARKER_BEGIN)
        } else {
            false
        }
    }
}
