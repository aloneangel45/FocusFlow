package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostsManager_AtomicApplyAndRollback(t *testing.T) {
	tmpDir := t.TempDir()
	hostsPath := filepath.Join(tmpDir, "hosts")
	initialContent := "127.0.0.1 localhost\n::1 localhost\n"

	if err := os.WriteFile(hostsPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to create temp hosts: %v", err)
	}

	manager := NewHostsManagerWithPath(hostsPath)

	initialHash, err := manager.ComputeSHA256(hostsPath)
	if err != nil {
		t.Fatalf("failed to compute initial hash: %v", err)
	}

	// Apply blocklist
	domains := []string{"instagram.com", "tiktok.com"}
	if err := manager.ApplyBlocklist(domains); err != nil {
		t.Fatalf("failed to apply blocklist: %v", err)
	}

	if !manager.IsActive() {
		t.Fatalf("expected manager to report active")
	}

	modifiedData, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("failed to read modified hosts: %v", err)
	}
	modifiedStr := string(modifiedData)

	if !strings.Contains(modifiedStr, "0.0.0.0 instagram.com") {
		t.Errorf("expected 0.0.0.0 instagram.com in hosts")
	}
	if !strings.Contains(modifiedStr, "0.0.0.0 www.instagram.com") {
		t.Errorf("expected 0.0.0.0 www.instagram.com in hosts")
	}
	if !strings.Contains(modifiedStr, "0.0.0.0 tiktok.com") {
		t.Errorf("expected 0.0.0.0 tiktok.com in hosts")
	}

	// Rollback
	if err := manager.Rollback(); err != nil {
		t.Fatalf("failed to rollback: %v", err)
	}

	if manager.IsActive() {
		t.Fatalf("expected manager to report inactive after rollback")
	}

	restoredHash, err := manager.ComputeSHA256(hostsPath)
	if err != nil {
		t.Fatalf("failed to compute restored hash: %v", err)
	}

	if initialHash != restoredHash {
		t.Fatalf("restored hash %s does not match initial %s", restoredHash, initialHash)
	}
}
