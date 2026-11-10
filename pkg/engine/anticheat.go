package engine

type StrictnessLevel string

const (
	StrictnessRelaxed  StrictnessLevel = "relaxed"
	StrictnessStandard StrictnessLevel = "standard"
	StrictnessHardcore StrictnessLevel = "hardcore"
)

var HardcoreAntiCheatBinaries = []string{
	"taskmgr.exe",
	"processhacker.exe",
	"SystemInformer.exe",
	"procmon.exe",
	"procmon64.exe",
	"procexp.exe",
	"procexp64.exe",
}

type AntiCheatGuard struct {
	level      StrictnessLevel
	supervisor *ProcessSupervisor
}

func NewAntiCheatGuard(level StrictnessLevel) *AntiCheatGuard {
	var blocked []string
	if level == StrictnessHardcore {
		blocked = append(blocked, HardcoreAntiCheatBinaries...)
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
