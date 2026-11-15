package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/aloneangel45/focusflow/pkg/storage"
)

const (
	DefaultWindowsHostsPath = `C:\Windows\System32\drivers\etc\hosts`
	DefaultPosixHostsPath   = "/etc/hosts"
	DefaultHostsPath        = DefaultWindowsHostsPath

	MarkerBegin       = "# --- BEGIN FOCUSFLOW MANAGED BLOCK ---"
	MarkerEnd         = "# --- END FOCUSFLOW MANAGED BLOCK ---"
	LegacyMarkerBegin = "# --- FOCUSFLOW BLOCK START ---"
	LegacyMarkerEnd   = "# --- FOCUSFLOW BLOCK END ---"

	BackupFilename  = "hosts.focusflow.bak"
	JournalFilename = "hosts.focusflow.journal.json"

	AlivePIDLeaseGrace = 15 * time.Minute
)

type HostsJournal struct {
	OriginalSHA256 string `json:"original_sha256"`
	BackupPath     string `json:"backup_path"`
	HostsPath      string `json:"hosts_path"`
	SessionID      string `json:"session_id"`
	PID            int    `json:"pid"`
	StartedAt      string `json:"started_at"`
	ExpectedEndAt  string `json:"expected_end_at"`
	Profile        string `json:"profile"`
}

type SessionMeta struct {
	SessionID     string
	PID           int
	StartedAt     time.Time
	ExpectedEndAt time.Time
	Profile       string
}

type RecoveryResult struct {
	Recovered        bool   `json:"recovered"`
	UsedFallback     bool   `json:"used_fallback"`
	OrphanSessionID  string `json:"orphan_session_id,omitempty"`
	ActiveOwnerPID   int    `json:"active_owner_pid,omitempty"`
	RestoredChecksum string `json:"restored_checksum,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

type HostsManager struct {
	hostsPath   string
	stateDir    string
	backupPath  string
	journalPath string
}

func systemDefaultHostsPath() string {
	if runtime.GOOS == "windows" {
		return DefaultWindowsHostsPath
	}
	return DefaultPosixHostsPath
}

func GetDefaultHostsPath() string {
	if override := strings.TrimSpace(os.Getenv("FOCUSFLOW_HOSTS_PATH")); override != "" {
		return filepath.Clean(override)
	}
	return systemDefaultHostsPath()
}

func NewHostsManager() *HostsManager {
	return NewHostsManagerWithState(GetDefaultHostsPath(), storage.GetStateDir())
}

func NewHostsManagerWithPath(path string) *HostsManager {
	cleanPath := filepath.Clean(path)
	var stateDir string
	if strings.TrimSpace(os.Getenv("FOCUSFLOW_HOME")) != "" {
		stateDir = storage.GetStateDir()
	} else if cleanPath != filepath.Clean(systemDefaultHostsPath()) {
		// Hermetic test isolation: never write to real %APPDATA% when a custom hosts path is supplied
		stateDir = filepath.Dir(cleanPath)
	} else {
		stateDir = storage.GetStateDir()
	}
	return NewHostsManagerWithState(cleanPath, stateDir)
}

func NewHostsManagerWithState(hostsPath, stateDir string) *HostsManager {
	return &HostsManager{
		hostsPath:   hostsPath,
		stateDir:    stateDir,
		backupPath:  filepath.Join(stateDir, BackupFilename),
		journalPath: filepath.Join(stateDir, JournalFilename),
	}
}

func (h *HostsManager) HostsPath() string   { return h.hostsPath }
func (h *HostsManager) BackupPath() string  { return h.backupPath }
func (h *HostsManager) JournalPath() string { return h.journalPath }

func ComputeBytesSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
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

func HasManagedMarkers(content string) bool {
	return strings.Contains(content, MarkerBegin) ||
		strings.Contains(content, MarkerEnd) ||
		strings.Contains(content, LegacyMarkerBegin) ||
		strings.Contains(content, LegacyMarkerEnd)
}

func detectLineEnding(content string) string {
	if strings.Contains(content, "\r\n") {
		return "\r\n"
	}
	if strings.Contains(content, "\n") {
		return "\n"
	}
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}

func isBeginMarkerLine(trimmed string) bool {
	t := strings.TrimSpace(trimmed)
	return t == MarkerBegin || t == LegacyMarkerBegin ||
		strings.Contains(trimmed, MarkerBegin) || strings.Contains(trimmed, LegacyMarkerBegin)
}

func isEndMarkerLine(trimmed string) bool {
	t := strings.TrimSpace(trimmed)
	return t == MarkerEnd || t == LegacyMarkerEnd ||
		strings.Contains(trimmed, MarkerEnd) || strings.Contains(trimmed, LegacyMarkerEnd)
}

func isLikelyFocusFlowSinkholeLine(trimmed string) bool {
	t := strings.TrimSpace(trimmed)
	if t == "" {
		return true
	}
	if strings.HasPrefix(t, "#") {
		return strings.Contains(strings.ToLower(t), "focusflow")
	}
	fields := strings.Fields(t)
	if len(fields) < 2 || strings.HasPrefix(fields[1], "#") {
		return false
	}
	ip := fields[0]
	return ip == "0.0.0.0" || ip == "::"
}

// StripManagedBlocks removes all current and legacy FocusFlow managed blocks deterministically.
// Safely handles missing/truncated end markers without string-slice corruption, and preserves
// original bytes unchanged when no FocusFlow markers exist.
func StripManagedBlocks(content string) string {
	if !HasManagedMarkers(content) {
		return content
	}

	eol := detectLineEnding(content)
	hadTrailingNewline := strings.HasSuffix(content, "\n")

	rawLines := strings.Split(content, "\n")
	if hadTrailingNewline && len(rawLines) > 0 && rawLines[len(rawLines)-1] == "" {
		rawLines = rawLines[:len(rawLines)-1]
	}

	lines := make([]string, len(rawLines))
	for i, l := range rawLines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}

	kept := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		line := lines[i]
		if isBeginMarkerLine(line) {
			// Pop at most one separator blank line added before MarkerBegin
			if len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
				kept = kept[:len(kept)-1]
			}

			// Look ahead to see if a matching end marker exists before another begin marker or EOF
			endIdx := -1
			for j := i + 1; j < len(lines); j++ {
				if isEndMarkerLine(lines[j]) {
					endIdx = j
					break
				}
				if isBeginMarkerLine(lines[j]) {
					break
				}
			}

			if endIdx >= 0 {
				i = endIdx + 1
				continue
			}

			// Missing end marker: consume FocusFlow comments/sinkhole lines safely without eating custom user entries
			i++
			for i < len(lines) {
				if isBeginMarkerLine(lines[i]) {
					break
				}
				if isEndMarkerLine(lines[i]) {
					i++
					break
				}
				if isLikelyFocusFlowSinkholeLine(lines[i]) {
					i++
					continue
				}
				break
			}
			continue
		}

		if isEndMarkerLine(line) {
			// Stray orphan end marker
			i++
			continue
		}

		kept = append(kept, line)
		i++
	}

	if len(kept) == 0 {
		return ""
	}
	out := strings.Join(kept, eol)
	if hadTrailingNewline {
		out += eol
	}
	return out
}

func (h *HostsManager) stripManagedBlock(content string) string {
	return StripManagedBlocks(content)
}

func writeAtomicStateFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(dir, "ff-state-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	_ = os.Chmod(tmpPath, 0644)
	var renameErr error
	for attempt := 0; attempt < 5; attempt++ {
		renameErr = os.Rename(tmpPath, path)
		if renameErr == nil {
			return nil
		}
		if attempt+1 < 5 {
			time.Sleep(25 * time.Millisecond)
		}
	}
	_ = os.Remove(tmpPath)
	return renameErr
}

// Backup creates a guaranteed-clean baseline backup of the hosts file (stripping any
// existing FocusFlow markers first so a contaminated hosts file is NEVER backed up)
// and returns the clean baseline SHA-256 hex digest.
func (h *HostsManager) Backup() (string, error) {
	if _, err := os.Stat(h.hostsPath); os.IsNotExist(err) {
		return "", fmt.Errorf("hosts file not found at %s", h.hostsPath)
	}

	rawData, err := readHostsFileWithRetry(h.hostsPath)
	if err != nil {
		return "", err
	}

	cleanContent := StripManagedBlocks(string(rawData))
	cleanBytes := []byte(cleanContent)

	if err := writeAtomicStateFile(h.backupPath, cleanBytes); err != nil {
		return "", err
	}

	return ComputeBytesSHA256(cleanBytes), nil
}

func (h *HostsManager) LoadJournal() (*HostsJournal, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		raw, err := os.ReadFile(h.journalPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, err
			}
			lastErr = err
			if attempt+1 < 3 {
				time.Sleep(25 * time.Millisecond)
			}
			continue
		}
		raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
		if len(bytes.TrimSpace(raw)) == 0 {
			lastErr = fmt.Errorf("empty hosts journal")
			if attempt+1 < 3 {
				time.Sleep(25 * time.Millisecond)
			}
			continue
		}
		var j HostsJournal
		if err := json.Unmarshal(raw, &j); err != nil {
			lastErr = err
			if attempt+1 < 3 {
				time.Sleep(25 * time.Millisecond)
			}
			continue
		}
		return &j, nil
	}
	return nil, lastErr
}

func (h *HostsManager) writeJournal(j *HostsJournal) error {
	payload, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return writeAtomicStateFile(h.journalPath, payload)
}

func BuildManagedBlock(sanitizedDomains []string, eol string) string {
	var b strings.Builder
	b.WriteString(MarkerBegin)
	b.WriteString(eol)
	b.WriteString("# FocusFlow v2.1 - Dual-Stack Managed Sinkhole")
	b.WriteString(eol)
	for _, d := range sanitizedDomains {
		b.WriteString("0.0.0.0 ")
		b.WriteString(d)
		b.WriteString(eol)
		if !strings.HasPrefix(d, "www.") {
			b.WriteString("0.0.0.0 www.")
			b.WriteString(d)
			b.WriteString(eol)
		}
		b.WriteString(":: ")
		b.WriteString(d)
		b.WriteString(eol)
		if !strings.HasPrefix(d, "www.") {
			b.WriteString(":: www.")
			b.WriteString(d)
			b.WriteString(eol)
		}
	}
	b.WriteString(MarkerEnd)
	b.WriteString(eol)
	return b.String()
}

func (h *HostsManager) ApplyBlocklist(domains []string) error {
	now := time.Now().UTC()
	return h.ApplyBlocklistWithSession(domains, SessionMeta{
		SessionID:     storage.GenerateSessionID(),
		PID:           os.Getpid(),
		StartedAt:     now,
		ExpectedEndAt: now.Add(2 * time.Hour),
		Profile:       "default",
	})
}

func (h *HostsManager) ApplyBlocklistWithSession(domains []string, meta SessionMeta) error {
	cleanDomains := SanitizeDomains(domains)
	if len(cleanDomains) == 0 {
		return nil
	}

	if _, err := os.Stat(h.hostsPath); os.IsNotExist(err) {
		return fmt.Errorf("hosts file not found at %s", h.hostsPath)
	}

	rawData, err := readHostsFileWithRetry(h.hostsPath)
	if err != nil {
		return err
	}

	cleanBaseline := StripManagedBlocks(string(rawData))
	cleanBytes := []byte(cleanBaseline)
	baselineSHA := ComputeBytesSHA256(cleanBytes)

	// Reuse existing verified clean backup if present; otherwise create a fresh clean backup
	needFreshBackup := true
	if existingJournal, jErr := h.LoadJournal(); jErr == nil && existingJournal.OriginalSHA256 != "" {
		if bakData, bErr := os.ReadFile(h.backupPath); bErr == nil {
			if !HasManagedMarkers(string(bakData)) && ComputeBytesSHA256(bakData) == existingJournal.OriginalSHA256 {
				baselineSHA = existingJournal.OriginalSHA256
				needFreshBackup = false
			}
		}
	}

	if needFreshBackup {
		if err := writeAtomicStateFile(h.backupPath, cleanBytes); err != nil {
			return fmt.Errorf("failed to create clean hosts backup: %w", err)
		}
	}

	now := time.Now().UTC()
	if meta.PID <= 0 {
		meta.PID = os.Getpid()
	}
	if meta.SessionID == "" {
		meta.SessionID = storage.GenerateSessionID()
	}
	if meta.StartedAt.IsZero() {
		meta.StartedAt = now
	}
	if meta.ExpectedEndAt.IsZero() {
		meta.ExpectedEndAt = meta.StartedAt.Add(60 * time.Minute)
	}
	if meta.Profile == "" {
		meta.Profile = "study"
	}

	journal := &HostsJournal{
		OriginalSHA256: baselineSHA,
		BackupPath:     h.backupPath,
		HostsPath:      h.hostsPath,
		SessionID:      meta.SessionID,
		PID:            meta.PID,
		StartedAt:      meta.StartedAt.UTC().Format(time.RFC3339),
		ExpectedEndAt:  meta.ExpectedEndAt.UTC().Format(time.RFC3339),
		Profile:        meta.Profile,
	}

	// Write-Ahead Rollback Journal persisted and synced BEFORE hosts modification
	if err := h.writeJournal(journal); err != nil {
		return fmt.Errorf("failed to persist write-ahead hosts journal: %w", err)
	}

	eol := detectLineEnding(cleanBaseline)
	block := BuildManagedBlock(cleanDomains, eol)

	trimmedBase := strings.TrimRight(cleanBaseline, "\r\n")
	var newContent string
	if trimmedBase == "" {
		newContent = block
	} else {
		newContent = trimmedBase + eol + eol + block
	}

	return writeHostsFileSafe(h.hostsPath, []byte(newContent))
}

func hasIntactManagedSinkhole(content string, cleanDomains []string) bool {
	rawLines := strings.Split(content, "\n")
	beginIdx := -1
	endIdx := -1
	beginCount := 0
	endCount := 0

	for i, raw := range rawLines {
		t := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if t == MarkerBegin {
			beginCount++
			if beginIdx < 0 {
				beginIdx = i
			}
		} else if t == MarkerEnd {
			endCount++
			if endIdx < 0 {
				endIdx = i
			}
		}
	}

	if beginCount != 1 || endCount != 1 || beginIdx < 0 || endIdx <= beginIdx {
		return false
	}

	v4Active := make(map[string]struct{}, len(cleanDomains)*2)
	v6Active := make(map[string]struct{}, len(cleanDomains)*2)

	for _, raw := range rawLines[beginIdx+1 : endIdx] {
		line := strings.TrimSuffix(raw, "\r")
		if hashIdx := strings.IndexByte(line, '#'); hashIdx >= 0 {
			line = line[:hashIdx]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ip := fields[0]
		if ip != "0.0.0.0" && ip != "::" {
			continue
		}
		for _, hostTok := range fields[1:] {
			h := strings.ToLower(strings.TrimSpace(hostTok))
			if h == "" {
				continue
			}
			if ip == "0.0.0.0" {
				v4Active[h] = struct{}{}
			} else {
				v6Active[h] = struct{}{}
			}
		}
	}

	for _, d := range cleanDomains {
		if _, ok := v4Active[d]; !ok {
			return false
		}
		if _, ok := v6Active[d]; !ok {
			return false
		}
		if !strings.HasPrefix(d, "www.") {
			wwwHost := "www." + d
			if _, ok := v4Active[wwwHost]; !ok {
				return false
			}
			if _, ok := v6Active[wwwHost]; !ok {
				return false
			}
		}
	}

	return true
}

// buildPosixElevateSudoArgs constructs sudo invocation arguments that forward FOCUSFLOW_HOME
// (and FOCUSFLOW_HOSTS_PATH if set) across sudo's environment reset so privileged and
// unprivileged processes always share the same state directory.
func buildPosixElevateSudoArgs(exePath string, args []string, stateDir string, hostsOverride string) []string {
	sudoArgs := []string{"env"}
	if trimmedState := strings.TrimSpace(stateDir); trimmedState != "" {
		sudoArgs = append(sudoArgs, "FOCUSFLOW_HOME="+trimmedState)
	}
	if trimmedHosts := strings.TrimSpace(hostsOverride); trimmedHosts != "" {
		sudoArgs = append(sudoArgs, "FOCUSFLOW_HOSTS_PATH="+trimmedHosts)
	}
	sudoArgs = append(sudoArgs, exePath)
	sudoArgs = append(sudoArgs, args...)
	return sudoArgs
}

// VerifyOrReapply checks mid-session integrity of the managed block in hosts and re-applies
// the sinkhole if any tampering or external overwrite occurred.
func (h *HostsManager) VerifyOrReapply(domains []string, meta SessionMeta) (bool, error) {
	cleanDomains := SanitizeDomains(domains)
	if len(cleanDomains) == 0 {
		return false, nil
	}

	rawData, err := readHostsFileWithRetry(h.hostsPath)
	if err != nil {
		return false, err
	}
	if !hasIntactManagedSinkhole(string(rawData), cleanDomains) {
		return true, h.ApplyBlocklistWithSession(cleanDomains, meta)
	}
	return false, nil
}

// Rollback restores the clean hosts file using SHA-256 verified backup or deterministic
// marker stripping fallback, then cleans up journal and backup artifacts.
func (h *HostsManager) Rollback() error {
	_, err := h.restoreHostsFromBackupOrStrip()
	if err != nil {
		return err
	}
	h.cleanupStateArtifacts()
	return nil
}

func (h *HostsManager) restoreHostsFromBackupOrStrip() (usedFallback bool, err error) {
	journal, jErr := h.LoadJournal()
	backupPath := h.backupPath
	if jErr == nil && journal.BackupPath != "" {
		backupPath = journal.BackupPath
	}

	if bakData, bErr := os.ReadFile(backupPath); bErr == nil {
		bakClean := !HasManagedMarkers(string(bakData))
		hashMatches := true
		if jErr == nil && journal.OriginalSHA256 != "" {
			hashMatches = ComputeBytesSHA256(bakData) == journal.OriginalSHA256
		}
		if bakClean && hashMatches {
			if wErr := writeHostsFileSafe(h.hostsPath, bakData); wErr == nil {
				return false, nil
			}
		}
	}

	// Fallback: deterministic marker stripping directly on hostsPath
	if _, statErr := os.Stat(h.hostsPath); statErr == nil {
		rawData, rErr := readHostsFileWithRetry(h.hostsPath)
		if rErr != nil {
			return true, rErr
		}
		clean := StripManagedBlocks(string(rawData))
		if wErr := writeHostsFileSafe(h.hostsPath, []byte(clean)); wErr != nil {
			return true, wErr
		}
		return true, nil
	}

	return false, nil
}

func (h *HostsManager) cleanupStateArtifacts() {
	_ = os.Remove(h.journalPath)
	_ = os.Remove(h.backupPath)
	// Also clean up any legacy adjacent backup file from v2.0
	_ = os.Remove(h.hostsPath + ".focusflow.bak")
}

func isJournalExpired(expectedEndAt string, now time.Time) bool {
	return isJournalExpiredWithGrace(expectedEndAt, now, 0)
}

func isJournalExpiredWithGrace(expectedEndAt string, now time.Time, grace time.Duration) bool {
	expectedEndAt = strings.TrimSpace(expectedEndAt)
	if expectedEndAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, expectedEndAt)
	if err != nil {
		return false
	}
	return now.After(t.Add(grace))
}

// RecoverIfOrphaned inspects the write-ahead journal and hosts file on startup.
// If an orphaned crash/reboot state or expired lock is found, it verifies .bak SHA-256
// (falling back to deterministic marker stripping if altered/missing), restores hosts,
// flushes DNS asynchronously, and invokes onOrphanSession so sessions.json marks completed: false.
func (h *HostsManager) RecoverIfOrphaned(onOrphanSession func(sessionID string)) (*RecoveryResult, error) {
	return h.RecoverIfOrphanedAt(time.Now().UTC(), onOrphanSession)
}

func (h *HostsManager) RecoverIfOrphanedAt(now time.Time, onOrphanSession func(sessionID string)) (*RecoveryResult, error) {
	journal, jErr := h.LoadJournal()
	hasJournal := jErr == nil && journal != nil
	hasCorruptJournal := false
	if jErr != nil && !os.IsNotExist(jErr) {
		hasCorruptJournal = true
	}

	hostsHasMarkers := false
	if rawData, err := readHostsFileWithRetry(h.hostsPath); err == nil {
		hostsHasMarkers = HasManagedMarkers(string(rawData))
	}

	if !hasJournal && !hasCorruptJournal && !hostsHasMarkers {
		return &RecoveryResult{Recovered: false, Reason: "clean"}, nil
	}

	reason := "stray_markers_without_journal"
	orphanSessionID := ""

	if hasJournal {
		orphanSessionID = journal.SessionID
		pidAlive := journal.PID > 0 && IsPIDAlive(journal.PID)
		if pidAlive {
			if !isJournalExpiredWithGrace(journal.ExpectedEndAt, now, AlivePIDLeaseGrace) {
				return &RecoveryResult{
					Recovered:      false,
					ActiveOwnerPID: journal.PID,
					Reason:         "active_session_alive",
				}, nil
			}
			reason = "session_lease_expired"
		} else {
			reason = fmt.Sprintf("owner_pid_%d_dead", journal.PID)
		}
	} else if hasCorruptJournal {
		reason = "corrupt_journal"
	}

	usedFallback, err := h.restoreHostsFromBackupOrStrip()
	if err != nil {
		return &RecoveryResult{
			Recovered:       false,
			UsedFallback:    usedFallback,
			OrphanSessionID: orphanSessionID,
			Reason:          reason,
		}, err
	}

	h.cleanupStateArtifacts()

	if onOrphanSession != nil {
		onOrphanSession(orphanSessionID)
	}

	dns := &DnsEngine{}
	dns.FlushResolverCacheAsync()

	restoredSum, _ := h.ComputeSHA256(h.hostsPath)
	return &RecoveryResult{
		Recovered:        true,
		UsedFallback:     usedFallback,
		OrphanSessionID:  orphanSessionID,
		RestoredChecksum: restoredSum,
		Reason:           reason,
	}, nil
}

func (h *HostsManager) IsActive() bool {
	data, err := readHostsFileWithRetry(h.hostsPath)
	if err != nil {
		return false
	}
	s := string(data)
	return strings.Contains(s, MarkerBegin) || strings.Contains(s, LegacyMarkerBegin)
}
