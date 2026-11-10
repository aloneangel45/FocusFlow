package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProcessSupervisor_NormalizationAndDenylist(t *testing.T) {
	blocked := []string{"Discord.exe", "spotify", " STEAM.EXE ", "explorer.exe", "lsass.exe", "python.exe", "focusflow.exe"}
	sup := NewProcessSupervisor(blocked)

	expectedAllowed := []string{"discord.exe", "discord", "spotify", "spotify.exe", "steam.exe", "steam"}
	for _, key := range expectedAllowed {
		if _, exists := sup.blockedNames[key]; !exists {
			t.Errorf("expected %q to be in blockedNames map", key)
		}
	}

	forbidden := []string{"explorer.exe", "explorer", "lsass.exe", "lsass", "python.exe", "python", "focusflow.exe", "focusflow"}
	for _, key := range forbidden {
		if _, exists := sup.blockedNames[key]; exists {
			t.Errorf("protected binary %q must never enter blockedNames", key)
		}
	}
}

func TestProcessSupervisor_ProtectedPIDsNeverTerminated(t *testing.T) {
	sup := NewProcessSupervisor([]string{"discord.exe"})

	for _, pid := range []int{0, 1, 4, os.Getpid(), os.Getppid()} {
		if _, ok := sup.shouldTerminateProcess(pid, "discord.exe"); ok {
			t.Errorf("protected PID %d should never be approved for termination", pid)
		}
	}

	if matched, ok := sup.shouldTerminateProcess(424242, "discord.exe"); !ok || matched != "discord.exe" {
		t.Errorf("expected normal PID 424242 discord.exe to match, got (%q, %v)", matched, ok)
	}
	if _, ok := sup.shouldTerminateProcess(424242, "svchost.exe"); ok {
		t.Errorf("svchost.exe must never match")
	}
}

func TestResolveProcFSNames_Linux15ByteCommTruncation(t *testing.T) {
	fakeProc := t.TempDir()
	pidDir := filepath.Join(fakeProc, "12345")
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Simulate Linux 15-byte TASK_COMM_LEN truncation for "epicgameslauncher" -> "epicgameslaunch"
	if err := os.WriteFile(filepath.Join(pidDir, "comm"), []byte("epicgameslaunch\n"), 0644); err != nil {
		t.Fatalf("write comm: %v", err)
	}
	cmdline := "/opt/games/epicgameslauncher\x00--silent\x00"
	if err := os.WriteFile(filepath.Join(pidDir, "cmdline"), []byte(cmdline), 0644); err != nil {
		t.Fatalf("write cmdline: %v", err)
	}

	candidates := resolveProcFSNames(fakeProc, "12345")
	sup := NewProcessSupervisor([]string{"epicgameslauncher.exe"})
	matched, ok := sup.shouldTerminateProcess(12345, candidates...)
	if !ok || matched != "epicgameslauncher" {
		t.Fatalf("expected truncated comm to resolve full binary via cmdline, got candidates=%v matched=%q ok=%v",
			candidates, matched, ok)
	}
}

func TestAntiCheatGuard_IncludesSystemInformer(t *testing.T) {
	ac := NewAntiCheatGuard(StrictnessHardcore)
	for _, bin := range []string{"taskmgr.exe", "processhacker.exe", "systeminformer.exe", "procmon.exe", "procexp.exe"} {
		if _, exists := ac.supervisor.blockedNames[bin]; !exists {
			t.Errorf("expected %q in hardcore AntiCheatGuard", bin)
		}
	}

	relaxed := NewAntiCheatGuard(StrictnessRelaxed)
	if len(relaxed.supervisor.blockedNames) != 0 {
		t.Errorf("expected empty anti-cheat blocklist in relaxed mode")
	}
}

func TestProcessSupervisor_ScanAndTerminate_EmptyAndNonExistent(t *testing.T) {
	sup := NewProcessSupervisor(nil)
	if got := sup.ScanAndTerminate(); len(got) != 0 {
		t.Errorf("expected 0 terminated for empty supervisor, got %d", len(got))
	}
	if err := KillProcessesByNames(nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	sup2 := NewProcessSupervisor([]string{"nonexistent_focusflow_test_proc_987654.exe"})
	if got := sup2.ScanAndTerminate(); len(got) != 0 {
		t.Errorf("expected 0 terminated for nonexistent process, got %d", len(got))
	}
}
