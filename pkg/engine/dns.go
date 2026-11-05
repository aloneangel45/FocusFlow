package engine

import (
	"os/exec"
	"runtime"
)

type DnsEngine struct{}

func (d *DnsEngine) FlushResolverCache() error {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("ipconfig", "/flushdns")
		return cmd.Run()
	case "darwin":
		cmd := exec.Command("dscacheutil", "-flushcache")
		return cmd.Run()
	default:
		cmd := exec.Command("resolvectl", "flush-caches")
		if err := cmd.Run(); err != nil {
			_ = exec.Command("systemd-resolve", "--flush-caches").Run()
		}
		return nil
	}
}

func (d *DnsEngine) GetDoHBypassDomains() []string {
	return []string{
		"cloudflare-dns.com",
		"one.one.one.one",
		"dns.google",
		"dns.google.com",
		"dns.quad9.net",
		"doh.opendns.com",
		"chrome.cloudflare-dns.com",
		"mozilla.cloudflare-dns.com",
	}
}
