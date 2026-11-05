use std::process::Command;
use std::io;

pub struct DnsEngine;

impl DnsEngine {
    /// Flushes Windows resolver cache so hosts edits take effect immediately across all sockets and browsers.
    pub fn flush_resolver_cache() -> io::Result<()> {
        let output = Command::new("ipconfig")
            .arg("/flushdns")
            .output()?;

        if output.status.success() {
            Ok(())
        } else {
            Err(io::Error::new(
                io::ErrorKind::Other,
                format!("Failed to flush DNS: {}", String::from_utf8_lossy(&output.stderr)),
            ))
        }
    }

    /// Common DNS-over-HTTPS (DoH) endpoints that can bypass hosts file if left open in Chrome/Brave/Edge.
    pub fn get_doh_bypass_domains() -> Vec<String> {
        vec![
            "cloudflare-dns.com".to_string(),
            "one.one.one.one".to_string(),
            "dns.google".to_string(),
            "dns.google.com".to_string(),
            "dns.quad9.net".to_string(),
            "doh.opendns.com".to_string(),
            "chrome.cloudflare-dns.com".to_string(),
            "mozilla.cloudflare-dns.com".to_string(),
        ]
    }
}
