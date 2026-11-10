package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestHostsManager_AtomicApplyDualStackAndRollback(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FOCUSFLOW_HOME", tmpDir)

	hostsPath := filepath.Join(tmpDir, "hosts")
	initialContent := "127.0.0.1 localhost\n::1 localhost\n"
	if err := os.WriteFile(hostsPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to create temp hosts: %v", err)
	}

	manager := NewHostsManagerWithState(hostsPath, tmpDir)
	initialHash, err := manager.ComputeSHA256(hostsPath)
	if err != nil {
		t.Fatalf("failed to compute initial hash: %v", err)
	}

	now := time.Now().UTC()
	meta := SessionMeta{
		SessionID:     "sess-test-001",
		PID:           os.Getpid(),
		StartedAt:     now,
		ExpectedEndAt: now.Add(25 * time.Minute),
		Profile:       "study",
	}

	domains := []string{"https://www.Instagram.com/reels", "tiktok.com:443"}
	if err := manager.ApplyBlocklistWithSession(domains, meta); err != nil {
		t.Fatalf("failed to apply blocklist: %v", err)
	}

	if !manager.IsActive() {
		t.Fatalf("expected manager to report active")
	}

	// Verify write-ahead journal fields
	journal, err := manager.LoadJournal()
	if err != nil {
		t.Fatalf("expected journal to exist: %v", err)
	}
	if journal.OriginalSHA256 != initialHash {
		t.Errorf("journal original_sha256 = %s, want %s", journal.OriginalSHA256, initialHash)
	}
	if journal.SessionID != "sess-test-001" || journal.Profile != "study" || journal.PID != os.Getpid() {
		t.Errorf("unexpected journal metadata: %+v", journal)
	}

	modifiedData, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("failed to read modified hosts: %v", err)
	}
	modifiedStr := string(modifiedData)

	expectedEntries := []string{
		MarkerBegin,
		"0.0.0.0 instagram.com",
		"0.0.0.0 www.instagram.com",
		":: instagram.com",
		":: www.instagram.com",
		"0.0.0.0 tiktok.com",
		"0.0.0.0 www.tiktok.com",
		":: tiktok.com",
		":: www.tiktok.com",
		MarkerEnd,
	}
	for _, entry := range expectedEntries {
		if !strings.Contains(modifiedStr, entry) {
			t.Errorf("expected %q in modified hosts, got:\n%s", entry, modifiedStr)
		}
	}

	if err := manager.Rollback(); err != nil {
		t.Fatalf("failed to rollback: %v", err)
	}

	if manager.IsActive() {
		t.Fatalf("expected manager inactive after rollback")
	}
	if _, err := os.Stat(manager.JournalPath()); !os.IsNotExist(err) {
		t.Errorf("expected journal removed after rollback")
	}

	restoredHash, err := manager.ComputeSHA256(hostsPath)
	if err != nil {
		t.Fatalf("failed to compute restored hash: %v", err)
	}
	if initialHash != restoredHash {
		t.Fatalf("restored hash %s != initial %s", restoredHash, initialHash)
	}
}

func TestHostsManager_NeverBackupContaminatedHosts(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FOCUSFLOW_HOME", tmpDir)

	hostsPath := filepath.Join(tmpDir, "hosts")
	cleanBase := "127.0.0.1 localhost\r\n192.168.1.20 devbox.lan\r\n"
	contaminated := cleanBase + "\r\n" +
		LegacyMarkerBegin + "\r\n127.0.0.1 youtube.com\r\n" + LegacyMarkerEnd + "\r\n\r\n" +
		MarkerBegin + "\r\n0.0.0.0 reddit.com\r\n" + MarkerEnd + "\r\n"

	if err := os.WriteFile(hostsPath, []byte(contaminated), 0644); err != nil {
		t.Fatalf("failed to write contaminated hosts: %v", err)
	}

	manager := NewHostsManagerWithState(hostsPath, tmpDir)
	if err := manager.ApplyBlocklist([]string{"x.com"}); err != nil {
		t.Fatalf("ApplyBlocklist failed: %v", err)
	}

	bakBytes, err := os.ReadFile(manager.BackupPath())
	if err != nil {
		t.Fatalf("failed to read backup: %v", err)
	}
	if HasManagedMarkers(string(bakBytes)) {
		t.Fatalf("backup file was contaminated with managed markers:\n%s", string(bakBytes))
	}
	if string(bakBytes) != cleanBase {
		t.Fatalf("backup %q != cleanBase %q", string(bakBytes), cleanBase)
	}

	if err := manager.Rollback(); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	restored, _ := os.ReadFile(hostsPath)
	if string(restored) != cleanBase {
		t.Fatalf("restored %q != cleanBase %q", string(restored), cleanBase)
	}
}

func TestStripManagedBlocks_MissingEndMarkerNoCorruption(t *testing.T) {
	// Missing end marker on current block at EOF
	truncatedCurrent := "127.0.0.1 localhost\n::1 localhost\n\n" +
		MarkerBegin + "\n# FocusFlow v2.1 - Dual-Stack Managed Sinkhole\n0.0.0.0 youtube.com\n:: youtube.com\n"
	got := StripManagedBlocks(truncatedCurrent)
	want := "127.0.0.1 localhost\n::1 localhost\n"
	if got != want {
		t.Fatalf("truncated current strip got %q, want %q", got, want)
	}

	// Missing end marker on legacy block followed by custom user entry
	truncatedLegacyWithUserLine := "127.0.0.1 localhost\r\n\r\n" +
		LegacyMarkerBegin + "\r\n127.0.0.1 instagram.com\r\n10.20.30.40 internal.corp\r\n"
	got2 := StripManagedBlocks(truncatedLegacyWithUserLine)
	want2 := "127.0.0.1 localhost\r\n10.20.30.40 internal.corp\r\n"
	if got2 != want2 {
		t.Fatalf("truncated legacy strip got %q, want %q", got2, want2)
	}
}

func TestHostsManager_RecoverIfOrphaned_DeadPIDAndFallback(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FOCUSFLOW_HOME", tmpDir)

	hostsPath := filepath.Join(tmpDir, "hosts")
	cleanContent := "127.0.0.1 localhost\n::1 localhost\n"
	if err := os.WriteFile(hostsPath, []byte(cleanContent), 0644); err != nil {
		t.Fatalf("write hosts: %v", err)
	}

	manager := NewHostsManagerWithState(hostsPath, tmpDir)
	now := time.Now().UTC()

	// Case 1: Live owning process (current PID) with future lease must NOT be clobbered
	if err := manager.ApplyBlocklistWithSession([]string{"reddit.com"}, SessionMeta{
		SessionID:     "live-sess",
		PID:           os.Getpid(),
		StartedAt:     now,
		ExpectedEndAt: now.Add(30 * time.Minute),
		Profile:       "dev",
	}); err != nil {
		t.Fatalf("apply live: %v", err)
	}
	resLive, err := manager.RecoverIfOrphanedAt(now, nil)
	if err != nil || resLive.Recovered {
		t.Fatalf("expected live session not recovered, got res=%+v err=%v", resLive, err)
	}

	// Case 2: Dead PID + tampered/missing backup falls back to deterministic marker stripping
	deadMeta := SessionMeta{
		SessionID:     "dead-sess-42",
		PID:           99999999,
		StartedAt:     now.Add(-10 * time.Minute),
		ExpectedEndAt: now.Add(15 * time.Minute),
		Profile:       "hardcore",
	}
	if err := manager.ApplyBlocklistWithSession([]string{"reddit.com", "twitch.tv"}, deadMeta); err != nil {
		t.Fatalf("apply dead: %v", err)
	}
	// Tamper backup to force fallback path
	if err := os.WriteFile(manager.BackupPath(), []byte("tampered-corrupt-backup"), 0644); err != nil {
		t.Fatalf("tamper bak: %v", err)
	}

	var markedOrphanID string
	resDead, err := manager.RecoverIfOrphanedAt(now, func(sessionID string) {
		markedOrphanID = sessionID
	})
	if err != nil {
		t.Fatalf("RecoverIfOrphanedAt failed: %v", err)
	}
	if !resDead.Recovered || !resDead.UsedFallback {
		t.Fatalf("expected Recovered=true and UsedFallback=true, got %+v", resDead)
	}
	if markedOrphanID != "dead-sess-42" {
		t.Errorf("expected orphan callback with 'dead-sess-42', got %q", markedOrphanID)
	}
	restored, _ := os.ReadFile(hostsPath)
	if string(restored) != cleanContent {
		t.Errorf("restored hosts %q != cleanContent %q", string(restored), cleanContent)
	}
}

func TestHostsManager_VerifyOrReapply_MidSessionTamper(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FOCUSFLOW_HOME", tmpDir)

	hostsPath := filepath.Join(tmpDir, "hosts")
	cleanContent := "127.0.0.1 localhost\n"
	_ = os.WriteFile(hostsPath, []byte(cleanContent), 0644)

	manager := NewHostsManagerWithState(hostsPath, tmpDir)
	meta := SessionMeta{
		SessionID:     "watchdog-1",
		PID:           os.Getpid(),
		StartedAt:     time.Now().UTC(),
		ExpectedEndAt: time.Now().UTC().Add(25 * time.Minute),
		Profile:       "hardcore",
	}
	domains := []string{"youtube.com", "tiktok.com"}
	if err := manager.ApplyBlocklistWithSession(domains, meta); err != nil {
		t.Fatalf("initial apply: %v", err)
	}

	tampered, err := manager.VerifyOrReapply(domains, meta)
	if err != nil || tampered {
		t.Fatalf("expected untampered=false, got tampered=%v err=%v", tampered, err)
	}

	// Simulate user deleting the managed block mid-sprint
	_ = os.WriteFile(hostsPath, []byte(cleanContent), 0644)
	tampered, err = manager.VerifyOrReapply(domains, meta)
	if err != nil || !tampered {
		t.Fatalf("expected tamper detected and re-applied, got tampered=%v err=%v", tampered, err)
	}
	if !manager.IsActive() {
		t.Fatalf("expected blocklist active after VerifyOrReapply")
	}

	// Ensure clean rollback still restores original clean hosts
	if err := manager.Rollback(); err != nil {
		t.Fatalf("rollback after tamper reapply failed: %v", err)
	}
	finalBytes, _ := os.ReadFile(hostsPath)
	if string(finalBytes) != cleanContent {
		t.Fatalf("final hosts %q != %q", string(finalBytes), cleanContent)
	}
}

func TestHostsManager_PermissionsAndACLPreservation(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("FOCUSFLOW_HOME", tmpDir)
	hostsPath := filepath.Join(tmpDir, "hosts")
	if err := os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\r\n"), 0644); err != nil {
		t.Fatalf("write hosts: %v", err)
	}

	if runtime.GOOS == "windows" {
		// Grant explicit Users:(R) ACE and verify ReplaceFileW preserves it across Apply & Rollback
		_ = exec.Command("icacls", hostsPath, "/grant", "*S-1-5-32-545:(R)").Run()
		beforeOut, err := exec.Command("icacls", hostsPath).CombinedOutput()
		if err != nil {
			t.Skipf("icacls unavailable: %v", err)
		}

		manager := NewHostsManagerWithState(hostsPath, tmpDir)
		if err := manager.ApplyBlocklist([]string{"example.com"}); err != nil {
			t.Fatalf("apply: %v", err)
		}
		afterOut, err := exec.Command("icacls", hostsPath).CombinedOutput()
		if err != nil {
			t.Fatalf("icacls after: %v", err)
		}
		if !strings.Contains(string(beforeOut), "(R)") || !strings.Contains(string(afterOut), "(R)") {
			t.Logf("icacls before:\n%s\nafter:\n%s", string(beforeOut), string(afterOut))
		}
		if string(beforeOut) != string(afterOut) {
			t.Errorf("Windows DACL changed across atomic hosts update!\nbefore:\n%s\nafter:\n%s", string(beforeOut), string(afterOut))
		}
	} else {
		if err := os.Chmod(hostsPath, 0644); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		manager := NewHostsManagerWithState(hostsPath, tmpDir)
		if err := manager.ApplyBlocklist([]string{"example.com"}); err != nil {
			t.Fatalf("apply: %v", err)
		}
		info, err := os.Stat(hostsPath)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if info.Mode().Perm() != 0644 {
			t.Errorf("expected POSIX perm 0644 preserved, got %o", info.Mode().Perm())
		}
	}
}

func TestDnsEngine_ExpandedDoHBypassDomains(t *testing.T) {
	dns := &DnsEngine{}
	got := dns.GetDoHBypassDomains()
	required := []string{
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
	set := make(map[string]bool, len(got))
	for _, d := range got {
		set[d] = true
	}
	for _, req := range required {
		if !set[req] {
			t.Errorf("missing required DoH domain %q", req)
		}
	}
}
