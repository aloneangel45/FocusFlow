package engine

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

var (
	ErrEmptyDomain     = errors.New("domain cannot be empty")
	ErrControlChars    = errors.New("domain contains illegal control or whitespace characters")
	ErrIPLiteral       = errors.New("IP literals cannot be added to hosts blocklist")
	ErrReservedDomain  = errors.New("reserved or local domain cannot be blocked")
	ErrInvalidRFC1123  = errors.New("invalid RFC-1123 domain format")
	ErrEmptyProcess    = errors.New("process name cannot be empty")
	ErrInvalidProcess  = errors.New("invalid process executable name")
	ErrProtectedBinary = errors.New("protected system or host process cannot be blocked")
)

var reservedExactDomains = map[string]struct{}{
	"localhost":             {},
	"localhost.localdomain": {},
	"localdomain":           {},
	"local":                 {},
	"broadcasthost":         {},
	"ip6-localhost":         {},
	"ip6-loopback":          {},
	"ip6-localnet":          {},
	"ip6-mcastprefix":       {},
	"ip6-allnodes":          {},
	"ip6-allrouters":        {},
	"ip6-allhosts":          {},
	"home.arpa":             {},
}

func isReservedDomain(host string) bool {
	if _, reserved := reservedExactDomains[host]; reserved {
		return true
	}
	return strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".localdomain") ||
		strings.HasSuffix(host, ".home.arpa") ||
		strings.HasSuffix(host, ".invalid")
}

var protectedBaseNames = []string{
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
}

var protectedProcessMap = func() map[string]struct{} {
	m := make(map[string]struct{}, len(protectedBaseNames)*2)
	for _, base := range protectedBaseNames {
		lower := strings.ToLower(base)
		m[lower] = struct{}{}
		m[lower+".exe"] = struct{}{}
	}
	return m
}()

// ProtectedProcessNames returns the canonical protected Windows/host executable names.
func ProtectedProcessNames() []string {
	out := make([]string, 0, len(protectedBaseNames))
	for _, b := range protectedBaseNames {
		out = append(out, b+".exe")
	}
	return out
}

// IsProtectedProcess reports whether a process name belongs to the critical system/host denylist.
func IsProtectedProcess(name string) bool {
	cleaned := strings.ToLower(strings.TrimSpace(name))
	if cleaned == "" {
		return false
	}
	// Extract basename if any path separators exist
	if idx := strings.LastIndexAny(cleaned, `/\`); idx >= 0 {
		cleaned = cleaned[idx+1:]
	}
	_, exists := protectedProcessMap[cleaned]
	return exists
}

// NormalizeProcessName validates and normalizes an executable name for custom blocklists.
func NormalizeProcessName(raw string) (string, error) {
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c < 0x20 || c == 0x7f {
			return "", ErrInvalidProcess
		}
	}
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "", ErrEmptyProcess
	}
	if strings.ContainsAny(trimmed, `/\:*?"<>| `) || strings.Contains(trimmed, "..") {
		return "", ErrInvalidProcess
	}
	if IsProtectedProcess(trimmed) {
		return "", fmt.Errorf("%w: %s", ErrProtectedBinary, trimmed)
	}
	if !strings.HasSuffix(trimmed, ".exe") {
		trimmed += ".exe"
	}
	if IsProtectedProcess(trimmed) {
		return "", fmt.Errorf("%w: %s", ErrProtectedBinary, trimmed)
	}
	base := strings.TrimSuffix(trimmed, ".exe")
	if base == "" {
		return "", ErrInvalidProcess
	}
	return trimmed, nil
}

// SanitizeDomain strictly normalizes and validates a domain name per RFC-1123.
// Strips http(s)://, paths, query strings, fragments, valid ports, outer spaces, and leading www.
// Rejects newlines, control chars, embedded whitespace, IPv4/IPv6 literals, localhost, localdomain, and *.local.
func SanitizeDomain(raw string) (string, error) {
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c < 0x20 || c == 0x7f {
			return "", ErrControlChars
		}
	}

	s := strings.Trim(raw, " ")
	if s == "" || strings.HasPrefix(s, "#") {
		return "", ErrEmptyDomain
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return "", ErrControlChars
	}

	s = strings.ToLower(s)

	if strings.HasPrefix(s, "https://") {
		s = strings.TrimPrefix(s, "https://")
	} else if strings.HasPrefix(s, "http://") {
		s = strings.TrimPrefix(s, "http://")
	} else if strings.Contains(s, "://") || strings.HasPrefix(s, "//") {
		return "", ErrInvalidRFC1123
	}

	// Strip path, query, fragment
	if idx := strings.IndexAny(s, "/?#"); idx >= 0 {
		s = s[:idx]
	}
	if s == "" {
		return "", ErrEmptyDomain
	}

	// Reject userinfo or bracketed IPv6 literals
	if strings.Contains(s, "@") || strings.ContainsAny(s, "[]") {
		if strings.ContainsAny(s, "[]") {
			return "", ErrIPLiteral
		}
		return "", ErrInvalidRFC1123
	}

	// Handle port vs IPv6 colons
	if colonCount := strings.Count(s, ":"); colonCount > 1 {
		return "", ErrIPLiteral
	} else if colonCount == 1 {
		idx := strings.IndexByte(s, ':')
		hostPart := s[:idx]
		portPart := s[idx+1:]
		if portPart == "" {
			return "", ErrInvalidRFC1123
		}
		for i := 0; i < len(portPart); i++ {
			if portPart[i] < '0' || portPart[i] > '9' {
				return "", ErrInvalidRFC1123
			}
		}
		portNum, err := strconv.Atoi(portPart)
		if err != nil || portNum < 1 || portNum > 65535 {
			return "", ErrInvalidRFC1123
		}
		s = hostPart
	}

	// Reject multiple trailing dots before stripping single FQDN trailing dot
	if strings.HasSuffix(s, "..") {
		return "", ErrInvalidRFC1123
	}
	s = strings.TrimSuffix(s, ".")
	if s == "" {
		return "", ErrEmptyDomain
	}

	// Reject IPv4/IPv6 literals and reserved domains before and after stripping www.
	if net.ParseIP(s) != nil {
		return "", ErrIPLiteral
	}
	if isReservedDomain(s) {
		return "", ErrReservedDomain
	}

	if strings.HasPrefix(s, "www.") {
		rest := strings.TrimPrefix(s, "www.")
		if net.ParseIP(rest) != nil {
			return "", ErrIPLiteral
		}
		if isReservedDomain(rest) {
			return "", ErrReservedDomain
		}
		if strings.Contains(rest, ".") {
			s = rest
		}
	}

	if isReservedDomain(s) {
		return "", ErrReservedDomain
	}
	if net.ParseIP(s) != nil {
		return "", ErrIPLiteral
	}

	if len(s) > 253 || !strings.Contains(s, ".") {
		return "", ErrInvalidRFC1123
	}

	labels := strings.Split(s, ".")
	if len(labels) < 2 {
		return "", ErrInvalidRFC1123
	}

	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 {
			return "", ErrInvalidRFC1123
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return "", ErrInvalidRFC1123
		}
		for i := 0; i < len(label); i++ {
			ch := label[i]
			isLower := ch >= 'a' && ch <= 'z'
			isDigit := ch >= '0' && ch <= '9'
			if !isLower && !isDigit && ch != '-' {
				return "", ErrInvalidRFC1123
			}
		}
	}

	// RFC-1123 / IANA DNS rule: top-level label cannot start with a digit or be all digits
	// (rejects malformed numeric IPs like 999.999.999.999 as well as hex/octal IPs like 0x7f.0.0.0x1).
	tld := labels[len(labels)-1]
	if tld[0] >= '0' && tld[0] <= '9' {
		return "", ErrIPLiteral
	}
	if len(tld) < 2 {
		return "", ErrInvalidRFC1123
	}

	return s, nil
}

// SanitizeDomains filters, canonicalizes, and deduplicates a list of domains while preserving order.
func SanitizeDomains(domains []string) []string {
	seen := make(map[string]struct{}, len(domains))
	out := make([]string, 0, len(domains))
	for _, raw := range domains {
		clean, err := SanitizeDomain(raw)
		if err != nil {
			continue
		}
		if _, exists := seen[clean]; !exists {
			seen[clean] = struct{}{}
			out = append(out, clean)
		}
	}
	return out
}
