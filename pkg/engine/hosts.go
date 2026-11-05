package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultHostsPath = `C:\Windows\System32\drivers\etc\hosts`
	MarkerBegin      = "# --- BEGIN FOCUSFLOW MANAGED BLOCK ---"
	MarkerEnd        = "# --- END FOCUSFLOW MANAGED BLOCK ---"
)

type HostsManager struct {
	hostsPath  string
	backupPath string
}

func NewHostsManager() *HostsManager {
	return NewHostsManagerWithPath(DefaultHostsPath)
}

func NewHostsManagerWithPath(path string) *HostsManager {
	return &HostsManager{
		hostsPath:  path,
		backupPath: path + ".focusflow.bak",
	}
}

func (h *HostsManager) ComputeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (h *HostsManager) Backup() (string, error) {
	if _, err := os.Stat(h.hostsPath); os.IsNotExist(err) {
		return "", fmt.Errorf("hosts file not found at %s", h.hostsPath)
	}

	data, err := os.ReadFile(h.hostsPath)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(h.backupPath, data, 0644); err != nil {
		return "", err
	}

	return h.ComputeSHA256(h.backupPath)
}

func (h *HostsManager) ApplyBlocklist(domains []string) error {
	if len(domains) == 0 {
		return nil
	}

	// Ensure backup exists
	if _, err := os.Stat(h.backupPath); os.IsNotExist(err) {
		if _, err := h.Backup(); err != nil {
			return fmt.Errorf("failed to create hosts backup: %w", err)
		}
	}

	data, err := os.ReadFile(h.hostsPath)
	if err != nil {
		return err
	}

	cleanContent := h.stripManagedBlock(string(data))

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(MarkerBegin)
	b.WriteString("\n# FocusFlow V2 - Distraction-free system guard\n")

	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d != "" && !strings.HasPrefix(d, "#") {
			b.WriteString(fmt.Sprintf("0.0.0.0 %s\n", d))
			if !strings.HasPrefix(d, "www.") {
				b.WriteString(fmt.Sprintf("0.0.0.0 www.%s\n", d))
			}
		}
	}

	b.WriteString(MarkerEnd)
	b.WriteString("\n")

	newContent := strings.TrimRight(cleanContent, "\r\n") + b.String()

	// Atomic write via temp file in same directory
	dir := filepath.Dir(h.hostsPath)
	tmpFile, err := os.CreateTemp(dir, "focusflow-hosts-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(newContent); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}
	tmpFile.Close()

	return os.Rename(tmpPath, h.hostsPath)
}

func (h *HostsManager) Rollback() error {
	if _, err := os.Stat(h.backupPath); err == nil {
		data, err := os.ReadFile(h.backupPath)
		if err == nil {
			if err := os.WriteFile(h.hostsPath, data, 0644); err == nil {
				_ = os.Remove(h.backupPath)
				return nil
			}
		}
	}

	// Fallback: strip managed block directly
	if _, err := os.Stat(h.hostsPath); err == nil {
		data, err := os.ReadFile(h.hostsPath)
		if err != nil {
			return err
		}
		clean := h.stripManagedBlock(string(data))
		return os.WriteFile(h.hostsPath, []byte(clean), 0644)
	}

	return nil
}

func (h *HostsManager) stripManagedBlock(content string) string {
	var result strings.Builder
	insideBlock := false

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if strings.Contains(trimmed, MarkerBegin) {
			insideBlock = true
			continue
		}
		if strings.Contains(trimmed, MarkerEnd) {
			insideBlock = false
			continue
		}
		if !insideBlock {
			result.WriteString(trimmed)
			result.WriteString("\n")
		}
	}

	return result.String()
}

func (h *HostsManager) IsActive() bool {
	data, err := os.ReadFile(h.hostsPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), MarkerBegin)
}
