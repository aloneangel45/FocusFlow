//go:build !windows

package engine

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const (
	maxPosixWriteRetries   = 5
	posixWriteRetryBackoff = 40 * time.Millisecond
)

// writeHostsFileSafe atomically updates hostsPath while preserving existing POSIX permissions
// (defaulting to 0644 rather than CreateTemp's 0600 so non-root DNS resolution never breaks).
func writeHostsFileSafe(hostsPath string, content []byte) error {
	dir := filepath.Dir(hostsPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	targetMode := os.FileMode(0644)
	if info, err := os.Stat(hostsPath); err == nil {
		targetMode = info.Mode().Perm()
		if targetMode == 0 {
			targetMode = 0644
		}
	}

	var lastErr error
	for attempt := 0; attempt < maxPosixWriteRetries; attempt++ {
		tmpFile, err := os.CreateTemp(dir, "focusflow-hosts-*.tmp")
		if err != nil {
			lastErr = err
			if attempt+1 < maxPosixWriteRetries {
				time.Sleep(posixWriteRetryBackoff)
				continue
			}
			return lastErr
		}
		tmpPath := tmpFile.Name()

		if err := tmpFile.Chmod(targetMode); err != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
			return err
		}
		if _, err := tmpFile.Write(content); err != nil {
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
		if err := os.Rename(tmpPath, hostsPath); err == nil {
			return nil
		} else {
			lastErr = err
			_ = os.Remove(tmpPath)
			if attempt+1 < maxPosixWriteRetries {
				time.Sleep(posixWriteRetryBackoff)
			}
		}
	}
	return lastErr
}

func readHostsFileWithRetry(hostsPath string) ([]byte, error) {
	return os.ReadFile(hostsPath)
}

// IsElevated reports whether the current POSIX process runs as root (euid == 0).
func IsElevated() bool {
	return os.Geteuid() == 0
}

// ElevateSelf re-invokes the current binary via sudo on POSIX platforms.
func ElevateSelf(args []string) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	sudoArgs := append([]string{exePath}, args...)
	cmd := exec.Command("sudo", sudoArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// IsPIDAlive checks whether a process with the given PID exists on POSIX platforms.
func IsPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if pid == os.Getpid() {
		return true
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
