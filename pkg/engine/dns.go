package engine

import (
	"os/exec"
)

type DnsEngine struct{}

func (d *DnsEngine) FlushResolverCache() error {
	cmd := exec.Command("ipconfig", "/flushdns")
	return cmd.Run()
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
