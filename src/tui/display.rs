use colored::*;
use indicatif::{ProgressBar, ProgressStyle};
use std::time::Duration;

pub struct TerminalHud {
    progress_bar: ProgressBar,
}

impl TerminalHud {
    pub fn print_banner() {
        println!("{}", r"
  ███████╗ ██████╗  ██████╗██╗   ██╗███████╗███████╗██╗      ██████╗ ██╗    ██╗
  ██╔════╝██╔═══██╗██╔════╝██║   ██║██╔════╝██╔════╝██║     ██╔═══██╗██║    ██║
  █████╗  ██║   ██║██║     ██║   ██║███████╗█████╗  ██║     ██║   ██║██║ █╗ ██║
  ██╔══╝  ██║   ██║██║     ██║   ██║╚════██║██╔══╝  ██║     ██║   ██║██║███╗██║
  ██║     ╚██████╔╝╚██████╗╚██████╔╝███████║██║     ███████╗╚██████╔╝╚███╔███╔╝
  ╚═╝      ╚═════╝  ╚═════╝ ╚═════╝ ╚══════╝╚═╝     ╚══════╝ ╚═════╝  ╚══╝╚══╝ 
        ".bright_cyan().bold());
        println!(
            "  {} {}",
            "🛡️  Kernel-Level Deep Work & Distraction Shield".bright_white().bold(),
            "v2.0.0 (Rust Edition)".bright_green()
        );
        println!("  {}\n", "──────────────────────────────────────────────────────────────────────".dimmed());
    }

    pub fn new_sprint(total_seconds: u64, profile_name: &str) -> Self {
        let pb = ProgressBar::new(total_seconds);
        pb.set_style(
            ProgressStyle::default_bar()
                .template("  [{elapsed_precise}] [{bar:40.cyan/blue}] {pos}/{len}s ({percent}%) | {msg}")
                .unwrap()
                .progress_chars("█▓▒░ "),
        );
        pb.set_message(format!("Profile: {} | Active Guard", profile_name.bright_yellow()));
        Self { progress_bar: pb }
    }

    pub fn update(&self, elapsed: u64, blocked_count: u32) {
        self.progress_bar.set_position(elapsed);
        self.progress_bar.set_message(format!(
            "Blocked Distractions: {}",
            blocked_count.to_string().bright_red().bold()
        ));
    }

    pub fn finish(&self) {
        self.progress_bar.finish_with_message("Sprint Complete! Take a well-deserved break 🎉".bright_green().to_string());
    }
}
