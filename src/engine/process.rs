use std::collections::HashSet;
use windows_sys::Win32::Foundation::{CloseHandle, INVALID_HANDLE_VALUE};
use windows_sys::Win32::System::Diagnostics::ToolHelp::{
    CreateToolhelp32Snapshot, Process32First, Process32Next, PROCESSENTRY32, TH32CS_SNAPPROCESS,
};
use windows_sys::Win32::System::Threading::{OpenProcess, TerminateProcess, PROCESS_TERMINATE};

pub struct ProcessSupervisor {
    blocked_process_names: HashSet<String>,
}

impl ProcessSupervisor {
    pub fn new(blocked: &[String]) -> Self {
        let mut set = HashSet::new();
        for b in blocked {
            set.insert(b.trim().to_lowercase());
        }
        Self {
            blocked_process_names: set,
        }
    }

    pub fn scan_and_terminate(&self) -> Vec<String> {
        let mut terminated = Vec::new();
        if self.blocked_process_names.is_empty() {
            return terminated;
        }

        unsafe {
            let snapshot = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0);
            if snapshot == INVALID_HANDLE_VALUE {
                return terminated;
            }

            let mut entry: PROCESSENTRY32 = std::mem::zeroed();
            entry.dwSize = std::mem::size_of::<PROCESSENTRY32>() as u32;

            if Process32First(snapshot, &mut entry) != 0 {
                loop {
                    // Extract process name from CHAR array
                    let exe_name = Self::parse_exe_name(&entry.szExeFile);
                    let exe_lower = exe_name.to_lowercase();

                    if self.blocked_process_names.contains(&exe_lower) {
                        let h_proc = OpenProcess(PROCESS_TERMINATE, 0, entry.th32ProcessID);
                        if !h_proc.is_null() && h_proc != INVALID_HANDLE_VALUE {
                            if TerminateProcess(h_proc, 1) != 0 {
                                terminated.push(format!("{} (PID: {})", exe_name, entry.th32ProcessID));
                            }
                            CloseHandle(h_proc);
                        }
                    }

                    if Process32Next(snapshot, &mut entry) == 0 {
                        break;
                    }
                }
            }

            CloseHandle(snapshot);
        }

        terminated
    }

    fn parse_exe_name(raw: &[i8]) -> String {
        let mut bytes = Vec::new();
        for &b in raw {
            if b == 0 {
                break;
            }
            bytes.push(b as u8);
        }
        String::from_utf8_lossy(&bytes).to_string()
    }
}
