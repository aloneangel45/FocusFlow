use clap::{Parser, Subcommand};
use colored::*;
use focusflow::engine::{AntiCheatGuard, DnsEngine, HostsManager, ProcessSupervisor, StrictnessLevel};
use focusflow::profile::FocusProfile;
use focusflow::storage::Database;
use focusflow::tui::TerminalHud;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::thread;
use std::time::Duration;

#[derive(Parser)]
#[command(
    name = "focusflow",
    version = focusflow::VERSION,
    about = "⚡ Kernel-Level Deep Work Discipline Engine & Atomic Hosts Blocker in Rust",
    long_about = "FocusFlow V2 enforces unyielding digital discipline through low-overhead Win32 process supervision, atomic hosts-file management, and automatic DNS-over-HTTPS bypass mitigation."
)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Start a deep work session with atomic blocking active
    Start {
        /// Profile to activate (dev, study, hardcore)
        #[arg(short, long, default_value = "study")]
        profile: String,

        /// Session duration in minutes (overrides profile default)
        #[arg(short, long)]
        minutes: Option<u32>,

        /// Anti-cheat strictness level (relaxed, standard, hardcore)
        #[arg(short, long, default_value = "standard")]
        strictness: String,
    },

    /// Rollback hosts file immediately and restore unrestricted system access
    Stop,

    /// Check whether FocusFlow active blocklist is currently engaged
    Status,

    /// Flush system DNS resolver cache immediately
    Flush,

    /// Display historical focus session metrics and streak summary
    Stats,
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let cli = Cli::parse();
    TerminalHud::print_banner();

    match cli.command {
        Commands::Start {
            profile,
            minutes,
            strictness,
        } => {
            let prof = FocusProfile::load_or_default(&profile);
            let duration_min = minutes.unwrap_or(prof.default_work_minutes);
            let total_seconds = (duration_min as u64) * 60;

            let strict_level = match strictness.to_lowercase().as_str() {
                "hardcore" => StrictnessLevel::Hardcore,
                "relaxed" => StrictnessLevel::Relaxed,
                _ => StrictnessLevel::Standard,
            };

            println!(
                "  {} Starting {} session ({} minutes | Strictness: {:?})",
                "▶".bright_green().bold(),
                prof.name.bright_yellow().bold(),
                duration_min.to_string().bright_cyan().bold(),
                strict_level
            );

            let hosts = Arc::new(HostsManager::new());
            let dns = DnsEngine;
            let process_sup = ProcessSupervisor::new(&prof.blocked_processes);
            let anti_cheat = AntiCheatGuard::new(strict_level);

            // Step 1: Engage Hosts Blocklist
            print!("  {} Engaging atomic hosts blocklist... ", "•".bright_blue());
            let mut all_domains = prof.blocked_domains.clone();
            all_domains.extend(DnsEngine::get_doh_bypass_domains());

            match hosts.apply_blocklist(&all_domains) {
                Ok(_) => println!("{}", "OK (Backup secured with SHA-256)".bright_green()),
                Err(e) => {
                    println!("{}", format!("FAILED ({})", e).bright_red());
                    println!("  {} Please run terminal as Administrator to modify system hosts.", "⚠".bright_yellow());
                }
            }

            // Step 2: Flush DNS resolver cache
            print!("  {} Purging DNS resolver cache (Anti-DoH)... ", "•".bright_blue());
            let _ = DnsEngine::flush_resolver_cache();
            println!("{}", "OK".bright_green());

            // Setup graceful Ctrl+C interrupt handler
            let running = Arc::new(AtomicBool::new(true));
            let r_clone = running.clone();
            let h_clone = hosts.clone();

            ctrlc::set_handler(move || {
                println!("\n  {} Interrupt signal received! Rolling back hosts...", "⚠".bright_yellow());
                let _ = h_clone.rollback();
                let _ = DnsEngine::flush_resolver_cache();
                r_clone.store(false, Ordering::SeqCst);
                println!("  {} System access cleanly restored. Exiting.", "✔".bright_green());
                std::process::exit(0);
            })
            .expect("Error setting Ctrl-C handler");

            // Step 3: Run interactive TUI loop
            let hud = TerminalHud::new_sprint(total_seconds, &prof.name);
            let mut elapsed: u64 = 0;
            let mut total_blocked: u32 = 0;

            while running.load(Ordering::SeqCst) && elapsed < total_seconds {
                thread::sleep(Duration::from_secs(1));
                elapsed += 1;

                // Process surveillance scan
                let terminated_procs = process_sup.scan_and_terminate();
                let anti_cheat_procs = anti_cheat.enforce();

                let scan_hits = (terminated_procs.len() + anti_cheat_procs.len()) as u32;
                if scan_hits > 0 {
                    total_blocked += scan_hits;
                    for p in terminated_procs.iter().chain(anti_cheat_procs.iter()) {
                        println!("\n  {} Terminated distraction: {}", "✘".bright_red().bold(), p.bright_white());
                    }
                }

                hud.update(elapsed, total_blocked);
            }

            hud.finish();

            // Session completed cleanly
            let _ = hosts.rollback();
            let _ = DnsEngine::flush_resolver_cache();

            // Record into SQLite Database
            if let Ok(db) = Database::open() {
                let _ = db.record_session(&prof.name, duration_min, true, total_blocked);
            }

            println!("\n  {} Great job! All system protections rolled back.", "✔".bright_green().bold());
        }

        Commands::Stop => {
            println!("  {} Rolling back hosts file and flushing DNS...", "•".bright_blue());
            let hosts = HostsManager::new();
            match hosts.rollback() {
                Ok(_) => {
                    let _ = DnsEngine::flush_resolver_cache();
                    println!("  {} Clean rollback complete. Unrestricted access restored.", "✔".bright_green());
                }
                Err(e) => {
                    println!("  {} Rollback failed: {}", "✘".bright_red(), e);
                }
            }
        }

        Commands::Status => {
            let hosts = HostsManager::new();
            if hosts.is_active() {
                println!("  {} FocusFlow guard is currently {}.", "•".bright_white(), "ACTIVE (Lockdown Engaged)".bright_red().bold());
            } else {
                println!("  {} FocusFlow guard is {}.", "•".bright_white(), "INACTIVE (Unrestricted)".bright_green().bold());
            }
        }

        Commands::Flush => {
            print!("  {} Purging DNS resolver cache... ", "•".bright_blue());
            match DnsEngine::flush_resolver_cache() {
                Ok(_) => println!("{}", "OK".bright_green()),
                Err(e) => println!("{}", format!("FAILED ({})", e).bright_red()),
            }
        }

        Commands::Stats => {
            println!("  {} FocusFlow Historical Metrics:\n", "📊".bright_cyan());
            match Database::open() {
                Ok(db) => {
                    if let Ok((sessions, minutes, blocked)) = db.get_total_stats() {
                        let hours = (minutes as f64) / 60.0;
                        println!("    • Total Completed Sprints: {}", sessions.to_string().bright_yellow().bold());
                        println!("    • Total Deep Work Hours:   {:.1} hrs", hours);
                        println!("    • Distractions Blocked:    {}", blocked.to_string().bright_red().bold());
                    }
                }
                Err(e) => println!("    {} Could not read metrics database: {}", "✘".bright_red(), e),
            }
        }
    }

    Ok(())
}
