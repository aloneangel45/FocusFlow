"""Strict RFC-1123 domain sanitization, Anti-DoH list, and protected process denylist."""

from __future__ import annotations

import ipaddress
import re

DOH_BYPASS_DOMAINS: tuple[str, ...] = (
    "cloudflare-dns.com",
    "one.one.one.one",
    "chrome.cloudflare-dns.com",
    "mozilla.cloudflare-dns.com",
    "dns.google",
    "dns.google.com",
    "dns.quad9.net",
    "doh.opendns.com",
    "dns.adguard-dns.com",
    "dns.nextdns.io",
    "doh.cleanbrowsing.org",
    "doh.dns.sb",
    "freedns.controld.com",
)

RESERVED_EXACT_DOMAINS: frozenset[str] = frozenset(
    {
        "localhost",
        "localhost.localdomain",
        "localdomain",
        "local",
        "broadcasthost",
        "ip6-localhost",
        "ip6-loopback",
        "ip6-localnet",
        "ip6-mcastprefix",
        "ip6-allnodes",
        "ip6-allrouters",
        "ip6-allhosts",
        "home.arpa",
        "invalid",
    }
)

_PROTECTED_BASE_NAMES: tuple[str, ...] = (
    "csrss",
    "smss",
    "lsass",
    "services",
    "svchost",
    "wininit",
    "winlogon",
    "dwm",
    "explorer",
    "sihost",
    "fontdrvhost",
    "focusflow",
    "python",
    "python3",
    "pythonw",
    "conhost",
    "wt",
    "init",
    "systemd",
    "launchd",
    "kernel_task",
)

PROTECTED_PROCESS_NAMES: frozenset[str] = frozenset(
    name
    for base in _PROTECTED_BASE_NAMES
    for name in (base.lower(), f"{base.lower()}.exe")
)

_LABEL_RE = re.compile(r"^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$")


class DomainValidationError(ValueError):
    """Raised when a domain fails strict RFC-1123 or sinkhole safety validation."""


class ProcessValidationError(ValueError):
    """Raised when a process executable name is invalid or protected."""


def _is_ip_literal(candidate: str) -> bool:
    try:
        ipaddress.ip_address(candidate)
        return True
    except ValueError:
        return False


def _is_reserved_domain(candidate: str) -> bool:
    return candidate in RESERVED_EXACT_DOMAINS or candidate.endswith(
        (".local", ".localhost", ".localdomain", ".home.arpa", ".invalid")
    )


def sanitize_domain(raw: str) -> str:
    """Strictly normalize and validate a domain name per RFC-1123.

    Strips http(s)://, paths, query strings, fragments, valid ports, outer spaces, and leading www.
    Rejects newlines, control characters, embedded spaces, IP literals, localhost, localdomain, and *.local.
    """
    if not isinstance(raw, str):
        raise DomainValidationError("domain must be a string")

    for ch in raw:
        code = ord(ch)
        if code < 0x20 or code == 0x7F:
            raise DomainValidationError("domain contains illegal control characters")

    s = raw.strip(" ")
    if not s or s.startswith("#"):
        raise DomainValidationError("domain cannot be empty")

    if any(ws in s for ws in (" ", "\t", "\r", "\n")):
        raise DomainValidationError("domain contains embedded whitespace")

    s = s.lower()

    if s.startswith("https://"):
        s = s[len("https://") :]
    elif s.startswith("http://"):
        s = s[len("http://") :]
    elif "://" in s or s.startswith("//"):
        raise DomainValidationError("unsupported URL scheme")

    for sep in ("/", "?", "#"):
        idx = s.find(sep)
        if idx >= 0:
            s = s[:idx]

    if not s:
        raise DomainValidationError("domain cannot be empty")

    if "[" in s or "]" in s:
        raise DomainValidationError("IPv6 literals cannot be blocked")
    if "@" in s:
        raise DomainValidationError("userinfo is not allowed in domain")

    colon_count = s.count(":")
    if colon_count > 1:
        raise DomainValidationError("IPv6 literals cannot be blocked")
    if colon_count == 1:
        host_part, port_part = s.split(":", 1)
        if not port_part or not port_part.isascii() or not port_part.isdigit():
            raise DomainValidationError("invalid port in domain")
        port_num = int(port_part)
        if port_num < 1 or port_num > 65535:
            raise DomainValidationError("port out of range")
        s = host_part

    if s.endswith(".."):
        raise DomainValidationError("invalid consecutive trailing dots in domain")
    if s.endswith("."):
        s = s[:-1]
    if not s:
        raise DomainValidationError("domain cannot be empty")

    if _is_ip_literal(s):
        raise DomainValidationError("IP literals cannot be blocked")

    if _is_reserved_domain(s):
        raise DomainValidationError("reserved or local domain cannot be blocked")

    if s.startswith("www."):
        rest = s[4:]
        if not rest or _is_ip_literal(rest) or _is_reserved_domain(rest):
            raise DomainValidationError("reserved or local domain cannot be blocked")
        if "." in rest:
            s = rest

    if _is_ip_literal(s):
        raise DomainValidationError("IP literals cannot be blocked")
    if _is_reserved_domain(s):
        raise DomainValidationError("reserved or local domain cannot be blocked")

    if len(s) > 253 or "." not in s:
        raise DomainValidationError("invalid RFC-1123 domain")

    labels = s.split(".")
    if len(labels) < 2:
        raise DomainValidationError("domain must have at least two labels")

    for label in labels:
        if not label or len(label) > 63:
            raise DomainValidationError("invalid domain label length")
        if not _LABEL_RE.match(label):
            raise DomainValidationError("invalid RFC-1123 label characters")

    tld = labels[-1]
    if len(tld) < 2 or not ("a" <= tld[0] <= "z"):
        raise DomainValidationError("numeric TLD or malformed IPv4 literal is not allowed")

    return s


def sanitize_domains(domains: list[str] | tuple[str, ...]) -> list[str]:
    """Sanitize and deduplicate a list of domains while preserving deterministic order."""
    seen: set[str] = set()
    out: list[str] = []
    for raw in domains:
        try:
            clean = sanitize_domain(raw)
        except DomainValidationError:
            continue
        if clean not in seen:
            seen.add(clean)
            out.append(clean)
    return out


def is_protected_process(name: str) -> bool:
    """Return True if process name matches the critical Windows/POSIX system & host denylist."""
    if not isinstance(name, str):
        return False
    cleaned = name.strip().lower()
    if not cleaned:
        return False
    cleaned = cleaned.replace("\\", "/").rsplit("/", 1)[-1]
    return cleaned in PROTECTED_PROCESS_NAMES


def normalize_process_name(raw: str) -> str:
    """Validate and canonicalize a process name (appending .exe) while enforcing the denylist."""
    if not isinstance(raw, str):
        raise ProcessValidationError("process name must be a string")
    for ch in raw:
        code = ord(ch)
        if code < 0x20 or code == 0x7F:
            raise ProcessValidationError("process name contains control characters")

    trimmed = raw.strip().lower()
    if not trimmed:
        raise ProcessValidationError("process name cannot be empty")
    if any(bad in trimmed for bad in ("/", "\\", ":", "*", "?", '"', "<", ">", "|", " ")) or ".." in trimmed:
        raise ProcessValidationError("invalid process executable name")

    if is_protected_process(trimmed):
        raise ProcessValidationError(f"protected system or host process cannot be blocked: {trimmed}")

    if not trimmed.endswith(".exe"):
        trimmed = f"{trimmed}.exe"

    if is_protected_process(trimmed):
        raise ProcessValidationError(f"protected system or host process cannot be blocked: {trimmed}")

    base = trimmed[:-4]
    if not base:
        raise ProcessValidationError("invalid process executable name")
    return trimmed
