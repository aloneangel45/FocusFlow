//go:build !windows

package engine

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

func replaceEnvVar(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			if !replaced {
				out = append(out, prefix+value)
				replaced = true
			}
			continue
		}
		out = append(out, entry)
	}
	if !replaced {
		out = append(out, prefix+value)
	}
	return out
}

func buildPlatformHookCommand(ctx context.Context, command string, opts HookOptions) (*exec.Cmd, error) {
	drop, uid, gid, err := ResolvePosixHookPrivilege(
		os.Geteuid(),
		os.Getenv("SUDO_UID"),
		os.Getenv("SUDO_GID"),
		opts.AllowRootHooks,
	)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	env := os.Environ()
	sysAttr := &syscall.SysProcAttr{
		Setpgid: true,
	}

	if drop {
		sysAttr.Credential = &syscall.Credential{
			Uid:         uid,
			Gid:         gid,
			NoSetGroups: true,
		}
		if usr, lookupErr := user.LookupId(strconv.FormatUint(uint64(uid), 10)); lookupErr == nil && usr != nil {
			if strings.TrimSpace(usr.HomeDir) != "" {
				env = replaceEnvVar(env, "HOME", usr.HomeDir)
			}
			if strings.TrimSpace(usr.Username) != "" {
				env = replaceEnvVar(env, "USER", usr.Username)
				env = replaceEnvVar(env, "LOGNAME", usr.Username)
			}
		}
	}

	cmd.SysProcAttr = sysAttr
	// On timeout/cancellation, SIGKILL the entire child process group so background
	// grandchild processes spawned by `sh -c` cannot linger or hold stdout/stderr pipes open.
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}

	cmd.Env = env
	return cmd, nil
}
