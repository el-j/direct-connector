//go:build windows

package tunnel

import (
	"os/exec"
	"strconv"
)

// killProcessTree forcefully terminates a process and all its children on Windows.
// This is especially important for wsl.exe, where the ssh child lives in the WSL
// Linux namespace and may not receive the SIGKILL sent to the parent wsl.exe process.
func killProcessTree(pid int) {
	_ = exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
}
