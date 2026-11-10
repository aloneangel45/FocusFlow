package engine

import (
	"os/exec"
	"runtime"
)

var defaultDoHBypassDomains = []string{
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
}

type DnsEngine struct{}

func (d *DnsEngine) FlushResolverCache() error {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("ipconfig", "/flushdns")
		return cmd.Run()
	case "darwin":
		cmd := exec.Command("dscacheutil", "-flushcache")
		if err := cmd.Run(); err != nil {
			return err
		}
		_ = exec.Command("killall", "-HUP", "mDNSResponder").Run()
		return nil
	default:
		cmd := exec.Command("resolvectl", "flush-caches")
		if err := cmd.Run(); err != nil {
			_ = exec.Command("systemd-resolve", "--flush-caches").Run()
		}
		return nil
	}
}

func (d *DnsEngine) FlushResolverCacheAsync() {
	go func() {
		_ = d.FlushResolverCache()
	}()
}

func (d *DnsEngine) GetDoHBypassDomains() []string {
	out := make([]string, len(defaultDoHBypassDomains))
	copy(out, defaultDoHBypassDomains)
	return out
}
