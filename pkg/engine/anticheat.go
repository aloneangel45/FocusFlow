package engine

type StrictnessLevel string

const (
	StrictnessRelaxed  StrictnessLevel = "relaxed"
	StrictnessStandard StrictnessLevel = "standard"
	StrictnessHardcore StrictnessLevel = "hardcore"
)

type AntiCheatGuard struct {
	level      StrictnessLevel
	supervisor *ProcessSupervisor
}

func NewAntiCheatGuard(level StrictnessLevel) *AntiCheatGuard {
	var blocked []string
	if level == StrictnessHardcore {
		blocked = []string{
			"taskmgr.exe",
			"processhacker.exe",
			"procmon.exe",
			"procexp.exe",
		}
	}
	return &AntiCheatGuard{
		level:      level,
		supervisor: NewProcessSupervisor(blocked),
	}
}

func (a *AntiCheatGuard) Enforce() []string {
	if a.level == StrictnessHardcore {
		return a.supervisor.ScanAndTerminate()
	}
	return nil
}
