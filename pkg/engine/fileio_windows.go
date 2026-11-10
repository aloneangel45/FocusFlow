//go:build windows

package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	replaceFileIgnoreMergeErrors    = 0x00000002
	processQueryLimitedInformation  = 0x00001000
	win32StillActive                = 259
	win32ErrorSharingViolation      = syscall.Errno(32)
	win32ErrorLockViolation         = syscall.Errno(33)
	maxWindowsWriteRetries          = 5
	windowsWriteRetryBackoff        = 40 * time.Millisecond
)

var (
	modKernel32        = syscall.NewLazyDLL("kernel32.dll")
	modShell32         = syscall.NewLazyDLL("shell32.dll")
	procReplaceFileW   = modKernel32.NewProc("ReplaceFileW")
	procIsUserAnAdmin  = modShell32.NewProc("IsUserAnAdmin")
	procShellExecuteW  = modShell32.NewProc("ShellExecuteW")
)

func isTransientSharingError(err error) bool {
	if err == nil {
		return false
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == win32ErrorSharingViolation || errno == win32ErrorLockViolation
	}
	return false
}

// writeHostsFileSafe writes content to hostsPath while preserving existing Windows DACLs
// (ensuring NetworkService / Dnscache and Users retain read access), syncing to disk,
// and retrying transient sharing locks (5x with 40ms backoff).
func writeHostsFileSafe(hostsPath string, content []byte) error {
	dir := filepath.Dir(hostsPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, "focusflow-hosts-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

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

	// If target file does not exist yet, atomic rename with inherited parent permissions
	if _, statErr := os.Stat(hostsPath); os.IsNotExist(statErr) {
		var lastErr error
		for attempt := 0; attempt < maxWindowsWriteRetries; attempt++ {
			if err := os.Rename(tmpPath, hostsPath); err == nil {
				return nil
			} else {
				lastErr = err
				if attempt+1 < maxWindowsWriteRetries {
					time.Sleep(windowsWriteRetryBackoff)
				}
			}
		}
		_ = os.Remove(tmpPath)
		return lastErr
	}

	// Target exists: use Win32 ReplaceFileW to atomically swap data while preserving target DACLs
	targetPtr, err := syscall.UTF16PtrFromString(hostsPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	tempPtr, err := syscall.UTF16PtrFromString(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	var lastReplaceErr error
	for attempt := 0; attempt < maxWindowsWriteRetries; attempt++ {
		r1, _, callErr := procReplaceFileW.Call(
			uintptr(unsafe.Pointer(targetPtr)),
			uintptr(unsafe.Pointer(tempPtr)),
			0,
			uintptr(replaceFileIgnoreMergeErrors),
			0,
			0,
		)
		if r1 != 0 {
			return nil
		}
		lastReplaceErr = callErr
		if attempt+1 < maxWindowsWriteRetries {
			time.Sleep(windowsWriteRetryBackoff)
		}
	}

	// Clean up temp file before in-place DACL-preserving fallback
	_ = os.Remove(tmpPath)

	var lastFallbackErr error
	for attempt := 0; attempt < maxWindowsWriteRetries; attempt++ {
		f, openErr := os.OpenFile(hostsPath, os.O_WRONLY|os.O_TRUNC, 0644)
		if openErr != nil {
			lastFallbackErr = openErr
			if isTransientSharingError(openErr) && attempt+1 < maxWindowsWriteRetries {
				time.Sleep(windowsWriteRetryBackoff)
				continue
			}
			break
		}
		_, writeErr := f.Write(content)
		syncErr := f.Sync()
		closeErr := f.Close()
		if writeErr == nil && syncErr == nil && closeErr == nil {
			return nil
		}
		if writeErr != nil {
			lastFallbackErr = writeErr
		} else if syncErr != nil {
			lastFallbackErr = syncErr
		} else {
			lastFallbackErr = closeErr
		}
		if attempt+1 < maxWindowsWriteRetries {
			time.Sleep(windowsWriteRetryBackoff)
		}
	}

	if lastFallbackErr != nil {
		return lastFallbackErr
	}
	return lastReplaceErr
}

func readHostsFileWithRetry(hostsPath string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < maxWindowsWriteRetries; attempt++ {
		data, err := os.ReadFile(hostsPath)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if isTransientSharingError(err) && attempt+1 < maxWindowsWriteRetries {
			time.Sleep(windowsWriteRetryBackoff)
			continue
		}
		break
	}
	return nil, lastErr
}

// IsElevated reports whether the current Windows process holds Administrator privileges.
func IsElevated() bool {
	r1, _, _ := procIsUserAnAdmin.Call()
	return r1 != 0
}

// ElevateSelf re-launches the current FocusFlow binary under UAC "runas" elevation,
// safely quoting paths and arguments containing spaces.
func ElevateSelf(args []string) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		quoted = append(quoted, syscall.EscapeArg(a))
	}
	paramStr := strings.Join(quoted, " ")
	cwd, _ := os.Getwd()

	verbPtr, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	exePtr, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return err
	}
	paramPtr, err := syscall.UTF16PtrFromString(paramStr)
	if err != nil {
		return err
	}
	cwdPtr, err := syscall.UTF16PtrFromString(cwd)
	if err != nil {
		return err
	}

	r1, _, callErr := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(exePtr)),
		uintptr(unsafe.Pointer(paramPtr)),
		uintptr(unsafe.Pointer(cwdPtr)),
		1, // SW_SHOWNORMAL
	)
	if r1 <= 32 {
		return fmt.Errorf("UAC elevation cancelled or failed (code %d: %v)", r1, callErr)
	}
	return nil
}

// IsPIDAlive checks whether a process with the given PID is currently active on Windows.
func IsPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if pid == os.Getpid() {
		return true
	}
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return errors.Is(err, syscall.ERROR_ACCESS_DENIED)
	}
	defer syscall.CloseHandle(h)

	var exitCode uint32
	if err := syscall.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	return exitCode == win32StillActive
}
