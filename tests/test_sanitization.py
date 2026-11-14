"""Tests for RFC-1123 domain sanitization, Anti-DoH domains, and protected process denylist."""

from __future__ import annotations

import pytest

from focusflow_core.sanitize import (
    DOH_BYPASS_DOMAINS,
    DomainValidationError,
    ProcessValidationError,
    is_protected_process,
    normalize_process_name,
    sanitize_domain,
    sanitize_domains,
)


@pytest.mark.parametrize(
    ("raw", "expected"),
    [
        ("youtube.com", "youtube.com"),
        ("  HTTPS://www.YouTube.com:443/watch?v=123#top  ", "youtube.com"),
        ("http://Sub.Reddit.com:8080/r/python/", "sub.reddit.com"),
        ("x.com.", "x.com"),
        ("https://www.reddit.com.:8443/r/python", "reddit.com"),
        ("dns.adguard-dns.com", "dns.adguard-dns.com"),
        ("one.one.one.one", "one.one.one.one"),
    ],
)
def test_sanitize_domain_valid(raw: str, expected: str) -> None:
    assert sanitize_domain(raw) == expected


@pytest.mark.parametrize(
    "bad_raw",
    [
        "",
        "   ",
        "# comment",
        "evil.com\n127.0.0.1 bank.com",
        "evil.com\r\n0.0.0.0 github.com",
        "evil.com\tinjected.com",
        "evil.com injected.com",
        "evil\x00.com",
        "127.0.0.1",
        "0.0.0.0",
        "192.168.1.1",
        "999.999.999.999",
        "::1",
        "[::1]:8080",
        "2001:db8::1",
        "localhost",
        "LOCALHOST",
        "localdomain",
        "localhost.localdomain",
        "printer.local",
        "dev.localhost",
        "box.localdomain",
        "singlelabel",
        "-bad.com",
        "bad-.com",
        "bad..dots.com",
        "example.com:99999",
        "example.com:0",
        "example.com:abc",
        "example.com:\u00b2",
        "ftp://example.com",
        "//reddit.com/r/python",
        "user@example.com",
        "https://user:pass@reddit.com:443/path",
        "reddit.com..",
        "www.localhost",
        "www.ip6-localhost",
        "www.ip6-loopback",
        "www.broadcasthost",
        "www.127.0.0.1",
        "www.0.0.0.0",
        "home.arpa",
        "router.home.arpa",
        "domain.invalid",
        "0x7f.0.0.0x1",
        "0177.0.0.1",
        "example.1com",
        "example.c",
    ],
)
def test_sanitize_domain_rejects_unsafe_inputs(bad_raw: str) -> None:
    with pytest.raises(DomainValidationError):
        sanitize_domain(bad_raw)


def test_sanitize_domains_deduplicates_deterministically() -> None:
    cleaned = sanitize_domains(
        ["https://www.Twitter.com/home", "twitter.com", "localhost", "127.0.0.1", "reddit.com"]
    )
    assert cleaned == ["twitter.com", "reddit.com"]


def test_expanded_doh_bypass_domains_present() -> None:
    required = {
        "cloudflare-dns.com",
        "dns.google",
        "dns.quad9.net",
        "doh.opendns.com",
        "dns.adguard-dns.com",
        "dns.nextdns.io",
        "doh.cleanbrowsing.org",
        "doh.dns.sb",
        "freedns.controld.com",
    }
    assert required.issubset(set(DOH_BYPASS_DOMAINS))


@pytest.mark.parametrize(
    "proc_name",
    [
        "csrss.exe",
        "smss.exe",
        "lsass.exe",
        "services.exe",
        "svchost.exe",
        "wininit.exe",
        "winlogon.exe",
        "dwm.exe",
        "explorer.exe",
        "sihost.exe",
        "fontdrvhost.exe",
        "focusflow.exe",
        "python.exe",
        "pythonw.exe",
        "conhost.exe",
        "wt.exe",
        "EXPLORER",
        "PythonW",
        r"C:\Windows\System32\svchost.exe",
    ],
)
def test_protected_process_denylist(proc_name: str) -> None:
    assert is_protected_process(proc_name)
    with pytest.raises(ProcessValidationError):
        normalize_process_name(proc_name)


def test_normalize_process_name_valid() -> None:
    assert normalize_process_name("  Discord  ") == "discord.exe"
    assert normalize_process_name("Steam.EXE") == "steam.exe"
