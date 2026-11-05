use focusflow::engine::HostsManager;
use focusflow::profile::FocusProfile;
use focusflow::storage::Database;
use std::fs;
use tempfile::NamedTempFile;

#[test]
fn test_hosts_atomic_apply_and_rollback() {
    // Create temporary mock hosts file
    let temp_hosts = NamedTempFile::new().expect("Failed to create temp hosts file");
    let initial_content = "127.0.0.1 localhost\n::1 localhost\n";
    fs::write(temp_hosts.path(), initial_content).expect("Failed to write initial hosts");

    let initial_hash = HostsManager::compute_sha256(temp_hosts.path()).expect("Hash calculation failed");

    let manager = HostsManager::with_path(temp_hosts.path().to_path_buf());

    // Apply blocklist
    let domains = vec!["instagram.com".to_string(), "tiktok.com".to_string()];
    manager.apply_blocklist(&domains).expect("Failed to apply blocklist");

    assert!(manager.is_active(), "Manager should report active");

    let modified_content = fs::read_to_string(temp_hosts.path()).expect("Failed to read modified hosts");
    assert!(modified_content.contains("0.0.0.0 instagram.com"));
    assert!(modified_content.contains("0.0.0.0 www.instagram.com"));
    assert!(modified_content.contains("0.0.0.0 tiktok.com"));

    // Rollback
    manager.rollback().expect("Failed to rollback");

    assert!(!manager.is_active(), "Manager should report inactive after rollback");

    let restored_hash = HostsManager::compute_sha256(temp_hosts.path()).expect("Restored hash failed");
    assert_eq!(initial_hash, restored_hash, "Restored hosts file must match original hash byte-for-byte!");
}

#[test]
fn test_database_in_memory_metrics() {
    let db = Database::open_in_memory().expect("Failed to open in-memory DB");

    // Record two successful sessions
    db.record_session("dev", 50, true, 5).expect("Record 1 failed");
    db.record_session("study", 25, true, 2).expect("Record 2 failed");

    // Record one failed/aborted session (should not count in completed)
    db.record_session("dev", 10, false, 1).expect("Record 3 failed");

    let (total_sessions, total_minutes, total_blocked) = db.get_total_stats().expect("Stats query failed");
    assert_eq!(total_sessions, 2);
    assert_eq!(total_minutes, 75);
    assert_eq!(total_blocked, 7);
}

#[test]
fn test_profiles_structure() {
    let dev = FocusProfile::dev_profile();
    assert_eq!(dev.name, "dev");
    assert!(dev.blocked_domains.contains(&"twitter.com".to_string()));
    assert!(dev.blocked_processes.contains(&"discord.exe".to_string()));

    let study = FocusProfile::study_profile();
    assert_eq!(study.name, "study");
    assert!(study.blocked_domains.contains(&"youtube.com".to_string()));
}
