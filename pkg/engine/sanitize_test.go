package engine

import "testing"

func TestSanitizeDomain_ValidInputs(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"youtube.com", "youtube.com"},
		{"  HTTPS://www.YouTube.com:443/watch?v=dQw4w9WgXcQ#top  ", "youtube.com"},
		{"http://Sub.Reddit.com:8080/r/golang/", "sub.reddit.com"},
		{"x.com.", "x.com"},
		{"https://www.reddit.com.:8443/r/golang", "reddit.com"},
		{"dns.adguard-dns.com", "dns.adguard-dns.com"},
		{"one.one.one.one", "one.one.one.one"},
	}

	for _, tc := range cases {
		got, err := SanitizeDomain(tc.raw)
		if err != nil {
			t.Errorf("SanitizeDomain(%q) unexpected error: %v", tc.raw, err)
			continue
		}
		if got != tc.want {
			t.Errorf("SanitizeDomain(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestSanitizeDomain_RejectsMaliciousAndReservedInputs(t *testing.T) {
	badInputs := []string{
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
		"sub.localhost",
		"dev.localdomain",
		"singlelabel",
		"-badlabel.com",
		"badlabel-.com",
		"bad..dots.com",
		"example.com:99999",
		"example.com:0",
		"example.com:notaport",
		"example.com:\u00b2",
		"ftp://example.com",
		"//reddit.com/r/golang",
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
	}

	for _, raw := range badInputs {
		if got, err := SanitizeDomain(raw); err == nil {
			t.Errorf("SanitizeDomain(%q) succeeded with %q; expected rejection", raw, got)
		}
	}
}

func TestProtectedProcesses_DenylistEnforcement(t *testing.T) {
	protected := []string{
		"csrss.exe", "smss.exe", "lsass.exe", "services.exe", "svchost.exe",
		"wininit.exe", "winlogon.exe", "dwm.exe", "explorer.exe", "sihost.exe",
		"fontdrvhost.exe", "focusflow.exe", "python.exe", "pythonw.exe",
		"conhost.exe", "wt.exe", "EXPLORER", "PythonW", `C:\Windows\System32\lsass.exe`,
	}

	for _, name := range protected {
		if !IsProtectedProcess(name) {
			t.Errorf("expected IsProtectedProcess(%q) == true", name)
		}
		if got, err := NormalizeProcessName(name); err == nil {
			t.Errorf("expected NormalizeProcessName(%q) to fail, got %q", name, got)
		}
	}

	valid, err := NormalizeProcessName("  Discord  ")
	if err != nil || valid != "discord.exe" {
		t.Fatalf("expected NormalizeProcessName('Discord') == 'discord.exe', got %q (err=%v)", valid, err)
	}
}
